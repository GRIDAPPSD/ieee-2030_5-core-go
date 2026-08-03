// Package assembly exports BuildProtocolRouter, Stores, RouterConfig, and
// AuthPolicy: the surface a consumer server (or a test) needs to assemble
// a fully-wired IEEE 2030.5 protocol mux from core handlers.
//
// Design (IEEECORE-001, Noor 2026-06-23):
//
//   - Stores is a verbatim lift of the reference server's Stores struct; every
//     field resolves to pkg/store or pkg/store/memory.
//   - RouterConfig replaces *config.Config so core never imports internal/config.
//   - AuthPolicy injects the auth seam (middleware wrap, identity accessor, SFDI
//     prefix guard) so core never imports internal/auth.
//   - BuildProtocolRouter wires all core handlers plus the four ported families
//     (enddevice, der, fsa, registration) and returns the composed http.Handler
//     and the sorted pattern list for boot-time logging.
//
// # This package is the server's router and is destined to move
//
// A router is server state. A 2030.5 client has no router: it issues requests
// against hrefs a server handed it. Under the four-layer target architecture
// (core is the shared library for client and server, server-go is the server,
// the bridge grafts onto the server), this package belongs in server-go, and it
// lives in core today only because the server currently does. Nothing here is
// part of the shared client-and-server surface, and callers should not treat it
// as such.
//
// That applies to everything routing-shaped in this package, including the
// mintable-href assertion in hrefs.go and the guards that back it in
// hrefsource_test.go and pathvalue_test.go: they check the router against
// itself, so they move with the router rather than calcifying here. Keep them
// coupled to BuildProtocolRouter and to the handler packages, and to nothing in
// the client half of core, so that the relocation stays a move rather than a
// breaking API change.
//
// The relocation is sequenced with the layering split, not with any one card.
// See the bridge/core boundary analysis (architecture decision IEEECORE-068)
// for the reasoning, and pkg/store's package documentation for the same note
// about the store contract.
package assembly

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"

	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2"
	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2/encoding"
	coreconfiguration "github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2srv/handlers/configuration"
	coredcap "github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2srv/handlers/dcap"
	coreder "github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2srv/handlers/der"
	coredevinfo "github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2srv/handlers/device_info"
	coreedev "github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2srv/handlers/enddevice"
	coreflowrsv "github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2srv/handlers/flow_reservation"
	corefsa "github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2srv/handlers/fsa"
	corelisthandler "github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2srv/handlers/listhandler"
	corelogevent "github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2srv/handlers/logevent"
	coremessaging "github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2srv/handlers/messaging"
	coremetering "github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2srv/handlers/metering"
	corepowerstatus "github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2srv/handlers/power_status"
	corereg "github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2srv/handlers/registration"
	coreresponse "github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2srv/handlers/response"
	coresdev "github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2srv/handlers/sdev"
	coresep2time "github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2srv/handlers/sep2time"
	coresingleton "github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2srv/handlers/singleton"
	coresub "github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2srv/handlers/subscription"
	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2srv/paging"
	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/store"
	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/store/memory"
)

// Stores holds all resource stores for the protocol router.
// Verbatim lift of the reference server's internal/server.Stores (router.go
// lines 31-84); every field already resolves to pkg/store or pkg/store/memory.
type Stores struct {
	EndDevices store.EndDeviceStore

	// EndDeviceIndexes allocates the opaque, server-chosen index that
	// addresses an EndDevice in resource URLs ("/edev/3/rg"). It is an
	// ADDRESSING mechanism only: device identity remains the
	// certificate-derived LFDI on the EndDevice record, and every ownership
	// check still compares that stored LFDI against the caller's
	// certificate.
	//
	// Nil is permitted and BuildProtocolRouter substitutes a fresh
	// process-local allocator, which keeps a zero-value Stores usable in
	// tests. A production server SHOULD supply
	// memory.NewEndDeviceIndexWithPersistence so indices survive restart;
	// with the process-local substitute they do not, and a restart
	// re-addresses the fleet. See memory.EndDeviceIndex for why that
	// matters and what a client's poll cycle does and does not recover.
	EndDeviceIndexes *memory.EndDeviceIndex
	// Registrations is the persistent-aware wrapper around the in-memory
	// Store[sep2.Registration]. The embedded *Store gives back-compat
	// method promotion (Get/List/Count) for call sites that don't need
	// the persistence flush.
	Registrations *memory.RegistrationStore

	// RegistrationPolicy supplies the pIN and pollRate for the Registration
	// that is created with every EndDevice (IEEECORE-083).
	//
	// The zero value provisions nothing, and that is fail-closed rather
	// than degraded: a server that cannot say what a device's pIN is has no
	// Registration to serve, so the device is served with no
	// RegistrationLink at all instead of one that answers 404. Wiring a
	// PIN resolver is what turns registration on.
	//
	// It is honored only when Registrations is non-nil, which is the same
	// condition that mounts GET /edev/{id}/rg: link and route are enabled
	// by one decision, so neither can exist without the other.
	//
	// An EndDevices store that is ALREADY a *memory.RegisteredEndDeviceStore
	// is left alone and this field is ignored for it. That is the path an
	// embedder that seeds devices at boot must take: the coupling has to be
	// in place before the first seeded Create, which happens before this
	// router is built.
	RegistrationPolicy memory.RegistrationPolicy

	MirrorUsagePoints   *memory.Store[sep2.MirrorUsagePoint]
	MirrorMeterReadings *memory.ScopedStore[sep2.MirrorMeterReading]

	// DER stores
	DERs              *memory.ScopedStore[sep2.DER]
	DERCapabilities   *memory.ScopedStore[sep2.DERCapability]
	DERSettings       *memory.ScopedStore[sep2.DERSettings]
	DERStatuses       *memory.ScopedStore[sep2.DERStatus]
	DERAvailabilities *memory.ScopedStore[sep2.DERAvailability]
	// DERPrograms is the persistent-aware wrapper. It embeds
	// *ScopedStore[sep2.DERProgram] so existing handlers that call
	// .ForParent(...) keep working unchanged; the shadowed
	// Create/Delete add the disk flush.
	DERPrograms        *memory.DERProgramStore
	DERControls        *memory.ScopedStore[sep2.DERControl]
	DefaultDERControls *memory.ScopedStore[sep2.DefaultDERControl]
	DERCurves          *memory.Store[sep2.DERCurve]

	// FSA store
	FSAs *memory.ScopedStore[sep2.FunctionSetAssignments]

	// IEEE-096: admin FSA management plane (operator-authored templates,
	// program links, device assignments). Distinct from FSAs above which is
	// the spec-facing scoped surface.
	AdminFSAs *memory.AdminFSAStore

	// Subscription store
	Subscriptions *memory.SubscriptionStore

	// Server-side metering
	UsagePoints   *memory.Store[sep2.UsagePoint]
	MeterReadings *memory.ScopedStore[sep2.MeterReading]
	Readings      *memory.ScopedStore[sep2.Reading]
	ReadingTypes  *memory.Store[sep2.ReadingType]

	// New function sets
	Configurations           *memory.ScopedStore[sep2.Configuration]
	DeviceStatuses           *memory.ScopedStore[sep2.DeviceStatus]
	LogEvents                *memory.ScopedStore[sep2.LogEvent]
	PowerStatuses            *memory.ScopedStore[sep2.PowerStatus]
	MessagingPrograms        *memory.Store[sep2.MessagingProgram]
	TextMessages             *memory.ScopedStore[sep2.TextMessage]
	FlowReservationRequests  *memory.ScopedStore[sep2.FlowReservationRequest]
	FlowReservationResponses *memory.ScopedStore[sep2.FlowReservationResponse]
	ResponseSets             *memory.Store[sep2.ResponseSet]
	Responses                *memory.ScopedStore[sep2.Response]
}

