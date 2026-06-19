package metering_test

import (
	"bytes"
	"context"
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"testing"

	"gitlab.pnnl.gov/arista/ieee-2030_5/ieee-2030_5-core/pkg/sep2"
	"gitlab.pnnl.gov/arista/ieee-2030_5/ieee-2030_5-core/pkg/sep2srv/handlers/metering"
	"gitlab.pnnl.gov/arista/ieee-2030_5/ieee-2030_5-core/pkg/store"
	"gitlab.pnnl.gov/arista/ieee-2030_5/ieee-2030_5-core/pkg/store/memory"
)

// --- BuildUsagePointList ---

func TestBuildUsagePointList(t *testing.T) {
	t.Parallel()
	items := []sep2.UsagePoint{{SubscribableResource: sep2.SubscribableResource{Resource: sep2.Resource{Href: "/upt/a"}}}}
	result := metering.BuildUsagePointList("/upt", store.ListResult[sep2.UsagePoint]{
		Items:   items,
		All:     3,
		Results: 1,
	}, 300)

	if result.Href != "/upt" {
		t.Errorf("Href = %q, want /upt", result.Href)
	}
	if result.All != 3 {
		t.Errorf("All = %d, want 3", result.All)
	}
	if len(result.UsagePoint) != 1 {
		t.Errorf("len(UsagePoint) = %d, want 1", len(result.UsagePoint))
	}
}

// --- HandleUsagePoint ---

