package logevent_test

import (
	"bytes"
	"context"
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2"
	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2srv/handlers/logevent"
	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/store"
	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/store/memory"
)

func TestHandlePostLogEvent_Created(t *testing.T) {
	t.Parallel()
	s := memory.NewScopedStore[sep2.LogEvent]()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /edev/{id}/log", logevent.HandlePostLogEvent(s))

	evt := sep2.LogEvent{
		LogEventCode: 1,
		LogEventID:   42,
	}
	body, _ := xml.Marshal(&evt)

	req := httptest.NewRequest(http.MethodPost, "/edev/dev1/log", bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body: %s", w.Code, w.Body.String())
	}

	loc := w.Header().Get("Location")
	if loc == "" {
		t.Error("Location header is empty, want /edev/dev1/log/<id>")
	}

	count, _ := s.Count(context.Background(), "dev1")
	if count != 1 {
		t.Errorf("stored count = %d, want 1", count)
	}
}

func TestHandlePostLogEvent_MethodNotAllowed(t *testing.T) {
	t.Parallel()
	s := memory.NewScopedStore[sep2.LogEvent]()
	h := logevent.HandlePostLogEvent(s)

	req := httptest.NewRequest(http.MethodGet, "/edev/dev1/log", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

func TestHandlePostLogEvent_BadXML(t *testing.T) {
	t.Parallel()
	s := memory.NewScopedStore[sep2.LogEvent]()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /edev/{id}/log", logevent.HandlePostLogEvent(s))

	req := httptest.NewRequest(http.MethodPost, "/edev/dev1/log", bytes.NewBufferString("not xml"))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestBuildLogEventList(t *testing.T) {
	t.Parallel()
	items := []sep2.LogEvent{
		{LogEventCode: 1},
		{LogEventCode: 2},
	}
	result := logevent.BuildLogEventList("/edev/1/log", store.ListResult[sep2.LogEvent]{
		Items:   items,
		All:     5,
		Results: uint32(len(items)),
	}, 900)

	if result.Href != "/edev/1/log" {
		t.Errorf("Href = %q, want /edev/1/log", result.Href)
	}
	if result.All != 5 {
		t.Errorf("All = %d, want 5", result.All)
	}
	if int(result.Results) != len(items) {
		t.Errorf("Results = %d, want %d", result.Results, len(items))
	}
	if len(result.LogEvent) != len(items) {
		t.Errorf("len(LogEvent) = %d, want %d", len(result.LogEvent), len(items))
	}
	if result.LogEvent[0].LogEventCode != 1 {
		t.Errorf("LogEvent[0].LogEventCode = %d, want 1", result.LogEvent[0].LogEventCode)
	}
}