// RouterConfig carries the scalar configuration values the protocol router
// needs. It replaces the server-side *config.Config so core never imports
// internal/config. Field types match config.Config exactly.
type RouterConfig struct {
	TZOffset    int32
	DSTOffset   int32
	DSTStart    int64 // DST start (unix seconds)
	DSTEnd      int64 // DST end (unix seconds)
	TimeQuality uint8 // sep2.TimeQuality* values

	// PostRateProvider supplies the server's preferred
	// MirrorUsagePoint.postRate for the client creating a mirror, keyed on
	// that client's LFDI. It is threaded to POST /mup; see
	// metering.PostRateProvider for the sep.xsd:6487 basis and the override
	// semantics.
	//
	// Nil (the zero value) means the server states no preference, and POST
	// /mup stores whatever postRate the client supplied, which is the
	// behavior every consumer had before this field existed. It lives on
	// RouterConfig rather than as a new BuildProtocolRouter parameter so
	// adding it breaks no existing caller.
	//
	// Rate policy is server-owned, not core-owned: core deliberately ships
	// no default here, because "how often may this client post to me" is an
	// ingest-budget question only the deploying server can answer.
	PostRateProvider coremetering.PostRateProvider
}

// AuthPolicy bundles the three auth touch points the protocol router and the
// ported handlers need. The server wires its internal/auth implementations;
// tests supply pass-through stubs. Core imports nothing from internal/auth.
//
// Zero-value safety: a zero-value AuthPolicy is safe and fail-closed.
// BuildProtocolRouter substitutes deny-all stubs for any nil func field
// before wiring, so a nil Identity or SFDIPrefix never causes a nil-panic
// at request time: they return ok=false (403) and an error (500)
// respectively. A nil Wrap installs NO middleware (no ACL enforcement);
// this is intentional for tests but is NOT safe for production: see the
// Wrap field comment below.
type AuthPolicy struct {
	// Wrap composes the identity and ACL middleware around the protocol mux.
	// The server passes auth.IdentityMiddleware composed with
	// auth.ACLMiddleware(auth.DefaultACLRules()); a test passes a no-op.
	//
	// WARNING: nil Wrap disables ALL middleware for the protocol mux,
	// meaning no TLS identity is extracted and no ACL rules are applied.
	// This is only safe for unit tests. A production server MUST supply a
	// non-nil Wrap that includes at minimum auth.IdentityMiddleware and
	// auth.ACLMiddleware; BuildProtocolRouter logs a warning when Wrap is nil.
	Wrap func(http.Handler) http.Handler

	// Identity extracts the authenticated device identity from the request
	// context. Replaces the direct auth.GetIdentity calls in the ported
	// edev and registration handlers. Returns ok=false when unauthenticated,
	// causing those handlers to return 403 Forbidden.
	//
	// If nil, BuildProtocolRouter substitutes a deny-all stub (always
	// returns ok=false) so handlers fail closed rather than panicking.
	Identity func(ctx context.Context) (lfdi, sfdi string, ok bool)

	// SFDIPrefix derives the EndDevice id prefix from an SFDI, replacing
	// auth.ExtractSFDIPrefix (IEEE-014 short-SFDI guard). Injected so the
	// guard policy stays server-owned.
	//
	// If nil, BuildProtocolRouter substitutes a stub that always returns an
	// error so the create path fails with 500 rather than panicking.
	SFDIPrefix func(sfdi string) (string, error)
}

// ResourceNotifier is aliased from the enddevice package so consumers can
// name assembly.ResourceNotifier without importing the enddevice subpackage.
// (IEEECORE-002)
type ResourceNotifier = coreedev.ResourceNotifier

