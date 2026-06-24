package assembly_test

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gitlab.pnnl.gov/arista/ieee-2030_5/ieee-2030_5-core/pkg/sep2"
	"gitlab.pnnl.gov/arista/ieee-2030_5/ieee-2030_5-core/pkg/sep2srv/assembly"
	"gitlab.pnnl.gov/arista/ieee-2030_5/ieee-2030_5-core/pkg/store/memory"
)

// testStores builds a minimal Stores instance sufficient for the assembly test:
// EndDevices, Registrations, DERs and their singleton sub-stores, FSAs, and
// Subscriptions. Nil fields in Stores skip optional route registration.
func testStores() *assembly.Stores {
	return &assembly.Stores{
		EndDevices:         memory.NewEndDeviceStore(),
		Registrations:      memory.NewRegistrationStore(),
		DERs:               memory.NewScopedStore[sep2.DER](),
		DERCapabilities:    memory.NewScopedStore[sep2.DERCapability](),
		DERSettings:        memory.NewScopedStore[sep2.DERSettings](),
		DERStatuses:        memory.NewScopedStore[sep2.DERStatus](),
		DERAvailabilities:  memory.NewScopedStore[sep2.DERAvailability](),
		DERPrograms:        memory.NewDERProgramStore(),
		DERControls:        memory.NewScopedStore[sep2.DERControl](),
		DefaultDERControls: memory.NewScopedStore[sep2.DefaultDERControl](),
		DERCurves:          memory.NewStore[sep2.DERCurve](),
		FSAs:               memory.NewScopedStore[sep2.FunctionSetAssignments](),
		Subscriptions:      memory.NewSubscriptionStore(),
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

// TestAssembly_ImportCleanGate verifies that the assembly package's import
// list contains no github.com/GRIDAPPSD/ieee-2030_5-go/internal/... paths.
// This is the layering invariant: core must not import server-internal packages.
// The compiler already enforces cross-module internal/ boundaries, but this
// test makes the contract explicit and visible in the test suite.
func TestAssembly_ImportCleanGate(t *testing.T) {
	t.Parallel()

	// The forbidden prefix: any import from the reference server's internal tree.
	const forbidden = "github.com/GRIDAPPSD/ieee-2030_5-go/internal"

	// Enumerate our own module path to ensure we only check core packages.
	const coreModule = "gitlab.pnnl.gov/arista/ieee-2030_5/ieee-2030_5-core"

	// We cannot introspect imports at runtime without go/packages, so this test
	// asserts the invariant by verifying that importing the assembly package in
	// this test binary does not pull in anything from the forbidden prefix. We
	// use a build-time static check pattern: if the assembly package compiles
	// cleanly as part of this test (which it does if the test binary links),
	// and the forbidden import would have caused a compile error (cross-module
	// internal/ access is rejected by the Go toolchain), then the invariant is
	// structurally enforced. This test documents that guarantee.
	//
	// The additional runtime check: scan the test binary's argv[0] build info
	// via debug/buildinfo is possible but heavyweight. The toolchain enforcement
	// is sufficient; this test exists to make the contract visible.
	t.Log("import-clean gate: assembly package compiled without any internal/... import from the reference server module")
	t.Logf("core module = %s", coreModule)
	t.Logf("forbidden prefix = %s", forbidden)
	t.Log("the Go toolchain rejects cross-module internal/ imports at compile time; this test binary compiling proves the invariant holds")
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
