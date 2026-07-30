package metering

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2"
	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2/encoding"
	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/store"
	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/store/memory"
)

// LFDIProvider extracts a device LFDI from the request context.
// Returns the LFDI string and a boolean indicating whether an identity
// was present. Server passes auth.GetIdentity-derived closure; tests
// inject directly. The server-side auth package stays out of core by
// threading this callback through at construction time (same pattern
// as the subscription manager's NotificationObserver from Phase D1).
type LFDIProvider func(ctx context.Context) (lfdi string, ok bool)

// BuildMirrorUsagePointList constructs a MirrorUsagePointList from store results.
func BuildMirrorUsagePointList(href string, result store.ListResult[sep2.MirrorUsagePoint], pollRate uint32) sep2.MirrorUsagePointList {
	return sep2.MirrorUsagePointList{
		ListResource: sep2.ListResource{
			SubscribableResource: sep2.SubscribableResource{
				Resource: sep2.Resource{Href: href},
			},
			All:      result.All,
			Results:  result.Results,
			PollRate: pollRate,
		},
		MirrorUsagePoint: result.Items,
	}
}

// HandleCreateMirrorUsagePoint returns a handler for POST /mup.
// Inverters create MirrorUsagePoints to register for metering data reporting.
// lfdiProvider extracts the client LFDI from the request context; the server
// passes a closure over auth.GetIdentity so that the auth package does not
// become a dependency of core.
func HandleCreateMirrorUsagePoint(s store.ResourceStore[sep2.MirrorUsagePoint], lfdiProvider LFDIProvider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			encoding.MethodNotAllowed(w, "POST")
			return
		}

		lfdi, ok := lfdiProvider(r.Context())
		if !ok {
			http.Error(w, "identity required", http.StatusForbidden)
			return
		}

		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			http.Error(w, "read body failed", http.StatusBadRequest)
			return
		}

		var mup sep2.MirrorUsagePoint
		if err := xml.Unmarshal(body, &mup); err != nil {
			http.Error(w, "invalid XML: "+err.Error(), http.StatusBadRequest)
			return
		}

		// Set DeviceLFDI from cert (override client-supplied value)
		mup.DeviceLFDI = lfdi

		// Generate ID from MRID or timestamp
		id := mup.MRID
		if id == "" {
			id = fmt.Sprintf("mup-%d", time.Now().UnixNano())
		}
		mup.Href = "/mup/" + id
		// sep.xsd carries MirrorMeterReading inline on MirrorUsagePoint
		// (sep2.MirrorUsagePoint.MirrorMeterReading), not via a link
		// element; the schema has no MirrorMeterReadingListLink type at
		// all. The POST /mup/{id}/mr endpoint below remains the
		// out-of-band route clients use to add readings; its target
		// path is a fixed convention documented on MirrorUsagePoint,
		// not carried in the resource body.

		if err := s.Create(r.Context(), id, mup); err != nil {
			if errors.Is(err, store.ErrAlreadyExists) {
				// Race condition: another goroutine registered this MUP.
				// If the record was deleted between Create and Get, return 5xx
				// rather than a zero-value 200 (silent data loss).
				existing, getErr := s.Get(r.Context(), id)
				if getErr != nil {
					log.Printf("mup: race-loss after ErrAlreadyExists for id=%q: %v", id, getErr)
					http.Error(w, "registration race", http.StatusInternalServerError)
					return
				}
				w.Header().Set("Location", existing.Href)
				encoding.WriteXML(w, http.StatusOK, &existing)
				return
			}
			log.Printf("mup: create id=%q: %v (path=%s)", id, err, r.URL.Path)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Location", mup.Href)
		encoding.WriteXML(w, http.StatusCreated, &mup)
	}
}

// HandleMirrorUsagePoint returns a handler for GET /mup/{id}.
func HandleMirrorUsagePoint(s store.ResourceStore[sep2.MirrorUsagePoint]) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			encoding.MethodNotAllowed(w, "GET, HEAD")
			return
		}

		id := r.PathValue("id")
		mup, err := s.Get(r.Context(), id)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			log.Printf("mup: GET id=%q: %v (path=%s)", id, err, r.URL.Path)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		encoding.WriteXML(w, http.StatusOK, &mup)
	}
}

// HandlePostMirrorMeterReading returns a handler for POST /mup/{id}/mr.
// Inverters POST metering data (power, energy, etc.) to this endpoint.
func HandlePostMirrorMeterReading(
	mupStore store.ResourceStore[sep2.MirrorUsagePoint],
	mmrStore *memory.ScopedStore[sep2.MirrorMeterReading],
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			encoding.MethodNotAllowed(w, "POST")
			return
		}

		parentID := r.PathValue("id")

		// Verify parent exists
		if _, err := mupStore.Get(r.Context(), parentID); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				http.Error(w, "mirror usage point not found", http.StatusNotFound)
				return
			}
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			http.Error(w, "read body failed", http.StatusBadRequest)
			return
		}

		var mmr sep2.MirrorMeterReading
		if err := xml.Unmarshal(body, &mmr); err != nil {
			http.Error(w, "invalid XML: "+err.Error(), http.StatusBadRequest)
			return
		}

		// Generate time-based ID for sorted ordering
		id := fmt.Sprintf("%020d", time.Now().UnixNano())
		mmr.Href = fmt.Sprintf("/mup/%s/mr/%s", parentID, id)
		mmr.LastUpdateTime = time.Now().Unix()

		if err := mmrStore.Create(r.Context(), parentID, id, mmr); err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Location", mmr.Href)
		w.WriteHeader(http.StatusCreated)
	}
}