// BuildProtocolRouter creates the HTTP router for the protocol listener
// and returns the canonical pattern list mounted on its protocol mux.
// The notifier is invoked on resource state changes that drive subscription
// fan-out (e.g. CSIP V1.2 MAINT-002 EndDevice DELETE). Pass nil to disable
// notification: tests that don't care about subscriptions can do this.
//
// The admin surface (AdminCertService, admin_* routes) and the test-mutation
// surface (RegisterMutationHandlers) are NOT included: they are
// server-config-specific and not part of the core export. Admin FSA create
// (HandleCreateFSA in the fsa handler package) is one such consumer-wired
// admin handler: it is wired by the consuming server on its own admin mux,
// not here.
func BuildProtocolRouter(
	cfg RouterConfig,
	stores *Stores,
	authPolicy AuthPolicy,
	serverSFDI, serverLFDI string,
	notifier ResourceNotifier,
) (http.Handler, []string) {
	// F1: substitute deny-all stubs for nil func fields so zero-value
	// AuthPolicy is safe and fail-closed, never a nil-panic at request time.
	if authPolicy.Identity == nil {
		authPolicy.Identity = func(_ context.Context) (string, string, bool) {
			return "", "", false // deny: handlers return 403
		}
	}
	if authPolicy.SFDIPrefix == nil {
		authPolicy.SFDIPrefix = func(_ string) (string, error) {
			return "", fmt.Errorf("SFDIPrefix not configured: deny")
		}
	}

	// F2: nil Wrap disables all middleware (no TLS identity extraction, no
	// ACL enforcement). Log loudly so a production misconfiguration is visible.
	if authPolicy.Wrap == nil {
		log.Print("assembly: AuthPolicy.Wrap is nil: no identity middleware and no ACL enforcement; safe for tests only")
	}

	top := http.NewServeMux()

	protocolMux := newRecordingMux()
	protocolMux.HandleFunc("GET /dcap", coredcap.HandleDeviceCapability())
	protocolMux.HandleFunc("GET /tm", coresep2time.HandleTime(coresep2time.TimeParams{
		TZOffset:    cfg.TZOffset,
		DSTOffset:   cfg.DSTOffset,
		DSTStart:    cfg.DSTStart,
		DSTEnd:      cfg.DSTEnd,
		TimeQuality: cfg.TimeQuality,
	}))
	protocolMux.HandleFunc("GET /sdev", coresdev.HandleSelfDevice(serverSFDI, serverLFDI))
	protocolMux.HandleFunc("GET /sdev/sdi", coredevinfo.HandleDeviceInformation(serverLFDI))

	if stores != nil {
		registerEndDeviceRoutes(protocolMux, stores, authPolicy, notifier)
		registerMirrorRoutes(protocolMux, stores, authPolicy, cfg.PostRateProvider)
		registerDERRoutes(protocolMux, stores)
		registerMeteringRoutes(protocolMux, stores)
		registerNewFunctionSetRoutes(protocolMux, stores)
	}

	var protocolChain http.Handler
	if authPolicy.Wrap != nil {
		protocolChain = authPolicy.Wrap(protocolMux)
	} else {
		protocolChain = protocolMux
	}

	for _, prefix := range topLevelMounts {
		top.Handle(prefix, protocolChain)
	}

	return encoding.NamespaceMiddleware(top), protocolMux.Patterns()
}

// topLevelMounts are the prefixes under which the protocol mux is mounted on
// the outer mux. A pattern registered on the protocol mux whose family is
// absent here is unreachable no matter how well formed it is, which is the
// same advertised-but-unrouted defect one layer up, so the href probe in
// hrefs.go mounts the same list rather than testing the protocol mux alone.
//
// The paired bare and trailing-slash entries are both required: "/edev"
// matches only the collection, "/edev/" matches everything beneath it.
var topLevelMounts = []string{
	"/dcap", "/tm", "/sdev", "/sdev/",
	"/edev", "/edev/",
	"/mup", "/mup/",
	"/dc", "/dc/",
	"/upt", "/upt/", "/rt", "/rt/",
	"/msg", "/msg/",
	"/rsps", "/rsps/",
}

// routeRegistrar is the surface the register*Routes helpers need from a mux.
// *http.ServeMux satisfies it; recordingMux below satisfies it and captures
// the pattern list.
type routeRegistrar interface {
	HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request))
}

// recordingMux is a thin wrapper around *http.ServeMux that captures every
// pattern handed to HandleFunc so Patterns() can enumerate them at boot.
type recordingMux struct {
	mux      *http.ServeMux
	patterns []string
}

func newRecordingMux() *recordingMux {
	return &recordingMux{mux: http.NewServeMux()}
}

func (r *recordingMux) HandleFunc(pattern string, h func(http.ResponseWriter, *http.Request)) {
	r.mux.HandleFunc(pattern, h)
	r.patterns = append(r.patterns, pattern)
}

func (r *recordingMux) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.mux.ServeHTTP(w, req)
}

// Patterns returns a sorted, de-duplicated copy of every registered pattern.
func (r *recordingMux) Patterns() []string {
	if len(r.patterns) == 0 {
		return nil
	}
	out := make([]string, len(r.patterns))
	copy(out, r.patterns)
	sort.Strings(out)
	w := 0
	for i, v := range out {
		if i == 0 || v != out[w-1] {
			out[w] = v
			w++
		}
	}
	return out[:w]
}

// notifyRemover is a local interface for the type assertion in asNotifyRemoved.
// core's *subscription.Manager satisfies it.
// Defined here at the consumer (Pike rule: interfaces at the consumer).
type notifyRemover interface {
	NotifyRemoved(ctx context.Context, sub sep2.Subscription) error
}

// asNotifyRemoved extracts NotifyRemoved as a function value if n implements
// notifyRemover, or returns nil. Keeps the router free of a hard import on
// the subscription package: ResourceNotifier is the published parameter surface
// and the production *subscription.Manager satisfies both interfaces.
func asNotifyRemoved(n ResourceNotifier) func(context.Context, sep2.Subscription) error {
	if n == nil {
		return nil
	}
	if nr, ok := n.(notifyRemover); ok {
		return nr.NotifyRemoved
	}
	return nil
}

// registrationBoundEndDevices returns the EndDevice store the /edev routes
// must use: one that writes an EndDevice and its Registration as a single
// act and serves a RegistrationLink only when the record behind it exists
// (IEEECORE-083).
//
// It decorates rather than replaces, and it declines to decorate twice. An
// embedder that seeds devices at boot has to build the binding itself,
// because seeding happens before this router is assembled; when it hands us
// the bound store it already has, wrapping it again would put a second
// binding with an empty policy in front of the configured one and silently
// deprovision the fleet.
//
// A nil Registrations store means the /rg route is not mounted at all, so
// there is nothing to bind to and nothing to advertise. The undecorated
// store is returned and no EndDevice carries a RegistrationLink, which is
// what 2018 section 4.4 p.19 requires of an unimplemented function set.
func registrationBoundEndDevices(stores *Stores) store.EndDeviceStore {
	if stores.EndDevices == nil || stores.Registrations == nil {
		return stores.EndDevices
	}
	if bound, ok := stores.EndDevices.(*memory.RegisteredEndDeviceStore); ok {
		return bound
	}
	return memory.NewRegisteredEndDeviceStore(stores.EndDevices, stores.Registrations, stores.RegistrationPolicy)
}

