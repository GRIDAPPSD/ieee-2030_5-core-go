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

	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2"
	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2/encoding"
	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/store"
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
//
// Its RETURN VALUE is no longer used to address the device: resource URLs
// now carry the opaque index allocated by EndDeviceIndexer (see below). It
// is still called, and its error still rejects the registration, because the
// IEEE-014 guard is a validity check on the SFDI itself and dropping the
// call would silently drop that check along with the addressing change.
type SFDIPrefixFunc func(sfdi string) (string, error)

// EndDeviceIndexer allocates the opaque, server-chosen index that identifies
// a device in resource URLs: the "3" in "/edev/3/rg". Declared here at the
// consumer; *memory.EndDeviceIndex is the production implementation.
//
// deviceKey is the most durable identity the caller has for the device. On
// this self-registration path the device is known only by its certificate,
// so the LFDI is the only key available. That means a device presenting a
// ROTATED certificate is an unknown key and receives a new index; surviving
// rotation requires out-of-band provisioning under a certificate-independent
// key, which this path by construction does not have. See the
// memory.EndDeviceIndex doc comment for the full discussion.
type EndDeviceIndexer interface {
	Allocate(deviceKey string) (string, error)
}

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
//
// It creates a new EndDevice, setting identity from the injected
// IdentityFunc, validating the SFDI through SFDIPrefixFunc (IEEE-014
// short-SFDI guard), and addressing the device by the opaque index allocated
// from idx.
//
// Identity and addressing are separate concerns here and must stay separate.
// The stored EndDevice keeps the certificate-derived LFDI and SFDI, which is
// what every ownership check compares against; the index only decides which
// URL the record is served under.
func HandleCreateEndDevice(s store.EndDeviceStore, idx EndDeviceIndexer, identity IdentityFunc, sfdiPrefix SFDIPrefixFunc) http.HandlerFunc {
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

		// IEEE-014 guard: reject a malformed or too-short SFDI before the
		// device is admitted. The returned prefix is intentionally discarded;
		// it used to be the device id, and addressing now comes from idx.
		if _, err := sfdiPrefix(sfdi); err != nil {
			log.Printf("edev create: %v", err)
			http.Error(w, "invalid device identity", http.StatusInternalServerError)
			return
		}

		// Address the device by an opaque server-chosen index. The LFDI is
		// the only device key this path has (the device is known solely by
		// its certificate), so a rotated certificate yields a new index here.
		id, err := idx.Allocate(lfdi)
		if err != nil {
			log.Printf("edev create: allocate index: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
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
