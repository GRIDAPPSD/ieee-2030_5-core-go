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

// stampMirrorMeterReading assigns the server-owned fields on a
// MirrorMeterReading and returns the store id it derived them from.
//
// href and lastUpdateTime are the server's to assign, never the client's.
// MirrorMeterReading embeds Resource, so href arrives as a client-settable
// attribute, and lastUpdateTime is a plain client-writable element. /mup is
// not /edev-scoped, so no ownership middleware constrains what a client may
// claim there, and GET /mup echoes stored records back to every reader: a
// verbatim client href lets one device plant a path that points at another
// device's reading namespace, and a verbatim lastUpdateTime lets it forge the
// age of a reading others consume.
//
// Both the inline MirrorUsagePoint.MirrorMeterReading path and the
// out-of-band POST /mup/{id}/mr path route through here so the two endpoints
// cannot drift into minting different href shapes for the same resource kind.
//
// The zero-padded fixed-width nanosecond id is load-bearing rather than
// cosmetic: the store orders keys lexicographically, so fixed-width digits
// make lexical order match chronological order. lastUpdateTime is derived
// from the same instant as the id rather than from a second clock read, so a
// record's timestamp and its ordering key can never disagree.
func stampMirrorMeterReading(mmr *sep2.MirrorMeterReading, parentID string, nanos int64) string {
	id := fmt.Sprintf("%020d", nanos)
	mmr.Href = fmt.Sprintf("/mup/%s/mr/%s", parentID, id)
	mmr.LastUpdateTime = time.Unix(0, nanos).Unix()
	return id
}

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

		// Stamp the server-owned fields on every inline reading, matching
		// what HandlePostMirrorMeterReading below does for the out-of-band
		// route. Without this, a client's href and lastUpdateTime are stored
		// and re-served verbatim on GET /mup, which is unscoped.
		//
		// The per-element nanos offset comes from one clock read plus the
		// index rather than a fresh time.Now() per element: on a coarse
		// monotonic clock repeated reads inside a loop can return the same
		// nanosecond, which would mint colliding hrefs for distinct
		// readings. Distinct ids are required because they are the store's
		// ordering keys.
		baseNanos := time.Now().UnixNano()
		for i := range mup.MirrorMeterReading {
			stampMirrorMeterReading(&mup.MirrorMeterReading[i], id, baseNanos+int64(i))
		}
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

		// Generate time-based ID for sorted ordering, and override the
		// client-supplied href and lastUpdateTime. Shared with the inline
		// MirrorUsagePoint.MirrorMeterReading path so both mint the same
		// href shape.
		id := stampMirrorMeterReading(&mmr, parentID, time.Now().UnixNano())

		if err := mmrStore.Create(r.Context(), parentID, id, mmr); err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Location", mmr.Href)
		w.WriteHeader(http.StatusCreated)
	}
}