func registerEndDeviceRoutes(mux routeRegistrar, stores *Stores, authPolicy AuthPolicy, notifier ResourceNotifier) {
	// A nil index allocator is substituted rather than rejected so a
	// zero-value Stores stays usable, but the substitute is process-local:
	// log it, because on a production server it means every device is
	// re-addressed on restart.
	edevIndexes := stores.EndDeviceIndexes
	if edevIndexes == nil {
		log.Print("assembly: Stores.EndDeviceIndexes is nil: using a process-local EndDevice index; URL indices will NOT survive restart")
		edevIndexes = memory.NewEndDeviceIndex()
	}

	// IEEECORE-083: every EndDevice route reads and writes through the
	// registration-bound store, so the EndDevice and its Registration are
	// one act on the write side and one derivation on the read side. Every
	// route below takes edevs, not stores.EndDevices: a route left on the
	// undecorated store would be the one that reintroduces the drift.
	edevs := registrationBoundEndDevices(stores)

	mux.HandleFunc("GET /edev", corelisthandler.ListHandler[sep2.EndDevice, sep2.EndDeviceList](
		edevs, coreedev.BuildEndDeviceList, 900,
	))
	mux.HandleFunc("POST /edev", coreedev.HandleCreateEndDevice(edevs, edevIndexes, authPolicy.Identity, authPolicy.SFDIPrefix))
	mux.HandleFunc("GET /edev/{id}", coreedev.HandleEndDevice(edevs))
	mux.HandleFunc("PUT /edev/{id}", coreedev.HandleUpdateEndDevice(edevs))
	mux.HandleFunc("DELETE /edev/{id}", coreedev.HandleDeleteEndDevice(edevs, notifier))

	// IEEE-101: Registration GET handler at /edev/{id}/rg.
	if stores.Registrations != nil {
		mux.HandleFunc("GET /edev/{id}/rg", corereg.HandleGetRegistration(edevs, stores.Registrations, authPolicy.Identity))
	}

	// FSA endpoints
	if stores.FSAs != nil {
		mux.HandleFunc("GET /edev/{id}/fsa", scopedListHandler[sep2.FunctionSetAssignments, sep2.FunctionSetAssignmentsList](
			stores.FSAs, "id", corefsa.BuildFSAList, 900,
		))
		mux.HandleFunc("GET /edev/{id}/fsa/{fsaId}", corefsa.HandleFSA(stores.FSAs))
	}

	// Subscription endpoints
	if stores.Subscriptions != nil {
		mux.HandleFunc("GET /edev/{id}/sub", coresub.HandleListSubscriptionsByDevice(stores.Subscriptions, 900))
		mux.HandleFunc("POST /edev/{id}/sub", coresub.HandleCreateSubscription(stores.Subscriptions))
		mux.HandleFunc("DELETE /edev/{id}/sub/{subId}", coresub.HandleDeleteSubscription(stores.Subscriptions, asNotifyRemoved(notifier)))
	}
}

func registerMirrorRoutes(mux routeRegistrar, stores *Stores, authPolicy AuthPolicy, postRateProvider coremetering.PostRateProvider) {
	if stores.MirrorUsagePoints == nil {
		return
	}
	// LFDIProvider extracts the device LFDI from the request context via the
	// injected AuthPolicy.Identity. Keeps internal/auth out of core (the same
	// callback-injection pattern that Phase D1 applied to the obs callback).
	lfdiProvider := coremetering.LFDIProvider(func(ctx context.Context) (string, bool) {
		lfdi, _, ok := authPolicy.Identity(ctx)
		return lfdi, ok
	})
	mux.HandleFunc("GET /mup", corelisthandler.ListHandler[sep2.MirrorUsagePoint, sep2.MirrorUsagePointList](
		stores.MirrorUsagePoints, coremetering.BuildMirrorUsagePointList, 300,
	))
	mux.HandleFunc("POST /mup", coremetering.HandleCreateMirrorUsagePoint(stores.MirrorUsagePoints, lfdiProvider, postRateProvider))
	mux.HandleFunc("GET /mup/{id}", coremetering.HandleMirrorUsagePoint(stores.MirrorUsagePoints, lfdiProvider))
	mux.HandleFunc("POST /mup/{id}/mr", coremetering.HandlePostMirrorMeterReading(
		stores.MirrorUsagePoints, stores.MirrorMeterReadings, lfdiProvider,
	))

	// IEEE 2030.5-2018 section 10.11.3 rule (d): the client posts readings
	// "to the resource identified in the Metering server's response... (e.g.,
	// /mup/3)" -- that resource is the Location header POST /mup returns,
	// which HandleCreateMirrorUsagePoint sets to exactly "/mup/{id}". The
	// WADL marks POST /mup/{id} Mandatory. Before this route existed, a
	// client that followed our own advertised Location header got a 405: the
	// EPRI reference client does exactly that (retrieve.c's
	// process_response reads http_location() on the POST /mup response and
	// posts the follow-up MirrorMeterReading to that literal path, not to
	// our /mr convention). Mounting it against the identical handler used
	// for POST /mup/{id}/mr means both routes share stampMirrorMeterReading
	// and the section 10.11.3 rule (e) ownership gate, so the two can never
	// drift into minting different href shapes, or into enforcing creator
	// scope on one path and not the other, for the same resource kind.
	mux.HandleFunc("POST /mup/{id}", coremetering.HandlePostMirrorMeterReading(
		stores.MirrorUsagePoints, stores.MirrorMeterReadings, lfdiProvider,
	))
}

