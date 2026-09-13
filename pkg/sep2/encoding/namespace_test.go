package encoding_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2"
	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2/encoding"
)

func TestDetectNamespace2013(t *testing.T) {
	// IEEE 2030.5-2018 clause 5.7.2: level=-S1 identifies the 2018 base
	// schema, not the 2013 one. Only level=-S0 selects the 2013 namespace.
	req := httptest.NewRequest("GET", "/dcap", nil)
	req.Header.Set("Accept", "application/sep+xml; level=-S0")

	mode := encoding.DetectNamespace(req)
	if mode != encoding.Namespace2013 {
		t.Errorf("S0 header should detect 2013, got %d", mode)
	}
}

func TestDetectNamespace2018Default(t *testing.T) {
	req := httptest.NewRequest("GET", "/dcap", nil)

	mode := encoding.DetectNamespace(req)
	if mode != encoding.Namespace2018 {
		t.Errorf("no header should default to 2018, got %d", mode)
	}
}

func TestDetectNamespace2018Explicit(t *testing.T) {
	req := httptest.NewRequest("GET", "/dcap", nil)
	req.Header.Set("Accept", "application/sep+xml")

	mode := encoding.DetectNamespace(req)
	if mode != encoding.Namespace2018 {
		t.Errorf("plain accept should be 2018, got %d", mode)
	}
}

func TestDetectNamespaceLevel(t *testing.T) {
	tests := []struct {
		name   string
		accept string
		want   encoding.NamespaceMode
	}{
		{"level -S1 selects 2018", "application/sep+xml; level=-S1", encoding.Namespace2018},
		{"level +S1 selects 2018", "application/sep+xml; level=+S1", encoding.Namespace2018},
		{"level -S0 selects 2013", "application/sep+xml; level=-S0", encoding.Namespace2013},
		{"level +S0 selects 2013", "application/sep+xml; level=+S0", encoding.Namespace2013},
		{"no level defaults to 2018", "application/sep+xml", encoding.Namespace2018},
		{"no Accept header defaults to 2018", "", encoding.Namespace2018},
		{"sep-exi with level -S1 selects 2018", "application/sep-exi; level=-S1", encoding.Namespace2018},
		{"sep-exi with level -S0 selects 2013", "application/sep-exi; level=-S0", encoding.Namespace2013},
		{
			"multiple ranges with q values, conflicting S1 and S0 selects 2018",
			"application/sep+xml; level=-S1; q=0.1, application/sep-exi; level=-S0; q=0.9",
			encoding.Namespace2018,
		},
		{
			"S0 alone across multiple ranges with q values selects 2013",
			"text/plain; q=0.1, application/sep-exi; level=-S0; q=0.9",
			encoding.Namespace2013,
		},
		{
			"conflicting S0 and S1 in one header selects 2018",
			"application/sep+xml; level=-S1, application/sep-exi; level=-S0",
			encoding.Namespace2018,
		},
		{"malformed header defaults to 2018", "application/sep+xml; level", encoding.Namespace2018},
		{"malformed header with stray semicolon defaults to 2018", ";;;", encoding.Namespace2018},
		{"level -S2 selects 2018 (2023 base schema)", "application/sep+xml; level=-S2", encoding.Namespace2018},
		{"level +S2 selects 2018 (2023 base schema)", "application/sep+xml; level=+S2", encoding.Namespace2018},
		{"level -s0 lowercase selects 2013", "application/sep+xml; level=-s0", encoding.Namespace2013},
		{
			"lowercase s1 suppresses a conflicting S0 signal",
			"application/sep+xml; level=-S0, application/sep-exi; level=-s1",
			encoding.Namespace2018,
		},
		{"level on a non-sep media type is ignored", "text/html; level=-S0", encoding.Namespace2018},
		{"q=0 disables the range", "application/sep+xml; level=-S0; q=0", encoding.Namespace2018},
		{
			"quoted comma in a parameter value does not start a new range",
			`text/plain; q="0.5, application/sep+xml; level=-S0, x"`,
			encoding.Namespace2018,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest("GET", "/dcap", nil)
			if tt.accept != "" {
				req.Header.Set("Accept", tt.accept)
			}

			got := encoding.DetectNamespace(req)
			if got != tt.want {
				t.Errorf("DetectNamespace(%q) = %d, want %d", tt.accept, got, tt.want)
			}
		})
	}
}

