// Package enddevice provides IEEE 2030.5 EndDevice resource handlers for
// GET/POST/PUT/DELETE /edev and /edev/{id}. Ported from the reference
// server's internal/handler/edev.go; auth touch points replaced by the
// injected AuthPolicy seam (IEEECORE-001 design section 4).
package enddevice

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"gitlab.pnnl.gov/arista/ieee-2030_5/ieee-2030_5-core/pkg/sep2"
	"gitlab.pnnl.gov/arista/ieee-2030_5/ieee-2030_5-core/pkg/sep2/encoding"
	"gitlab.pnnl.gov/arista/ieee-2030_5/ieee-2030_5-core/pkg/store"
)

// EndDeviceListHref is the resource href that EndDeviceList subscribers
// register against per IEEE 2030.5 section 11.1. Used by the DELETE handler
// to identify the subscription fan-out target when an EndDevice is removed
// (CSIP V1.2 MAINT-002 step 5).
const EndDeviceListHref = "/edev"

// ResourceNotifier dispatches subscription notifications for a resource
// href. It is the minimal surface HandleDeleteEndDevice needs from the
// subscription package and is defined here at the consumer (Pike rule:
// interfaces at the consumer, not at the producer). The production
// implementation is *subscription.Manager.
type ResourceNotifier interface {
	Notify(ctx context.Context, resourceHref string, status uint8)
}

// IdentityFunc extracts the authenticated device identity from the request
// context. Returns the LFDI, SFDI, and ok=true when identity is present.
// Replaces the direct auth.GetIdentity call in the original edev.go.
// The server wires auth.GetIdentity; tests supply a fixed identity.
type IdentityFunc func(ctx context.Context) (lfdi, sfdi string, ok bool)

// SFDIPrefixFunc derives the EndDevice id prefix from an SFDI string.
// Replaces auth.ExtractSFDIPrefix (IEEE-014 short-SFDI guard). The server
// wires auth.ExtractSFDIPrefix; tests supply a trivial truncation.
type SFDIPrefixFunc func(sfdi string) (string, error)

// BuildEndDeviceList constructs an EndDeviceList from store results.
func BuildEndDeviceList(href string, result store.ListResult[sep2.EndDevice], pollRate uint32) sep2.EndDeviceList {
	return sep2.EndDeviceList{
		ListResource: sep2.ListResource{
			SubscribableResource: sep2.SubscribableResource{
				Resource: sep2.Resource{Href: href},
			},
			All:      result.All,
			Results:  result.Results,
			PollRate: pollRate,
		},
		EndDevice: result.Items,
	}
}

// HandleEndDevice returns a handler for GET /edev/{id}.
func HandleEndDevice(s store.EndDeviceStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			encoding.MethodNotAllowed(w, "GET, HEAD")
			return
		}

		id := r.PathValue("id")
		if id == "" {
			http.Error(w, "device id required", http.StatusBadRequest)
			return
		}

		dev, err := s.Get(r.Context(), id)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			log.Printf("edev: GET id=%q: %v (path=%s)", id, err, r.URL.Path)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		encoding.WriteXML(w, http.StatusOK, &dev)
	}
}