func registerDERRoutes(mux routeRegistrar, stores *Stores) {
	if stores.DERs == nil {
		return
	}

	dercap, derg, ders, dera := coreder.DERSingletonHandlers(
		stores.DERCapabilities, stores.DERSettings, stores.DERStatuses, stores.DERAvailabilities,
	)

	// Which DER sub-resource links this router is permitted to advertise, per
	// IEEE 2030.5-2018 section 4.4 ("If a function set is not implemented, Link
	// elements to resources in that function set SHALL NOT be included").
	//
	// It is declared HERE, next to the mounts it describes, because the answer
	// to "is this function set implemented" is a property of this function and
	// nothing else. A parallel constant elsewhere could rot; this cannot drift
	// further than the next four lines. The four sub-resources below are mounted
	// unconditionally whenever the DER function set is wired at all, which is
	// what makes all four bits true. A card that mounts a fifth flips its bit in
	// the same commit: mounting and advertising are one act.
	derLinks := coreder.DERLinkPolicy{
		Capability:   true,
		Settings:     true,
		Status:       true,
		Availability: true,
	}

	mux.HandleFunc("GET /edev/{id}/der", scopedListHandler[sep2.DER, sep2.DERList](
		stores.DERs, "id", coreder.DERListBuilder(derLinks), 900,
	))

	// The DER instance itself (IEEECORE-052). Every DERList member carries this
	// href as its own, and before this route existed following it produced a 404
	// from the server that had just advertised it. HEAD comes free: a ServeMux
	// pattern registered for GET matches HEAD as well.
	//
	// PUT is mode O in the WADL (sep_wadl.xml:4116) but on the certified path:
	// SunSpec CTP CORE-014 and CORE-016 walk DERList through to the DER and PUT
	// to these resources. GET and PUT share ONE handler value so the two verbs
	// cannot drift into different scope derivations, the same reason the four
	// sub-resources above are registered as pairs against one handler.
	//
	// DELETE (mode O, deferred to IEEECORE-058) and POST (mode E) fall through
	// to a 405 carrying an Allow header derived from itemMethods, which section
	// 4.3 c) 4) requires and an unmounted path could not produce: it would 404.
	derInstance := scopedResourceHandler[sep2.DER](
		stores.DERs, "id", "derId", itemMethods{Put: true}, coreder.StampDERInstance(derLinks),
	)
	mux.HandleFunc("GET /edev/{id}/der/{derId}", derInstance)
	mux.HandleFunc("PUT /edev/{id}/der/{derId}", derInstance)

	mux.HandleFunc("GET /edev/{id}/der/{derId}/dercap", dercap)
	mux.HandleFunc("PUT /edev/{id}/der/{derId}/dercap", dercap)
	mux.HandleFunc("GET /edev/{id}/der/{derId}/derg", derg)
	mux.HandleFunc("PUT /edev/{id}/der/{derId}/derg", derg)
	mux.HandleFunc("GET /edev/{id}/der/{derId}/ders", ders)
	mux.HandleFunc("PUT /edev/{id}/der/{derId}/ders", ders)
	mux.HandleFunc("GET /edev/{id}/der/{derId}/dera", dera)
	mux.HandleFunc("PUT /edev/{id}/der/{derId}/dera", dera)

	// DERProgram under FSA: use the embedded *ScopedStore so the helper
	// signature stays unchanged. Writes through stores.DERPrograms.Create
	// still go through the persistent wrapper (the list handler is read-only).
	mux.HandleFunc("GET /edev/{id}/fsa/{fsaId}/derp", scopedListHandler[sep2.DERProgram, sep2.DERProgramList](
		stores.DERPrograms.ScopedStore, "id", coreder.BuildDERProgramList, 900,
	))

	// A DERProgram's own href, so the FSA-to-DERProgramList-to-member link
	// walk CSIP v2.0 s5.2.3.1 requires actually resolves (IEEECORE-082).
	// Scoped by device {id} only, matching the list route above (and the
	// DERProgram store's own scoping): {fsaId} is part of the mounted path
	// shape, not a filter on which programs are visible under it. Read-only:
	// core exposes no write route for a single DERProgram, mirroring the
	// DERControl single-resource route just below. The empty itemMethods is
	// what says so, and it renders exactly the Allow this route answered with
	// before the method set became declarative: GET, HEAD.
	mux.HandleFunc("GET /edev/{id}/fsa/{fsaId}/derp/{derpId}",
		scopedResourceHandler[sep2.DERProgram](stores.DERPrograms.ScopedStore, "id", "derpId", itemMethods{}, nil))

	// DERControl under DERProgram
	mux.HandleFunc("GET /edev/{id}/fsa/{fsaId}/derp/{derpId}/derc",
		scopedListHandlerDeep[sep2.DERControl, sep2.DERControlList](
			stores.DERControls, coreder.BuildDERControlList, 900,
		))

	// A DERControl's own href, so an activated event resolves. A client that
	// activates an event arms a fast poll against the event's own URI, so
	// without this route that poll 404s and the client tears the event down
	// (the EPRI reference client turns a non-200 into RETRIEVE_FAIL and then
	// calls remove_stub), which caps delivery at one event per client.
	//
	// Scoped by the SAME composite parent key as the list route above
	// (id/fsaId/derpId, see scopedResourceHandlerDeep), so a control is
	// reachable only under the device path it was stored beneath. Read-only:
	// the DOWN path writes controls through the store, never over HTTP.
	//
	// Stamped with the response request on the way out, exactly as the list
	// route stamps its members (IEEECORE-067). The adapter drops the request
	// because the stamp does not depend on it: replyTo names one server-owned
	// URI and responseRequired is a constant, unlike the DER instance stamp
	// above, which derives links from path values.
	mux.HandleFunc("GET /edev/{id}/fsa/{fsaId}/derp/{derpId}/derc/{dercId}",
		scopedResourceHandlerDeep[sep2.DERControl](stores.DERControls, "dercId",
			func(_ *http.Request, ctrl *sep2.DERControl) { coreder.StampResponseRequest(ctrl) }))

	// DefaultDERControl
	mux.HandleFunc("GET /edev/{id}/fsa/{fsaId}/derp/{derpId}/dderc",
		coreder.DefaultDERControlHandler(stores.DefaultDERControls))
	mux.HandleFunc("PUT /edev/{id}/fsa/{fsaId}/derp/{derpId}/dderc",
		coreder.DefaultDERControlHandler(stores.DefaultDERControls))

	// Global DERCurve
	mux.HandleFunc("GET /dc", corelisthandler.ListHandler[sep2.DERCurve, sep2.DERCurveList](
		stores.DERCurves, coreder.BuildDERCurveList, 900,
	))
}

