package assembly_test

import (
	"context"
	"debug/buildinfo"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"gitlab.pnnl.gov/arista/ieee-2030_5/ieee-2030_5-core/pkg/sep2"
	"gitlab.pnnl.gov/arista/ieee-2030_5/ieee-2030_5-core/pkg/sep2srv/assembly"
	"gitlab.pnnl.gov/arista/ieee-2030_5/ieee-2030_5-core/pkg/store/memory"
)

// testStores builds a fully-populated Stores instance for assembly tests.
// Populated stores cause all optional route-registration branches to fire,
// driving coverage on registerMirrorRoutes, registerMeteringRoutes, and
// registerNewFunctionSetRoutes. Fields that are nil skip the branch;
// we populate all of them to maximise coverage.
func testStores() *assembly.Stores {
	return &assembly.Stores{
		EndDevices:         memory.NewEndDeviceStore(),
		Registrations:      memory.NewRegistrationStore(),
		MirrorUsagePoints:  memory.NewStore[sep2.MirrorUsagePoint](),
		MirrorMeterReadings: memory.NewScopedStore[sep2.MirrorMeterReading](),

		DERs:               memory.NewScopedStore[sep2.DER](),
		DERCapabilities:    memory.NewScopedStore[sep2.DERCapability](),
		DERSettings:        memory.NewScopedStore[sep2.DERSettings](),
		DERStatuses:        memory.NewScopedStore[sep2.DERStatus](),
		DERAvailabilities:  memory.NewScopedStore[sep2.DERAvailability](),
		DERPrograms:        memory.NewDERProgramStore(),
		DERControls:        memory.NewScopedStore[sep2.DERControl](),
		DefaultDERControls: memory.NewScopedStore[sep2.DefaultDERControl](),
		DERCurves:          memory.NewStore[sep2.DERCurve](),

		FSAs:          memory.NewScopedStore[sep2.FunctionSetAssignments](),
		Subscriptions: memory.NewSubscriptionStore(),

		// Server-side metering
		UsagePoints:   memory.NewStore[sep2.UsagePoint](),
		MeterReadings: memory.NewScopedStore[sep2.MeterReading](),
		Readings:      memory.NewScopedStore[sep2.Reading](),
		ReadingTypes:  memory.NewStore[sep2.ReadingType](),

		// New function sets
		Configurations:           memory.NewScopedStore[sep2.Configuration](),
		DeviceStatuses:           memory.NewScopedStore[sep2.DeviceStatus](),
		LogEvents:                memory.NewScopedStore[sep2.LogEvent](),
		PowerStatuses:            memory.NewScopedStore[sep2.PowerStatus](),
		MessagingPrograms:        memory.NewStore[sep2.MessagingProgram](),
		TextMessages:             memory.NewScopedStore[sep2.TextMessage](),
		FlowReservationRequests:  memory.NewScopedStore[sep2.FlowReservationRequest](),
		FlowReservationResponses: memory.NewScopedStore[sep2.FlowReservationResponse](),
		ResponseSets:             memory.NewStore[sep2.ResponseSet](),
		Responses:                memory.NewScopedStore[sep2.Response](),
	}
}

// testAuthPolicy returns a pass-through AuthPolicy suitable for tests:
//   - Wrap is a no-op (no TLS required).
//   - Identity always returns the fixed test identity (ok=true).
//   - SFDIPrefix truncates to 8 chars (mirrors auth.ExtractSFDIPrefix).
const testLFDI = "AABBCCDDEEFF001122334455667788990011223344556677"
const testSFDI = "AABBCCDD11223344"

func testAuthPolicy() assembly.AuthPolicy {
	return assembly.AuthPolicy{
		Wrap: func(h http.Handler) http.Handler { return h },
		Identity: func(_ context.Context) (lfdi, sfdi string, ok bool) {
			return testLFDI, testSFDI, true
		},
		SFDIPrefix: func(sfdi string) (string, error) {
			if len(sfdi) < 8 {
				return "", fmt.Errorf("SFDI too short: %q", sfdi)
			}
			return sfdi[:8], nil
		},
	}
}

