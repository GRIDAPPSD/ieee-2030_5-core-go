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
package assembly

import (
	"context"
	"errors"
	"fmt"
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
	// Registrations is the persistent-aware wrapper around the in-memory
	// Store[sep2.Registration]. The embedded *Store gives back-compat
	// method promotion (Get/List/Count) for call sites that don't need
	// the persistence flush.
	Registrations       *memory.RegistrationStore
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
		registerMirrorRoutes(protocolMux, stores, authPolicy)
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

	for _, prefix := range []string{
		"/dcap", "/tm", "/sdev", "/sdev/",
		"/edev", "/edev/",
		"/mup", "/mup/",
		"/dc", "/dc/",
		"/upt", "/upt/", "/rt", "/rt/",
		"/msg", "/msg/",
		"/rsps", "/rsps/",
	} {
		top.Handle(prefix, protocolChain)
	}

	return encoding.NamespaceMiddleware(top), protocolMux.Patterns()
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

func registerEndDeviceRoutes(mux routeRegistrar, stores *Stores, authPolicy AuthPolicy, notifier ResourceNotifier) {
	mux.HandleFunc("GET /edev", corelisthandler.ListHandler[sep2.EndDevice, sep2.EndDeviceList](
		stores.EndDevices, coreedev.BuildEndDeviceList, 900,
	))
	mux.HandleFunc("POST /edev", coreedev.HandleCreateEndDevice(stores.EndDevices, authPolicy.Identity, authPolicy.SFDIPrefix))
	mux.HandleFunc("GET /edev/{id}", coreedev.HandleEndDevice(stores.EndDevices))
	mux.HandleFunc("PUT /edev/{id}", coreedev.HandleUpdateEndDevice(stores.EndDevices))
	mux.HandleFunc("DELETE /edev/{id}", coreedev.HandleDeleteEndDevice(stores.EndDevices, notifier))

	// IEEE-101: Registration GET handler at /edev/{id}/rg.
	if stores.Registrations != nil {
		mux.HandleFunc("GET /edev/{id}/rg", corereg.HandleGetRegistration(stores.EndDevices, stores.Registrations, authPolicy.Identity))
	}

	// FSA endpoints
	if stores.FSAs != nil {
		mux.HandleFunc("GET /edev/{id}/fsa", scopedListHandler[sep2.FunctionSetAssignments, sep2.FunctionSetAssignmentsList](
			stores.FSAs, corefsa.BuildFSAList, 900,
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

func registerMirrorRoutes(mux routeRegistrar, stores *Stores, authPolicy AuthPolicy) {
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
	mux.HandleFunc("POST /mup", coremetering.HandleCreateMirrorUsagePoint(stores.MirrorUsagePoints, lfdiProvider))
	mux.HandleFunc("GET /mup/{id}", coremetering.HandleMirrorUsagePoint(stores.MirrorUsagePoints))
	mux.HandleFunc("POST /mup/{id}/mr", coremetering.HandlePostMirrorMeterReading(
		stores.MirrorUsagePoints, stores.MirrorMeterReadings,
	))
}

func registerDERRoutes(mux routeRegistrar, stores *Stores) {
	if stores.DERs == nil {
		return
	}

	dercap, derg, ders, dera := coreder.DERSingletonHandlers(
		stores.DERCapabilities, stores.DERSettings, stores.DERStatuses, stores.DERAvailabilities,
	)

	mux.HandleFunc("GET /edev/{id}/der", scopedListHandler[sep2.DER, sep2.DERList](
		stores.DERs, coreder.BuildDERList, 900,
	))
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
		stores.DERPrograms.ScopedStore, coreder.BuildDERProgramList, 900,
	))

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
	mux.HandleFunc("GET /edev/{id}/fsa/{fsaId}/derp/{derpId}/derc/{dercId}",
		scopedResourceHandlerDeep[sep2.DERControl](stores.DERControls, "dercId"))

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

// scopedListHandler creates a list handler that scopes by the {id} path value.
func scopedListHandler[T store.Copier[T], L any](
	scopedStore *memory.ScopedStore[T],
	buildList func(href string, result store.ListResult[T], pollRate uint32) L,
	pollRate uint32,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		parentID := r.PathValue("id")
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
// repeated in each. The key is what confines a resource to the device path it
// was stored beneath: a caller supplying another device's {id} produces a
// different parent store, so the lookup misses rather than resolving a
// resource it does not own. Duplicating the expression would let the two
// routes drift, and a single-resource route that scoped more loosely than its
// list would be a cross-device read.
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
func scopedResourceHandlerDeep[T store.Copier[T]](
	scopedStore *memory.ScopedStore[T],
	idParam string,
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

		encoding.WriteXML(w, http.StatusOK, &resource)
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
		stores.MeterReadings, coremetering.BuildMeterReadingList, 900,
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
			stores.LogEvents, corelogevent.BuildLogEventList, 900,
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
			stores.TextMessages, coremessaging.BuildTextMessageList, 900,
		))
		mux.HandleFunc("POST /msg/{msgId}/tm", coremessaging.HandlePostTextMessage(stores.TextMessages))
	}

	if stores.FlowReservationRequests != nil {
		mux.HandleFunc("GET /edev/{id}/frq", scopedListHandler[sep2.FlowReservationRequest, sep2.FlowReservationRequestList](
			stores.FlowReservationRequests, coreflowrsv.BuildFlowReservationRequestList, 900,
		))
		mux.HandleFunc("POST /edev/{id}/frq", coreflowrsv.HandlePostFlowReservationRequest(
			stores.FlowReservationRequests, stores.FlowReservationResponses,
		))
		mux.HandleFunc("GET /edev/{id}/frp", scopedListHandler[sep2.FlowReservationResponse, sep2.FlowReservationResponseList](
			stores.FlowReservationResponses, coreflowrsv.BuildFlowReservationResponseList, 900,
		))
	}

	if stores.ResponseSets != nil {
		mux.HandleFunc("GET /rsps", corelisthandler.ListHandler[sep2.ResponseSet, sep2.ResponseSetList](
			stores.ResponseSets, coreflowrsv.BuildResponseSetList, 900,
		))
		mux.HandleFunc("GET /rsps/{rspsId}/rsp", func(w http.ResponseWriter, r *http.Request) {
			rspsID := r.PathValue("rspsId")
			inner := stores.Responses.ForParent(rspsID)
			corelisthandler.ListHandler[sep2.Response, sep2.ResponseList](
				inner, coreflowrsv.BuildResponseList, 900,
			)(w, r)
		})
		mux.HandleFunc("POST /rsps/{rspsId}/rsp", coreflowrsv.HandlePostResponse(stores.Responses))
	}
}

// suppress unused import
var _ = paging.DefaultLimit