func TestHandleUsagePoint_Get(t *testing.T) {
	t.Parallel()
	s := memory.NewStore[sep2.UsagePoint]()
	_ = s.Create(context.Background(), "upt1", sep2.UsagePoint{SubscribableResource: sep2.SubscribableResource{Resource: sep2.Resource{Href: "/upt/upt1"}}})

	mux := http.NewServeMux()
	mux.HandleFunc("GET /upt/{uptId}", metering.HandleUsagePoint(s))

	req := httptest.NewRequest(http.MethodGet, "/upt/upt1", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var upt sep2.UsagePoint
	_ = xml.Unmarshal(w.Body.Bytes(), &upt)
	if upt.Href != "/upt/upt1" {
		t.Errorf("Href = %q, want /upt/upt1", upt.Href)
	}
}

func TestHandleUsagePoint_NotFound(t *testing.T) {
	t.Parallel()
	s := memory.NewStore[sep2.UsagePoint]()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /upt/{uptId}", metering.HandleUsagePoint(s))

	req := httptest.NewRequest(http.MethodGet, "/upt/missing", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

// --- HandleCreateUsagePoint ---

func TestHandleCreateUsagePoint_Created(t *testing.T) {
	t.Parallel()
	s := memory.NewStore[sep2.UsagePoint]()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /upt", metering.HandleCreateUsagePoint(s))

	upt := sep2.UsagePoint{MRID: "UPT001"}
	body, _ := xml.Marshal(&upt)

	req := httptest.NewRequest(http.MethodPost, "/upt", bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body: %s", w.Code, w.Body.String())
	}

	loc := w.Header().Get("Location")
	if loc != "/upt/UPT001" {
		t.Errorf("Location = %q, want /upt/UPT001", loc)
	}

	var result sep2.UsagePoint
	if err := xml.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if result.Href != "/upt/UPT001" {
		t.Errorf("Href = %q, want /upt/UPT001", result.Href)
	}
	if result.MeterReadingListLink == nil {
		t.Fatal("MeterReadingListLink is nil, want set")
	}
	if result.MeterReadingListLink.Href != "/upt/UPT001/mr" {
		t.Errorf("MeterReadingListLink.Href = %q, want /upt/UPT001/mr", result.MeterReadingListLink.Href)
	}
}

func TestHandleCreateUsagePoint_MethodNotAllowed(t *testing.T) {
	t.Parallel()
	s := memory.NewStore[sep2.UsagePoint]()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /upt", metering.HandleCreateUsagePoint(s))

	req := httptest.NewRequest(http.MethodGet, "/upt", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

// --- HandleReadingType ---

func TestHandleReadingType_Get(t *testing.T) {
	t.Parallel()
	s := memory.NewStore[sep2.ReadingType]()
	uom := sep2.UomWatts
	_ = s.Create(context.Background(), "rt1", sep2.ReadingType{
		Resource: sep2.Resource{Href: "/rt/rt1"},
		Uom:      &uom,
	})

	mux := http.NewServeMux()
	mux.HandleFunc("GET /rt/{id}", metering.HandleReadingType(s))

	req := httptest.NewRequest(http.MethodGet, "/rt/rt1", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var rt sep2.ReadingType
	_ = xml.Unmarshal(w.Body.Bytes(), &rt)
	if rt.Href != "/rt/rt1" {
		t.Errorf("Href = %q, want /rt/rt1", rt.Href)
	}
}

// --- HandleCreateMirrorUsagePoint ---

func identityProvider(lfdi string) metering.LFDIProvider {
	return func(_ context.Context) (string, bool) {
		return lfdi, lfdi != ""
	}
}

func TestHandleCreateMirrorUsagePoint_Created(t *testing.T) {
	t.Parallel()
	s := memory.NewStore[sep2.MirrorUsagePoint]()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /mup", metering.HandleCreateMirrorUsagePoint(s, identityProvider("TEST_LFDI_ABCDEF")))

	mup := sep2.MirrorUsagePoint{
		MRID:        "INV001",
		Description: "Inverter 1",
	}
	body, _ := xml.Marshal(&mup)

	req := httptest.NewRequest(http.MethodPost, "/mup", bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body: %s", w.Code, w.Body.String())
	}

	loc := w.Header().Get("Location")
	if loc != "/mup/INV001" {
		t.Errorf("Location = %q, want /mup/INV001", loc)
	}

	var result sep2.MirrorUsagePoint
	_ = xml.Unmarshal(w.Body.Bytes(), &result)
	if result.DeviceLFDI != "TEST_LFDI_ABCDEF" {
		t.Errorf("DeviceLFDI = %q, want TEST_LFDI_ABCDEF (from provider)", result.DeviceLFDI)
	}
	if result.MirrorMeterReadingListLink == nil || result.MirrorMeterReadingListLink.Href != "/mup/INV001/mr" {
		t.Errorf("MirrorMeterReadingListLink.Href = %q, want /mup/INV001/mr",
			result.MirrorMeterReadingListLink.Href)
	}
}

func TestHandleCreateMirrorUsagePoint_NoIdentity(t *testing.T) {
	t.Parallel()
	s := memory.NewStore[sep2.MirrorUsagePoint]()
	noIdentity := metering.LFDIProvider(func(_ context.Context) (string, bool) {
		return "", false
	})

	mux := http.NewServeMux()
	mux.HandleFunc("POST /mup", metering.HandleCreateMirrorUsagePoint(s, noIdentity))

	req := httptest.NewRequest(http.MethodPost, "/mup", bytes.NewBufferString("<MirrorUsagePoint/>"))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", w.Code)
	}
}

// --- HandlePostMirrorMeterReading ---

func TestHandlePostMirrorMeterReading_Created(t *testing.T) {
	t.Parallel()
	mupStore := memory.NewStore[sep2.MirrorUsagePoint]()
	mmrStore := memory.NewScopedStore[sep2.MirrorMeterReading]()

	_ = mupStore.Create(context.Background(), "inv1", sep2.MirrorUsagePoint{
		Resource: sep2.Resource{Href: "/mup/inv1"},
		MRID:     "INV1",
	})

	val := int64(5000)
	uom := sep2.UomWatts
	mmr := sep2.MirrorMeterReading{
		MRID:        "MMR01",
		Description: "Active Power",
		ReadingType: &sep2.ReadingType{Uom: &uom},
		Reading:     &sep2.Reading{Value: &val},
	}
	body, _ := xml.Marshal(&mmr)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /mup/{id}/mr", metering.HandlePostMirrorMeterReading(mupStore, mmrStore))

	req := httptest.NewRequest(http.MethodPost, "/mup/inv1/mr", bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, body: %s", w.Code, w.Body.String())
	}

	count, _ := mmrStore.Count(context.Background(), "inv1")
	if count != 1 {
		t.Errorf("mirror meter readings count = %d, want 1", count)
	}
}

func TestHandlePostMirrorMeterReading_NotFoundParent(t *testing.T) {
	t.Parallel()
	mupStore := memory.NewStore[sep2.MirrorUsagePoint]()
	mmrStore := memory.NewScopedStore[sep2.MirrorMeterReading]()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /mup/{id}/mr", metering.HandlePostMirrorMeterReading(mupStore, mmrStore))

	req := httptest.NewRequest(http.MethodPost, "/mup/nonexistent/mr", bytes.NewBufferString("<MirrorMeterReading/>"))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

// --- BuildMirrorUsagePointList ---

func TestBuildMirrorUsagePointList(t *testing.T) {
	t.Parallel()
	items := []sep2.MirrorUsagePoint{
		{Resource: sep2.Resource{Href: "/mup/a"}, MRID: "A"},
		{Resource: sep2.Resource{Href: "/mup/b"}, MRID: "B"},
	}
	result := metering.BuildMirrorUsagePointList("/mup", store.ListResult[sep2.MirrorUsagePoint]{
		Items:   items,
		All:     2,
		Results: 2,
	}, 300)

	if result.Href != "/mup" {
		t.Errorf("Href = %q, want /mup", result.Href)
	}
	if result.All != 2 {
		t.Errorf("All = %d, want 2", result.All)
	}
	if len(result.MirrorUsagePoint) != 2 {
		t.Errorf("len(MirrorUsagePoint) = %d, want 2", len(result.MirrorUsagePoint))
	}
}