// scopedListHandler creates a list handler scoped by the path value named
// parentParam.
//
// parentParam is passed rather than hardcoded to "id" for the reason argued at
// [scopedResourceHandler], and this helper is where that defect actually shipped
// (IEEECORE-059). The parent wildcard is not called {id} on every mounted shape:
// the metering family names it {uptId} and the messaging family {msgId}.
// r.PathValue on a wildcard the pattern does not declare returns "" rather than
// failing, so a hardcoded "id" scoped every lookup on those two shapes under the
// empty parent, and both lists served empty forever with a 200 and no log line.
// Naming the parameter at the mount is what prevents it: a wrong name is now a
// visible mismatch between the mount and the pattern, which the guard in
// pathvalue_test.go reads directly.
func scopedListHandler[T store.Copier[T], L any](
	scopedStore *memory.ScopedStore[T],
	parentParam string,
	buildList func(href string, result store.ListResult[T], pollRate uint32) L,
	pollRate uint32,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		parentID := r.PathValue(parentParam)
		st := scopedStore.ForParent(parentID)
		h := corelisthandler.ListHandler[T, L](st, buildList, pollRate)
		h.ServeHTTP(w, r)
	}
}

// scopedListHandlerDeep creates a list handler scoped by composite key id/fsaId/derpId.
func scopedListHandlerDeep[T store.Copier[T], L any](
	scopedStore *memory.ScopedStore[T],
	buildList func(href string, result store.ListResult[T], pollRate uint32) L,
	pollRate uint32,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := deepScopeKey(r)
		st := scopedStore.ForParent(key)
		h := corelisthandler.ListHandler[T, L](st, buildList, pollRate)
		h.ServeHTTP(w, r)
	}
}

// deepScopeKey builds the composite parent key id/fsaId/derpId that scopes
// resources nested under a DERProgram.
//
// The list handler and the single-resource handler MUST derive their scope
// identically, which is why this is one function rather than the expression
// repeated in each. Duplicating the expression would let the two routes
// drift, and a single-resource route that scoped more loosely than its list
// would widen what a store lookup can return.
//
// This key binds a RESOURCE to the path it was stored under. It does NOT
// bind a CALLER to that path: nothing here checks that the authenticated
// caller is the device named by {id}. A caller who supplies its own {id}
// alongside another device's dercId gets a scope miss (404), but a caller
// who supplies another device's {id} directly is not rejected by this
// function at all; store scoping and caller ownership are different
// properties, and this function implements only the former. Ownership
// enforcement is IEEECORE-028's cross-cutting fix, not yet present here.
// Do not read the absence of a panic or a wrong result from this function as
// evidence that unauthorized cross-device reads are blocked.
func deepScopeKey(r *http.Request) string {
	return r.PathValue("id") + "/" + r.PathValue("fsaId") + "/" + r.PathValue("derpId")
}

// scopedResourceHandlerDeep creates a read-only single-resource handler scoped
// by the same composite key id/fsaId/derpId as scopedListHandlerDeep, keyed
// within that scope by the path value named idParam.
//
// It serves the STORED value directly rather than rebuilding a document, so
// the bytes match what the list serves for the same resource field for field.
// Behavior on the two non-happy paths is deliberate:
//
//   - A miss (wrong device, wrong program, or an id that never existed) is a
//     clean 404 with no body, never a synthesized zero-valued resource. An
//     empty 200 would be worse than the 404 this route exists to fix: a client
//     would parse the zero value as a real resource and could act on it.
//   - A store error other than not-found is a 500, because it means the store
//     failed rather than that the resource is absent, and collapsing the two
//     would report a broken server as a missing resource.
//
// stamp, when non-nil, completes a resource for the wire before it is served,
// and takes the same shape as [scopedResourceHandler]'s so the two mounts are
// read the same way. It exists here so a resource kind whose LIST is stamped on
// the way out (DERControl's replyTo and responseRequired, IEEECORE-067) is
// stamped identically on its own href: the two routes serve the same resource,
// and a field present on one and absent on the other is a conformance trap,
// which TestSingleDERControlBytesMatchListMember exists to catch.
func scopedResourceHandlerDeep[T store.Copier[T]](
	scopedStore *memory.ScopedStore[T],
	idParam string,
	stamp func(r *http.Request, resource *T),
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			encoding.MethodNotAllowed(w, "GET, HEAD")
			return
		}

		resource, err := scopedStore.Get(r.Context(), deepScopeKey(r), r.PathValue(idParam))
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		if stamp != nil {
			stamp(r, &resource)
		}

		encoding.WriteXML(w, http.StatusOK, &resource)
	}
}

// maxRequestBody caps how much of a request body this package reads into
// memory. It matches the limit the singleton handler applies to the DER
// sub-resources (handlers/singleton), so a client cannot find a larger document
// accepted on one DER path than on its neighbour.
const maxRequestBody = 1 << 20

// itemMethods declares which WADL-declared methods a [scopedResourceHandler]
// mount implements beyond GET and HEAD, which every mount serves.
//
// It is a parameter rather than a hardcoded switch because the WADL is the
// source of truth for the method set and it differs per resource. Passing it in
// means the Allow header on a 405 is generated from the same declaration that
// decides which branches exist, so the two cannot disagree. That matters
// directly: section 4.3 c) 4) requires an explicit 400 or 405 for a method in
// mode E, and a 405 whose Allow lies about what is served is barely better than
// the 404 an unmounted path would have produced.
type itemMethods struct {
	// Put mounts PUT on the resource, upserting the request body at the id the
	// path names.
	Put bool
}

// allow renders the Allow header value for a 405 on this mount.
func (m itemMethods) allow() string {
	if m.Put {
		return "GET, HEAD, PUT"
	}
	return "GET, HEAD"
}