// decodeXML unmarshals the response body into dst and fails the test on error.
func decodeXML(t *testing.T, resp *http.Response, dst any) {
	t.Helper()
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if err := xml.Unmarshal(body, dst); err != nil {
		t.Fatalf("decode XML (status %d): %v\nbody: %s", resp.StatusCode, err, body)
	}
}

// TestAssembly_DCAPWired asserts that GET /dcap returns a correctly-wired
// DeviceCapability with required links (data-invariants Rule 1: assert field
// values, not just 200).
func TestAssembly_DCAPWired(t *testing.T) {
	t.Parallel()
	handler, _ := assembly.BuildProtocolRouter(
		assembly.RouterConfig{TZOffset: -28800},
		testStores(),
		testAuthPolicy(),
		"serverSFDI", "serverLFDI",
		nil,
	)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/dcap")
	if err != nil {
		t.Fatalf("GET /dcap: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("GET /dcap: want 200, got %d", resp.StatusCode)
	}

	var dcap sep2.DeviceCapability
	decodeXML(t, resp, &dcap)

	if dcap.Href != "/dcap" {
		t.Errorf("dcap.Href = %q, want %q", dcap.Href, "/dcap")
	}
	if dcap.TimeLink == nil {
		t.Fatal("dcap.TimeLink is nil")
	}
	if dcap.TimeLink.Href != "/tm" {
		t.Errorf("dcap.TimeLink.Href = %q, want %q", dcap.TimeLink.Href, "/tm")
	}
	if dcap.EndDeviceListLink == nil {
		t.Fatal("dcap.EndDeviceListLink is nil")
	}
	if dcap.EndDeviceListLink.Href != "/edev" {
		t.Errorf("dcap.EndDeviceListLink.Href = %q, want %q", dcap.EndDeviceListLink.Href, "/edev")
	}
	if dcap.MirrorUsagePointListLink == nil {
		t.Fatal("dcap.MirrorUsagePointListLink is nil")
	}
}

// TestAssembly_EndDeviceCreateRoundTrip asserts that POST /edev creates an
// EndDevice and GET /edev returns a list containing it with the test SFDI
// (data-invariants Rule 1: assert stored field values).
func TestAssembly_EndDeviceCreateRoundTrip(t *testing.T) {
	t.Parallel()
	handler, _ := assembly.BuildProtocolRouter(
		assembly.RouterConfig{},
		testStores(),
		testAuthPolicy(),
		"serverSFDI", "serverLFDI",
		nil,
	)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	// POST /edev: create a device. The SEP2 XML namespace is required by the
	// EndDevice struct's XMLName tag; a namespace-less body returns 400.
	postBody := `<EndDevice xmlns="urn:ieee:std:2030.5:ns"/>`
	resp, err := http.Post(srv.URL+"/edev", "application/sep+xml", strings.NewReader(postBody))
	if err != nil {
		t.Fatalf("POST /edev: %v", err)
	}
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("POST /edev: want 201/200, got %d: %s", resp.StatusCode, body)
	}

	var created sep2.EndDevice
	decodeXML(t, resp, &created)
	if created.SFDI != testSFDI {
		t.Errorf("created.SFDI = %q, want %q", created.SFDI, testSFDI)
	}
	if created.LFDI != testLFDI {
		t.Errorf("created.LFDI = %q, want %q", created.LFDI, testLFDI)
	}
	if created.Href == "" {
		t.Error("created.Href is empty")
	}

	// GET /edev: list must contain the created device
	resp2, err := http.Get(srv.URL + "/edev")
	if err != nil {
		t.Fatalf("GET /edev: %v", err)
	}
	if resp2.StatusCode != http.StatusOK {
		resp2.Body.Close()
		t.Fatalf("GET /edev: want 200, got %d", resp2.StatusCode)
	}

	var list sep2.EndDeviceList
	decodeXML(t, resp2, &list)

	if list.All < 1 {
		t.Errorf("EndDeviceList.All = %d, want >= 1", list.All)
	}
	found := false
	for _, dev := range list.EndDevice {
		if dev.SFDI == testSFDI {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("EndDeviceList does not contain a device with SFDI %q", testSFDI)
	}
}

// TestAssembly_DERSingletonRoundTrip asserts that PUT then GET on
// /edev/{id}/der/{derId}/dercap returns a DERCapability whose
// RTGMaxW.Value == 10000 (data-invariants Rule 1: exact stored field).
func TestAssembly_DERSingletonRoundTrip(t *testing.T) {
	t.Parallel()
	stores := testStores()
	handler, _ := assembly.BuildProtocolRouter(
		assembly.RouterConfig{},
		stores,
		testAuthPolicy(),
		"serverSFDI", "serverLFDI",
		nil,
	)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	// First create an EndDevice so the edev ID exists in the path.
	// Include the SEP2 namespace: the EndDevice XMLName tag requires it.
	resp, err := http.Post(srv.URL+"/edev", "application/sep+xml",
		strings.NewReader(`<EndDevice xmlns="urn:ieee:std:2030.5:ns"/>`))
	if err != nil {
		t.Fatalf("POST /edev: %v", err)
	}
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("POST /edev: want 201/200, got %d: %s", resp.StatusCode, body)
	}
	var edev sep2.EndDevice
	decodeXML(t, resp, &edev)

	// Extract the edev ID from the Href: "/edev/<id>"
	edevID := strings.TrimPrefix(edev.Href, "/edev/")
	if edevID == "" || edevID == edev.Href {
		t.Fatalf("cannot extract edev ID from Href %q", edev.Href)
	}

	const derID = "1"
	dercapURL := fmt.Sprintf("%s/edev/%s/der/%s/dercap", srv.URL, edevID, derID)

	// PUT DERCapability with RTGMaxW.Value = 10000. Include the SEP2 namespace.
	putBody := `<DERCapability xmlns="urn:ieee:std:2030.5:ns"><rtgMaxW><multiplier>0</multiplier><value>10000</value></rtgMaxW></DERCapability>`
	req, _ := http.NewRequest(http.MethodPut, dercapURL, strings.NewReader(putBody))
	req.Header.Set("Content-Type", "application/sep+xml")
	putResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT %s: %v", dercapURL, err)
	}
	putResp.Body.Close()
	if putResp.StatusCode != http.StatusNoContent && putResp.StatusCode != http.StatusOK {
		t.Fatalf("PUT dercap: want 204/200, got %d", putResp.StatusCode)
	}

	// GET DERCapability: assert RTGMaxW.Value == 10000
	getResp, err := http.Get(dercapURL)
	if err != nil {
		t.Fatalf("GET %s: %v", dercapURL, err)
	}
	if getResp.StatusCode != http.StatusOK {
		getResp.Body.Close()
		t.Fatalf("GET dercap: want 200, got %d", getResp.StatusCode)
	}

	var cap sep2.DERCapability
	decodeXML(t, getResp, &cap)

	if cap.RTGMaxW == nil {
		t.Fatal("DERCapability.RTGMaxW is nil after PUT")
	}
	if cap.RTGMaxW.Value != 10000 {
		t.Errorf("DERCapability.RTGMaxW.Value = %d, want 10000", cap.RTGMaxW.Value)
	}
}

