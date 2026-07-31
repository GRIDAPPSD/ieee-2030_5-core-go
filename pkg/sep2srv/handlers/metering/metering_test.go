package metering_test

import (
	"bytes"
	"context"
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2"
	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2srv/handlers/metering"
	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/store"
	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/store/memory"
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
	// sep.xsd defines no MirrorMeterReadingListLink element on
	// MirrorUsagePoint (confirmed absent from the schema's full element
	// index); the served bytes must never carry one.
	if strings.Contains(w.Body.String(), "MirrorMeterReadingListLink") {
		t.Errorf("served MirrorUsagePoint carries undefined MirrorMeterReadingListLink; body=%s", w.Body.String())
	}
}

// TestHandleCreateMirrorUsagePoint_InlineReadingsAreServerStamped asserts that
// the inline MirrorMeterReading slice on a POSTed MirrorUsagePoint gets the same
// server-side overrides the out-of-band POST /mup/{id}/mr path applies.
//
// Each inline element embeds Resource, so href is client-supplied, and
// lastUpdateTime is a plain client-writable field. /mup is NOT /edev-scoped, so
// no ownership middleware runs on it, and GET /mup echoes the whole list back to
// every reader. Storing either field verbatim therefore lets one device plant a
// chosen href and a forged reading timestamp onto records other devices read.
// The sibling endpoint HandlePostMirrorMeterReading overwrites both
// (mirror.go: mmr.Href and mmr.LastUpdateTime); the inline path must not be the
// asymmetric hole that bypasses them.
func TestHandleCreateMirrorUsagePoint_InlineReadingsAreServerStamped(t *testing.T) {
	t.Parallel()
	s := memory.NewStore[sep2.MirrorUsagePoint]()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /mup", metering.HandleCreateMirrorUsagePoint(s, identityProvider("TEST_LFDI_ABCDEF")))

	const forgedHref = "/mup/VICTIM/mr/00000000000000000001"
	const forgedTime = int64(1)
	valA, valB := int64(1000), int64(2000)
	mup := sep2.MirrorUsagePoint{
		MRID:       "INV001",
		DeviceLFDI: "CLIENT_SUPPLIED_LFDI",
		MirrorMeterReading: []sep2.MirrorMeterReading{
			{
				Resource:       sep2.Resource{Href: forgedHref},
				MRID:           "MMR_A",
				LastUpdateTime: forgedTime,
				Reading:        &sep2.Reading{Value: &valA},
			},
			{
				Resource:       sep2.Resource{Href: forgedHref},
				MRID:           "MMR_B",
				LastUpdateTime: forgedTime,
				Reading:        &sep2.Reading{Value: &valB},
			},
		},
	}
	body, err := xml.Marshal(&mup)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	before := time.Now().Unix()
	req := httptest.NewRequest(http.MethodPost, "/mup", bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	after := time.Now().Unix()

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body: %s", w.Code, w.Body.String())
	}

	stored, err := s.Get(context.Background(), "INV001")
	if err != nil {
		t.Fatalf("get stored MirrorUsagePoint: %v", err)
	}
	if stored.DeviceLFDI != "TEST_LFDI_ABCDEF" {
		t.Errorf("stored DeviceLFDI = %q, want TEST_LFDI_ABCDEF (cert override)", stored.DeviceLFDI)
	}
	if len(stored.MirrorMeterReading) != 2 {
		t.Fatalf("stored MirrorMeterReading count = %d, want 2", len(stored.MirrorMeterReading))
	}

	seenHrefs := make(map[string]bool, len(stored.MirrorMeterReading))
	for i, got := range stored.MirrorMeterReading {
		if got.Href == forgedHref {
			t.Errorf("reading %d: stored Href = %q, client-supplied href persisted verbatim", i, got.Href)
		}
		// Match the shape HandlePostMirrorMeterReading mints:
		// "/mup/{parentID}/mr/{20-digit zero-padded id}".
		prefix := "/mup/INV001/mr/"
		if !strings.HasPrefix(got.Href, prefix) {
			t.Errorf("reading %d: stored Href = %q, want prefix %q", i, got.Href, prefix)
		} else {
			id := strings.TrimPrefix(got.Href, prefix)
			if len(id) != 20 {
				t.Errorf("reading %d: href id segment %q has length %d, want 20 (%%020d nanos, matching the POST /mup/{id}/mr path)", i, id, len(id))
			}
			for _, c := range id {
				if c < '0' || c > '9' {
					t.Errorf("reading %d: href id segment %q is not all digits", i, id)
					break
				}
			}
		}
		if seenHrefs[got.Href] {
			t.Errorf("reading %d: stored Href %q collides with an earlier reading; ids must be distinct for sorted ordering", i, got.Href)
		}
		seenHrefs[got.Href] = true

		if got.LastUpdateTime == forgedTime {
			t.Errorf("reading %d: stored LastUpdateTime = %d, forged client value persisted verbatim", i, got.LastUpdateTime)
		}
		if got.LastUpdateTime < before || got.LastUpdateTime > after {
			t.Errorf("reading %d: stored LastUpdateTime = %d, want server clock in [%d, %d]", i, got.LastUpdateTime, before, after)
		}
	}

	// Client-owned payload must survive: the fix overrides the two
	// server-owned fields, it does not discard the reading itself.
	if stored.MirrorMeterReading[0].MRID != "MMR_A" || stored.MirrorMeterReading[1].MRID != "MMR_B" {
		t.Errorf("inline reading mRIDs = %q, %q; want MMR_A, MMR_B preserved in order",
			stored.MirrorMeterReading[0].MRID, stored.MirrorMeterReading[1].MRID)
	}
	if stored.MirrorMeterReading[0].Reading == nil || *stored.MirrorMeterReading[0].Reading.Value != valA {
		t.Errorf("inline reading 0 value not preserved: %+v", stored.MirrorMeterReading[0].Reading)
	}
	if stored.MirrorMeterReading[1].Reading == nil || *stored.MirrorMeterReading[1].Reading.Value != valB {
		t.Errorf("inline reading 1 value not preserved: %+v", stored.MirrorMeterReading[1].Reading)
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
	const forgedHref = "/mup/VICTIM/mr/00000000000000000001"
	const forgedTime = int64(1)
	mmr := sep2.MirrorMeterReading{
		Resource:       sep2.Resource{Href: forgedHref},
		MRID:           "MMR01",
		Description:    "Active Power",
		LastUpdateTime: forgedTime,
		ReadingType:    &sep2.ReadingType{Uom: &uom},
		Reading:        &sep2.Reading{Value: &val},
	}
	body, _ := xml.Marshal(&mmr)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /mup/{id}/mr", metering.HandlePostMirrorMeterReading(mupStore, mmrStore))

	before := time.Now().Unix()
	req := httptest.NewRequest(http.MethodPost, "/mup/inv1/mr", bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	after := time.Now().Unix()

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, body: %s", w.Code, w.Body.String())
	}

	count, _ := mmrStore.Count(context.Background(), "inv1")
	if count != 1 {
		t.Fatalf("mirror meter readings count = %d, want 1", count)
	}

	// The Location header names the id the server minted, which is also the
	// store key. Assert on the STORED record: the response body here is empty.
	loc := w.Header().Get("Location")
	prefix := "/mup/inv1/mr/"
	if !strings.HasPrefix(loc, prefix) {
		t.Fatalf("Location = %q, want prefix %q", loc, prefix)
	}
	id := strings.TrimPrefix(loc, prefix)
	if len(id) != 20 {
		t.Errorf("minted id %q has length %d, want 20", id, len(id))
	}

	stored, err := mmrStore.Get(context.Background(), "inv1", id)
	if err != nil {
		t.Fatalf("get stored MirrorMeterReading %q: %v", id, err)
	}
	if stored.Href != loc {
		t.Errorf("stored Href = %q, want %q (server-synthesized, matching Location)", stored.Href, loc)
	}
	if stored.Href == forgedHref {
		t.Errorf("stored Href = %q, client-supplied href persisted verbatim", stored.Href)
	}
	if stored.LastUpdateTime == forgedTime {
		t.Errorf("stored LastUpdateTime = %d, forged client value persisted verbatim", stored.LastUpdateTime)
	}
	if stored.LastUpdateTime < before || stored.LastUpdateTime > after {
		t.Errorf("stored LastUpdateTime = %d, want server clock in [%d, %d]", stored.LastUpdateTime, before, after)
	}
	if stored.MRID != "MMR01" {
		t.Errorf("stored MRID = %q, want MMR01 preserved", stored.MRID)
	}
	if stored.Reading == nil || stored.Reading.Value == nil || *stored.Reading.Value != val {
		t.Errorf("stored Reading value not preserved: %+v", stored.Reading)
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