// scopedResourceHandler creates a single-resource handler scoped by the path
// value named parentParam alone, keyed within that scope by the path value
// named idParam.
//
// parentParam is passed rather than hardcoded to "id" because the parent
// wildcard is not called {id} on every mounted shape: the messaging family
// names it {msgId}. r.PathValue on a wildcard the pattern does not declare
// returns "", which would key every lookup under the empty parent, so the
// resource would be unreachable under its own parent and reachable under every
// other one. That is a silent wrong-scope defect rather than a visible error,
// and naming the parameter at the mount is what prevents it. The sibling list
// helper [scopedListHandler] carried exactly that defect and now takes the same
// argument for the same reason (IEEECORE-059).
//
// It mirrors [scopedResourceHandlerDeep] one scope level up and keeps that
// function's contract on the non-happy paths, whose reasoning is argued there
// and not restated: a clean 404 on a miss, never a synthesized zero-valued
// resource, and a 500 rather than a 404 when the store fails for any other
// reason.
//
// The clean-404 rule is sharper here than it is one level up, because this
// handler can also accept PUT. A synthesized 200 would hand a client the
// sub-resource links of a resource that does not exist; the EPRI reference
// client follows exactly those links and PUTs into them; the singleton handler
// upserts on PUT; and store entries would then appear under an id nobody
// provisioned. A GET that fabricates a writable resource is resource creation
// through a read path. The 404 and the upserting PUT are a coherent pair only
// because the id comes from the path, which the scope and ownership layers above
// constrain, and never from this handler inventing one.
//
// stamp, when non-nil, completes a resource for the wire before it is served and
// before it is stored: it is where a resource's own href and its derived links
// are settled, so a document served from the store and a document just written
// by a client cannot disagree about either. It may be nil for a resource that
// needs no completion.
func scopedResourceHandler[T store.Copier[T]](
	scopedStore *memory.ScopedStore[T],
	parentParam string,
	idParam string,
	methods itemMethods,
	stamp func(r *http.Request, resource *T),
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		parentKey := r.PathValue(parentParam)
		id := r.PathValue(idParam)

		switch {
		case r.Method == http.MethodGet, r.Method == http.MethodHead:
			resource, err := scopedStore.Get(r.Context(), parentKey, id)
			if err != nil {
				if errors.Is(err, store.ErrNotFound) {
					http.Error(w, "not found", http.StatusNotFound)
					return
				}
				log.Printf("assembly: get parent=%q id=%q: %v (path=%s)", parentKey, id, err, r.URL.Path)
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			if stamp != nil {
				stamp(r, &resource)
			}
			encoding.WriteXML(w, http.StatusOK, &resource)

		case r.Method == http.MethodPut && methods.Put:
			body, err := io.ReadAll(io.LimitReader(r.Body, maxRequestBody))
			if err != nil {
				http.Error(w, "read body failed", http.StatusBadRequest)
				return
			}
			var resource T
			if err := xml.Unmarshal(body, &resource); err != nil {
				http.Error(w, "invalid XML: "+err.Error(), http.StatusBadRequest)
				return
			}
			// Stamped BEFORE the store write, not on the way back out, so the
			// stored document is the one the server vouches for. A client's own
			// href and any link it invented never reach the store.
			if stamp != nil {
				stamp(r, &resource)
			}
			if err := scopedStore.Create(r.Context(), parentKey, id, resource); err != nil {
				if !errors.Is(err, store.ErrAlreadyExists) {
					log.Printf("assembly: create parent=%q id=%q: %v (path=%s)", parentKey, id, err, r.URL.Path)
					http.Error(w, "internal error", http.StatusInternalServerError)
					return
				}
				if err := scopedStore.Update(r.Context(), parentKey, id, resource); err != nil {
					log.Printf("assembly: update parent=%q id=%q: %v (path=%s)", parentKey, id, err, r.URL.Path)
					http.Error(w, "internal error", http.StatusInternalServerError)
					return
				}
			}
			w.WriteHeader(http.StatusNoContent)

		default:
			encoding.MethodNotAllowed(w, methods.allow())
		}
	}
}

func registerMeteringRoutes(mux routeRegistrar, stores *Stores) {
	if stores.UsagePoints == nil {
		return
	}
	mux.HandleFunc("GET /upt", corelisthandler.ListHandler[sep2.UsagePoint, sep2.UsagePointList](
		stores.UsagePoints, coremetering.BuildUsagePointList, 900,
	))
	mux.HandleFunc("POST /upt", coremetering.HandleCreateUsagePoint(stores.UsagePoints))
	mux.HandleFunc("GET /upt/{uptId}", coremetering.HandleUsagePoint(stores.UsagePoints))

	mux.HandleFunc("GET /upt/{uptId}/mr", scopedListHandler[sep2.MeterReading, sep2.MeterReadingList](
		stores.MeterReadings, "uptId", coremetering.BuildMeterReadingList, 900,
	))

	mux.HandleFunc("GET /upt/{uptId}/mr/{mrId}/r", func(w http.ResponseWriter, r *http.Request) {
		key := r.PathValue("uptId") + "/" + r.PathValue("mrId")
		st := stores.Readings.ForParent(key)
		h := corelisthandler.ListHandler[sep2.Reading, sep2.ReadingList](st, coremetering.BuildReadingList, 900)
		h.ServeHTTP(w, r)
	})

	mux.HandleFunc("GET /rt", corelisthandler.ListHandler[sep2.ReadingType, sep2.ReadingTypeList](
		stores.ReadingTypes, coremetering.BuildReadingTypeList, 900,
	))
	mux.HandleFunc("GET /rt/{id}", coremetering.HandleReadingType(stores.ReadingTypes))
}