// TestAssembly_TimeScalarsFlowThroughRouterConfig asserts that GET /tm returns
// a sep2.Time whose TzOffset matches the RouterConfig.TZOffset passed in
// (data-invariants Rule 1: assert the config field propagated, not just 200).
func TestAssembly_TimeScalarsFlowThroughRouterConfig(t *testing.T) {
	t.Parallel()
	const wantOffset int32 = -28800 // PST: UTC-8 in seconds

	handler, _ := assembly.BuildProtocolRouter(
		assembly.RouterConfig{TZOffset: wantOffset},
		nil, // no stores: static-only routes still work
		testAuthPolicy(),
		"serverSFDI", "serverLFDI",
		nil,
	)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/tm")
	if err != nil {
		t.Fatalf("GET /tm: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("GET /tm: want 200, got %d", resp.StatusCode)
	}

	var tm sep2.Time
	decodeXML(t, resp, &tm)

	if tm.TzOffset != wantOffset {
		t.Errorf("Time.TzOffset = %d, want %d", tm.TzOffset, wantOffset)
	}
}

// TestAssembly_ImportCleanGate verifies that the assembly package (and all
// packages it transitively imports) contains no import of
// github.com/GRIDAPPSD/ieee-2030_5-go/internal/... paths.
//
// This is the layering invariant: core must not import server-internal
// packages. The Go toolchain enforces cross-module internal/ at compile time,
// so this test binary linking at all already proves the invariant holds. The
// test adds a runtime assertion via debug/buildinfo that scans the actual
// dependency list in the compiled binary, turning the guarantee from
// "implicit and invisible" to "explicit and tested" (Pike LOW finding).
func TestAssembly_ImportCleanGate(t *testing.T) {
	t.Parallel()

	const forbidden = "github.com/GRIDAPPSD/ieee-2030_5-go/internal"

	// Read the build-info embedded in the current test binary. os.Args[0] is
	// the test binary; debug/buildinfo.ReadFile inspects the Go build metadata
	// section without forking or network access.
	info, err := buildinfo.ReadFile(os.Args[0])
	if err != nil {
		// Reproducible case: -trimpath strips the binary or the binary is
		// a stripped static build without build info. The compile-time
		// enforcement still holds; skip rather than fail so CI is not
		// broken by stripped binaries.
		t.Skipf("debug/buildinfo.ReadFile(%q): %v (skipping runtime check; compile-time enforcement still applies)", os.Args[0], err)
	}

	for _, dep := range info.Deps {
		if strings.HasPrefix(dep.Path, forbidden) {
			t.Errorf("forbidden import in test binary: dep.Path = %q (prefix %q)", dep.Path, forbidden)
		}
	}
}

// notifyRemoverStub satisfies both coreedev.ResourceNotifier and the internal
// notifyRemover interface so TestAssembly_AsNotifyRemoved can exercise the
// asNotifyRemoved type-assertion path in registerEndDeviceRoutes.
// The stub counts Notify calls and records the last NotifyRemoved call.
type notifyRemoverStub struct {
	notifyCalled       int
	notifyRemovedCalls []string // resourceHref values
}

func (s *notifyRemoverStub) Notify(_ context.Context, resourceHref string, _ uint8) {
	s.notifyCalled++
}

func (s *notifyRemoverStub) NotifyRemoved(_ context.Context, sub sep2.Subscription) error {
	s.notifyRemovedCalls = append(s.notifyRemovedCalls, sub.Href)
	return nil
}

// TestAssembly_ScopedListRoutesMounted: exercises the scopedListHandler and
// scopedListHandlerDeep closures by sending HTTP GETs to routes that are
// registered through those helpers (FSA list, DER list, DERControl list).
// These routes are hit at the HTTP level so the closure body is exercised.
func TestAssembly_ScopedListRoutesMounted(t *testing.T) {
	t.Parallel()

	handler, _ := assembly.BuildProtocolRouter(
		assembly.RouterConfig{},
		testStores(),
		testAuthPolicy(),
		"serverSFDI", "serverLFDI",
		nil,
	)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	// GET /edev/{id}/fsa (scoped by device id)
	resp, err := http.Get(srv.URL + "/edev/e1/fsa")
	if err != nil {
		t.Fatalf("GET /edev/e1/fsa: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET /edev/e1/fsa status = %d, want 200", resp.StatusCode)
	}

	// GET /edev/{id}/der (scoped list)
	resp2, err := http.Get(srv.URL + "/edev/e1/der")
	if err != nil {
		t.Fatalf("GET /edev/e1/der: %v", err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Errorf("GET /edev/e1/der status = %d, want 200", resp2.StatusCode)
	}

	// GET /edev/{id}/fsa/{fsaId}/derp/{derpId}/derc (scopedListHandlerDeep)
	resp3, err := http.Get(srv.URL + "/edev/e1/fsa/f1/derp/p1/derc")
	if err != nil {
		t.Fatalf("GET /edev/e1/fsa/f1/derp/p1/derc: %v", err)
	}
	resp3.Body.Close()
	if resp3.StatusCode != http.StatusOK {
		t.Errorf("GET /edev/e1/fsa/f1/derp/p1/derc status = %d, want 200", resp3.StatusCode)
	}

	// GET /mup (mirror usage point list — exercises registerMirrorRoutes)
	resp4, err := http.Get(srv.URL + "/mup")
	if err != nil {
		t.Fatalf("GET /mup: %v", err)
	}
	resp4.Body.Close()
	if resp4.StatusCode != http.StatusOK {
		t.Errorf("GET /mup status = %d, want 200", resp4.StatusCode)
	}

	// GET /upt (usage point list — exercises registerMeteringRoutes)
	resp5, err := http.Get(srv.URL + "/upt")
	if err != nil {
		t.Fatalf("GET /upt: %v", err)
	}
	resp5.Body.Close()
	if resp5.StatusCode != http.StatusOK {
		t.Errorf("GET /upt status = %d, want 200", resp5.StatusCode)
	}
}

// TestAssembly_AsNotifyRemoved: a notifier that also satisfies notifyRemover
// wires the subscription-delete handler with the NotifyRemoved callback.
// Asserts the DELETE /edev/{id}/sub/{subId} path is mounted and returns 204.
func TestAssembly_AsNotifyRemoved(t *testing.T) {
	t.Parallel()

	stores := testStores()
	stub := &notifyRemoverStub{}
	handler, patterns := assembly.BuildProtocolRouter(
		assembly.RouterConfig{},
		stores,
		testAuthPolicy(),
		"serverSFDI", "serverLFDI",
		stub,
	)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	// Verify the subscription-delete pattern is registered.
	found := false
	for _, p := range patterns {
		if p == "DELETE /edev/{id}/sub/{subId}" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("DELETE /edev/{id}/sub/{subId} not in pattern list; patterns = %v", patterns)
	}

	// Seed a subscription so the DELETE path has something to act on.
	ctx := context.Background()
	sub := sep2.Subscription{
		SubscribableResource: sep2.SubscribableResource{
			Resource: sep2.Resource{Href: "/edev/e1/sub/s1"},
		},
		SubscribedResource: "/edev",
	}
	if err := stores.Subscriptions.Create(ctx, "s1", sub); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}

	req, _ := http.NewRequestWithContext(ctx, http.MethodDelete, srv.URL+"/edev/e1/sub/s1", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE /edev/e1/sub/s1: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("DELETE /edev/e1/sub/s1 status = %d, want 204", resp.StatusCode)
	}
}

// TestAssembly_PatternListNonEmpty asserts that BuildProtocolRouter returns
// a non-empty, sorted pattern list (boot-logging invariant).
func TestAssembly_PatternListNonEmpty(t *testing.T) {
	t.Parallel()
	_, patterns := assembly.BuildProtocolRouter(
		assembly.RouterConfig{},
		testStores(),
		testAuthPolicy(),
		"serverSFDI", "serverLFDI",
		nil,
	)

	if len(patterns) == 0 {
		t.Fatal("BuildProtocolRouter returned empty pattern list")
	}

	// Assert sorted (the recordingMux.Patterns contract)
	for i := 1; i < len(patterns); i++ {
		if patterns[i] < patterns[i-1] {
			t.Errorf("pattern list not sorted at index %d: %q < %q", i, patterns[i], patterns[i-1])
		}
	}

	// Assert /edev and /tm are present
	found := map[string]bool{}
	for _, p := range patterns {
		found[p] = true
	}
	for _, want := range []string{"GET /dcap", "GET /tm", "GET /edev", "POST /edev"} {
		if !found[want] {
			t.Errorf("pattern list missing %q; got: %v", want, patterns)
		}
	}
}