// HandleCreateEndDevice returns a handler for POST /edev.
// It creates a new EndDevice, setting identity from the injected IdentityFunc
// and using SFDIPrefixFunc to derive the device-id prefix (IEEE-014 guard).
func HandleCreateEndDevice(s store.EndDeviceStore, identity IdentityFunc, sfdiPrefix SFDIPrefixFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			encoding.MethodNotAllowed(w, "POST")
			return
		}

		lfdi, sfdi, ok := identity(r.Context())
		if !ok {
			http.Error(w, "identity required", http.StatusForbidden)
			return
		}

		// Read body (optional: client may POST with minimal or empty body)
		var dev sep2.EndDevice
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			http.Error(w, "read body failed", http.StatusBadRequest)
			return
		}
		if len(body) > 0 {
			if err := xml.Unmarshal(body, &dev); err != nil {
				http.Error(w, "invalid XML: "+err.Error(), http.StatusBadRequest)
				return
			}
		}

		// Override identity from cert (never trust client-supplied SFDI/LFDI)
		dev.SFDI = sfdi
		dev.LFDI = lfdi
		dev.ChangedTime = time.Now().Unix()
		enabled := true
		dev.Enabled = &enabled

		// Check if already registered
		existing, err := s.GetBySFDI(r.Context(), sfdi)
		if err == nil {
			// Already exists: return 200 with existing device
			w.Header().Set("Location", existing.Href)
			encoding.WriteXML(w, http.StatusOK, &existing)
			return
		}

		// Generate ID and href: sfdiPrefix guards against short SFDI (IEEE-014).
		id, err := sfdiPrefix(sfdi)
		if err != nil {
			log.Printf("edev create: %v", err)
			http.Error(w, "invalid device identity", http.StatusInternalServerError)
			return
		}
		dev.Href = "/edev/" + id
		dev.RegistrationLink = &sep2.Link{Href: fmt.Sprintf("/edev/%s/rg", id)}
		dev.FunctionSetAssignmentsListLink = &sep2.ListLink{Href: fmt.Sprintf("/edev/%s/fsa", id)}

		if err := s.Create(r.Context(), id, dev); err != nil {
			if errors.Is(err, store.ErrAlreadyExists) {
				// Race condition: another goroutine registered this device.
				// If the record was deleted between Create and Get, return 5xx
				// rather than a zero-value 200 (silent data loss).
				existing, getErr := s.Get(r.Context(), id)
				if getErr != nil {
					log.Printf("edev: race-loss after ErrAlreadyExists for id=%q: %v", id, getErr)
					http.Error(w, "registration race", http.StatusInternalServerError)
					return
				}
				w.Header().Set("Location", existing.Href)
				encoding.WriteXML(w, http.StatusOK, &existing)
				return
			}
			log.Printf("edev: create id=%q: %v (path=%s)", id, err, r.URL.Path)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Location", dev.Href)
		encoding.WriteXML(w, http.StatusCreated, &dev)
	}
}

// HandleUpdateEndDevice returns a handler for PUT /edev/{id}.
func HandleUpdateEndDevice(s store.EndDeviceStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			encoding.MethodNotAllowed(w, "PUT")
			return
		}

		id := r.PathValue("id")
		if id == "" {
			http.Error(w, "device id required", http.StatusBadRequest)
			return
		}

		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			http.Error(w, "read body failed", http.StatusBadRequest)
			return
		}

		var dev sep2.EndDevice
		if err := xml.Unmarshal(body, &dev); err != nil {
			http.Error(w, "invalid XML: "+err.Error(), http.StatusBadRequest)
			return
		}

		dev.Href = "/edev/" + id
		dev.ChangedTime = time.Now().Unix()

		if err := s.Update(r.Context(), id, dev); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			log.Printf("edev: update id=%q: %v (path=%s)", id, err, r.URL.Path)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// HandleDeleteEndDevice returns a handler for DELETE /edev/{id}. CSIP V1.2
// MAINT-002 step 5 mandates that on successful deletion the server fires a
// Notification on the EndDeviceList subscription (SubscribedResource =
// "/edev") with NotificationStatusRemoved. The notifier is optional:
// passing nil disables notification fan-out (useful for tests that don't
// exercise the subscription path).
func HandleDeleteEndDevice(s store.EndDeviceStore, n ResourceNotifier) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			encoding.MethodNotAllowed(w, "DELETE")
			return
		}

		id := r.PathValue("id")
		if id == "" {
			http.Error(w, "device id required", http.StatusBadRequest)
			return
		}

		if err := s.Delete(r.Context(), id); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			log.Printf("edev: delete id=%q: %v", id, err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		// MAINT-002 step 5: fan out a removal notification to every
		// EndDeviceList subscriber. The Manager enqueues onto a bounded
		// queue and returns immediately, so this stays non-blocking.
		if n != nil {
			n.Notify(r.Context(), EndDeviceListHref, sep2.NotificationStatusRemoved)
		}

		w.WriteHeader(http.StatusNoContent)
	}
}