func TestDetectNamespaceMultipleAcceptLines(t *testing.T) {
	// r.Header.Values returns every Accept field line; an S1 signal on a
	// later line must not be missed just because it was not the first.
	req := httptest.NewRequest("GET", "/dcap", nil)
	req.Header.Add("Accept", "application/sep+xml; level=-S0")
	req.Header.Add("Accept", "application/sep-exi; level=-S1")

	if got := encoding.DetectNamespace(req); got != encoding.Namespace2018 {
		t.Errorf("DetectNamespace across two Accept lines (S0 then S1) = %d, want Namespace2018 (%d)", got, encoding.Namespace2018)
	}
}

func TestDetectNamespaceBoundedRanges(t *testing.T) {
	// The parse bound (namespace.go's maxMediaRanges, 32) must stop an
	// oversized header from being fully parsed: a signal past the bound
	// must not affect the result. This test assumes the bound is under 40.
	var sb strings.Builder
	sb.WriteString("application/sep+xml; level=-S0")
	for i := 0; i < 40; i++ {
		sb.WriteString(", application/sep+xml; p=filler")
	}
	sb.WriteString(", application/sep+xml; level=-S1")

	req := httptest.NewRequest("GET", "/dcap", nil)
	req.Header.Set("Accept", sb.String())

	if got := encoding.DetectNamespace(req); got != encoding.Namespace2013 {
		t.Errorf("DetectNamespace with S1 past the range bound = %d, want Namespace2013 (%d)", got, encoding.Namespace2013)
	}
}

func BenchmarkDetectNamespaceManyRanges(b *testing.B) {
	// Reproduces the unbounded-parsing shape from review: a header with
	// many repeated media ranges, each carrying a level parameter.
	var sb strings.Builder
	for i := 0; i < 20000; i++ {
		sb.WriteString("application/sep+xml; level=-S1, ")
	}
	req := httptest.NewRequest("GET", "/dcap", nil)
	req.Header.Set("Accept", sb.String())

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		encoding.DetectNamespace(req)
	}
}

func TestRewriteNamespaceTo2013(t *testing.T) {
	data := []byte(`<DeviceCapability xmlns="urn:ieee:std:2030.5:ns" href="/dcap"/>`)
	rewritten := encoding.RewriteNamespace(data, encoding.Namespace2013)

	if !bytes.Contains(rewritten, []byte("http://ieee.org/2030.5")) {
		t.Errorf("should contain 2013 namespace, got: %s", rewritten)
	}
	if bytes.Contains(rewritten, []byte("urn:ieee:std:2030.5:ns")) {
		t.Error("should NOT contain 2018 namespace after rewrite")
	}
}

func TestRewriteNamespace2018NoOp(t *testing.T) {
	data := []byte(`<DeviceCapability xmlns="urn:ieee:std:2030.5:ns" href="/dcap"/>`)
	rewritten := encoding.RewriteNamespace(data, encoding.Namespace2018)

	if !bytes.Equal(data, rewritten) {
		t.Error("2018 mode should not modify data")
	}
}

func TestNamespaceMiddleware2013(t *testing.T) {
	// Handler that writes XML with 2018 namespace
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		encoding.WriteXML(w, 200, &sep2.Time{
			Resource:    sep2.Resource{Href: "/tm"},
			CurrentTime: 1000,
			Quality:     7,
		})
	})

	wrapped := encoding.NamespaceMiddleware(inner)

	req := httptest.NewRequest("GET", "/tm", nil)
	req.Header.Set("Accept", "application/sep+xml; level=-S0")
	w := httptest.NewRecorder()
	wrapped.ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "http://ieee.org/2030.5") {
		t.Errorf("2013 client should get 2013 namespace, got: %s", body)
	}
	if strings.Contains(body, "urn:ieee:std:2030.5:ns") {
		t.Errorf("2013 client should NOT get 2018 namespace")
	}
}