func registerNewFunctionSetRoutes(mux routeRegistrar, stores *Stores) {
	if stores.Configurations != nil {
		mux.HandleFunc("GET /edev/{id}/cfg", coreconfiguration.HandleConfiguration(stores.Configurations))
		mux.HandleFunc("PUT /edev/{id}/cfg", coreconfiguration.HandleConfiguration(stores.Configurations))
	}
	if stores.DeviceStatuses != nil {
		mux.HandleFunc("GET /edev/{id}/dstat", coresingleton.HandleSingletonGetPut[sep2.DeviceStatus](
			stores.DeviceStatuses,
			func(r *http.Request) string { return r.PathValue("id") },
			func(r *http.Request) sep2.DeviceStatus {
				ds := sep2.DeviceStatus{}
				ds.Href = "/edev/" + r.PathValue("id") + "/dstat"
				return ds
			},
		))
		mux.HandleFunc("PUT /edev/{id}/dstat", coresingleton.HandleSingletonGetPut[sep2.DeviceStatus](
			stores.DeviceStatuses,
			func(r *http.Request) string { return r.PathValue("id") },
			func(r *http.Request) sep2.DeviceStatus {
				ds := sep2.DeviceStatus{}
				ds.Href = "/edev/" + r.PathValue("id") + "/dstat"
				return ds
			},
		))
	}
	if stores.LogEvents != nil {
		mux.HandleFunc("GET /edev/{id}/log", scopedListHandler[sep2.LogEvent, sep2.LogEventList](
			stores.LogEvents, "id", corelogevent.BuildLogEventList, 900,
		))
		mux.HandleFunc("POST /edev/{id}/log", corelogevent.HandlePostLogEvent(stores.LogEvents))
	}
	if stores.PowerStatuses != nil {
		mux.HandleFunc("GET /edev/{id}/ps", corepowerstatus.HandlePowerStatus(stores.PowerStatuses))
		mux.HandleFunc("PUT /edev/{id}/ps", corepowerstatus.HandlePowerStatus(stores.PowerStatuses))
	}

	if stores.MessagingPrograms != nil {
		mux.HandleFunc("GET /msg", corelisthandler.ListHandler[sep2.MessagingProgram, sep2.MessagingProgramList](
			stores.MessagingPrograms, coremessaging.BuildMessagingProgramList, 900,
		))
		mux.HandleFunc("GET /msg/{msgId}", coremessaging.HandleMessagingProgram(stores.MessagingPrograms))
		mux.HandleFunc("GET /msg/{msgId}/tm", scopedListHandler[sep2.TextMessage, sep2.TextMessageList](
			stores.TextMessages, "msgId", coremessaging.BuildTextMessageList, 900,
		))
		mux.HandleFunc("POST /msg/{msgId}/tm", coremessaging.HandlePostTextMessage(stores.TextMessages))

		// The TextMessage instance (IEEECORE-081). The POST above returns this
		// href in a Location header and nothing served it, so a client that
		// followed the URI the server had just handed it got a 404.
		//
		// GET and HEAD are Mandatory (sep_wadl.xml:2851 and 2857). PUT and POST
		// are mode E and DELETE is mode D, and none is implemented here, so each
		// answers 405: with only a GET pattern registered http.ServeMux produces
		// that 405 itself and derives Allow from the registered method set, which
		// is why the empty itemMethods below cannot disagree with what is served.
		//
		// Scoped by {msgId}, named explicitly: the parent wildcard on this shape
		// is not called {id}, and reading an undeclared one would silently key
		// every message under the empty parent.
		mux.HandleFunc("GET /msg/{msgId}/tm/{tmId}",
			scopedResourceHandler[sep2.TextMessage](stores.TextMessages, "msgId", "tmId", itemMethods{}, nil))
	}

	if stores.FlowReservationRequests != nil {
		mux.HandleFunc("GET /edev/{id}/frq", scopedListHandler[sep2.FlowReservationRequest, sep2.FlowReservationRequestList](
			stores.FlowReservationRequests, "id", coreflowrsv.BuildFlowReservationRequestList, 900,
		))
		mux.HandleFunc("POST /edev/{id}/frq", coreflowrsv.HandlePostFlowReservationRequest(
			stores.FlowReservationRequests, stores.FlowReservationResponses,
		))
		mux.HandleFunc("GET /edev/{id}/frp", scopedListHandler[sep2.FlowReservationResponse, sep2.FlowReservationResponseList](
			stores.FlowReservationResponses, "id", coreflowrsv.BuildFlowReservationResponseList, 900,
		))

		// The two FlowReservation instances (IEEECORE-081). One POST mints both
		// hrefs: the Location header for the request it just created, and the
		// FlowReservationResponse href the client polls for the server's
		// decision. Neither was served, so a client that made a reservation
		// could not read the reservation back nor learn whether it was granted.
		//
		// GET and HEAD are Mandatory on both (sep_wadl.xml:3956 and 3962 for the
		// request, 4033 and 4039 for the response). Every other declared method
		// answers 405 rather than 404, from http.ServeMux, which derives Allow
		// from the registered method set.
		//
		// Read-only here, deliberately. PUT on FlowReservationRequest is mode M
		// (sep_wadl.xml:3963) and is NOT mounted: it is a write surface, and a
		// write with no ownership binding lets any authenticated device rewrite
		// another device's reservation. Ownership is IEEECORE-031's sweep, and
		// the missing Mandatory PUT is carried as a finding rather than mounted
		// dark here.
		frqInstance := scopedResourceHandler[sep2.FlowReservationRequest](
			stores.FlowReservationRequests, "id", "frqId", itemMethods{}, nil)
		mux.HandleFunc("GET /edev/{id}/frq/{frqId}", frqInstance)

		frpInstance := scopedResourceHandler[sep2.FlowReservationResponse](
			stores.FlowReservationResponses, "id", "frpId", itemMethods{}, nil)
		mux.HandleFunc("GET /edev/{id}/frp/{frpId}", frpInstance)
	}

	if stores.ResponseSets != nil {
		// Seed the default ResponseSet before the routes that serve it.
		//
		// Every DERControl this server emits carries a replyTo pointing into
		// this set (IEEECORE-067), so an unseeded set would leave that href
		// dangling: the POST route would answer, but GET /rsps would list
		// nothing and GET /rsps/{id} would 404, and a client cannot tell an
		// empty channel from a server that invented one. Seeding is
		// idempotent and yields to a set a consumer created under the same id.
		if err := coreresponse.SeedDefaultSet(context.Background(), stores.ResponseSets); err != nil {
			log.Printf("assembly: seeding the default ResponseSet: %v; replyTo hrefs will not resolve", err)
		}

		mux.HandleFunc("GET /rsps", corelisthandler.ListHandler[sep2.ResponseSet, sep2.ResponseSetList](
			stores.ResponseSets, coreflowrsv.BuildResponseSetList, 900,
		))
		mux.HandleFunc("GET /rsps/{rspsId}", coreresponse.HandleResponseSet(stores.ResponseSets))
		mux.HandleFunc("GET /rsps/{rspsId}/rsp", func(w http.ResponseWriter, r *http.Request) {
			rspsID := r.PathValue("rspsId")
			inner := stores.Responses.ForParent(rspsID)
			corelisthandler.ListHandler[sep2.Response, sep2.ResponseList](
				inner, coreflowrsv.BuildResponseList, 900,
			)(w, r)
		})
		mux.HandleFunc("POST /rsps/{rspsId}/rsp", coreflowrsv.HandlePostResponse(stores.Responses))
		mux.HandleFunc("GET /rsps/{rspsId}/rsp/{rspId}", coreresponse.HandleResponse(stores.Responses))
	}
}

// suppress unused import
var _ = paging.DefaultLimit
