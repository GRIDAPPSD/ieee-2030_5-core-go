package encoding

import (
	"bytes"
	"context"
	"fmt"
	"mime"
	"net/http"
	"strings"

	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2"
)

type nsContextKey struct{}

// NamespaceMode identifies which IEEE 2030.5 XML namespace to use.
type NamespaceMode int

const (
	Namespace2018 NamespaceMode = iota // urn:ieee:std:2030.5:ns (2018/2023)
	Namespace2013                      // http://ieee.org/2030.5 (2013)
)

// DetectNamespace selects the response namespace from the request's Accept
// header. IEEE 2030.5-2018 clause 5.7.2 defines level=-S1 (or +S1) as the
// 2018 base schema; IEEE 2030.5-2013 defines level=-S0 (or +S0) as the 2013
// base schema. Only an explicit S0 level selects the legacy 2013 namespace:
// a missing level, an S1 level, or an unrecognized level all default to
// 2018. Each comma-separated media range is parsed on its own so a
// substring anywhere in the header (a q value, a media type) can never be
// mistaken for the level parameter. When a header carries both an S0 and an
// S1 signal, 2018 wins.
func DetectNamespace(r *http.Request) NamespaceMode {
	accept := r.Header.Get("Accept")
	if accept == "" {
		return Namespace2018
	}

	sawS1, sawS0 := false, false
	for _, part := range strings.Split(accept, ",") {
		_, params, err := mime.ParseMediaType(strings.TrimSpace(part))
		if err != nil {
			continue
		}
		switch params["level"] {
		case "-S1", "+S1":
			sawS1 = true
		case "-S0", "+S0":
			sawS0 = true
		}
	}

	if sawS0 && !sawS1 {
		return Namespace2013
	}
	return Namespace2018
}

// nsBufferedWriter buffers the response, rewrites namespace, then flushes.
type nsBufferedWriter struct {
	http.ResponseWriter
	buf     bytes.Buffer
	mode    NamespaceMode
	status  int
	headers http.Header
}

func (w *nsBufferedWriter) Header() http.Header {
	if w.headers == nil {
		w.headers = make(http.Header)
	}
	return w.headers
}

func (w *nsBufferedWriter) WriteHeader(statusCode int) {
	w.status = statusCode
}

func (w *nsBufferedWriter) Write(data []byte) (int, error) {
	return w.buf.Write(data)
}

func (w *nsBufferedWriter) flush() {
	data := w.buf.Bytes()
	data = RewriteNamespace(data, w.mode)

	// Copy buffered headers to real response (except Content-Length which may be wrong)
	realHeader := w.ResponseWriter.Header()
	for k, vals := range w.headers {
		if k == "Content-Length" {
			continue // will be set correctly below
		}
		for _, v := range vals {
			realHeader.Set(k, v)
		}
	}

	// Set correct Content-Length after namespace rewriting
	realHeader.Set("Content-Length", fmt.Sprintf("%d", len(data)))

	if w.status == 0 {
		w.status = 200
	}
	w.ResponseWriter.WriteHeader(w.status)
	_, _ = w.ResponseWriter.Write(data)
}

// NamespaceMiddleware detects the client's namespace preference and
// rewrites XML output to match. Supports 2013 and 2018/2023 namespaces.
func NamespaceMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mode := DetectNamespace(r)
		ctx := context.WithValue(r.Context(), nsContextKey{}, mode)

		if mode != Namespace2018 {
			bw := &nsBufferedWriter{ResponseWriter: w, mode: mode}
			next.ServeHTTP(bw, r.WithContext(ctx))
			bw.flush()
		} else {
			next.ServeHTTP(w, r.WithContext(ctx))
		}
	})
}

// GetNamespace retrieves the namespace mode from the context.
func GetNamespace(ctx context.Context) NamespaceMode {
	if mode, ok := ctx.Value(nsContextKey{}).(NamespaceMode); ok {
		return mode
	}
	return Namespace2018
}

// RewriteNamespace converts XML from 2018 namespace to 2013 if needed.
func RewriteNamespace(data []byte, mode NamespaceMode) []byte {
	if mode == Namespace2013 {
		return bytes.ReplaceAll(data,
			[]byte(sep2.Namespace),
			[]byte(sep2.Namespace2013))
	}
	return data
}