func TestNamespaceMiddleware2013NoExplicitWriteHeader(t *testing.T) {
	// Inner handler writes body without calling WriteHeader explicitly.
	// nsBufferedWriter.flush() must default w.status to 200 when it is zero,
	// and the body assertion below fails if buffering is skipped entirely,
	// since an unbuffered pass-through would leave the 2018 namespace as-is.
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Deliberately no WriteHeader call - exercises the w.status==0 default branch.
		if _, err := w.Write([]byte(`<T xmlns="urn:ieee:std:2030.5:ns"/>`)); err != nil {
			t.Errorf("inner Write: %v", err)
		}
	})

	wrapped := encoding.NamespaceMiddleware(inner)
	req := httptest.NewRequest("GET", "/tm", nil)
	req.Header.Set("Accept", "application/sep+xml; level=-S0")
	w := httptest.NewRecorder()
	wrapped.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("flush default status should be 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "http://ieee.org/2030.5") {
		t.Errorf("buffered 2013 response should contain the 2013 namespace, got: %s", body)
	}
	if strings.Contains(body, "urn:ieee:std:2030.5:ns") {
		t.Errorf("buffered 2013 response should NOT contain the 2018 namespace")
	}
}

func TestNamespaceMiddleware2018PassThrough(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		encoding.WriteXML(w, 200, &sep2.Time{
			Resource:    sep2.Resource{Href: "/tm"},
			CurrentTime: 1000,
			Quality:     7,
		})
	})

	wrapped := encoding.NamespaceMiddleware(inner)

	req := httptest.NewRequest("GET", "/tm", nil)
	w := httptest.NewRecorder()
	wrapped.ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "urn:ieee:std:2030.5:ns") {
		t.Errorf("2018 client should get 2018 namespace, got: %s", body)
	}
}

func TestNamespaceMiddlewareLevelS1Selects2018Body(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		encoding.WriteXML(w, 200, &sep2.Time{
			Resource:    sep2.Resource{Href: "/tm"},
			CurrentTime: 1000,
			Quality:     7,
		})
	})

	wrapped := encoding.NamespaceMiddleware(inner)

	req := httptest.NewRequest("GET", "/tm", nil)
	req.Header.Set("Accept", "application/sep+xml; level=-S1")
	w := httptest.NewRecorder()
	wrapped.ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "urn:ieee:std:2030.5:ns") {
		t.Errorf("level=-S1 client should get 2018 namespace, got: %s", body)
	}
	if strings.Contains(body, "http://ieee.org/2030.5") {
		t.Errorf("level=-S1 client should NOT get 2013 namespace")
	}
}

func TestNamespaceMiddlewareLevelPlusS1Selects2018Body(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		encoding.WriteXML(w, 200, &sep2.Time{
			Resource:    sep2.Resource{Href: "/tm"},
			CurrentTime: 1000,
			Quality:     7,
		})
	})

	wrapped := encoding.NamespaceMiddleware(inner)

	req := httptest.NewRequest("GET", "/tm", nil)
	req.Header.Set("Accept", "application/sep+xml; level=+S1")
	w := httptest.NewRecorder()
	wrapped.ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "urn:ieee:std:2030.5:ns") {
		t.Errorf("level=+S1 client should get 2018 namespace, got: %s", body)
	}
	if strings.Contains(body, "http://ieee.org/2030.5") {
		t.Errorf("level=+S1 client should NOT get 2013 namespace")
	}
}

func TestNamespaceMiddlewareNoLevelSelects2018Body(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		encoding.WriteXML(w, 200, &sep2.Time{
			Resource:    sep2.Resource{Href: "/tm"},
			CurrentTime: 1000,
			Quality:     7,
		})
	})

	wrapped := encoding.NamespaceMiddleware(inner)

	req := httptest.NewRequest("GET", "/tm", nil)
	req.Header.Set("Accept", "application/sep+xml")
	w := httptest.NewRecorder()
	wrapped.ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "urn:ieee:std:2030.5:ns") {
		t.Errorf("Accept without a level should get 2018 namespace, got: %s", body)
	}
}
