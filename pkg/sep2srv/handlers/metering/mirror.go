package metering

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
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

// stripMirrorMeterReadings returns a copy of mup with MirrorMeterReading
// omitted, for serving on GET.
//
// IEEE 2030.5-2018 section 10.11.3 rule (c): a GET of the MirrorUsagePoint
// "SHALL return a resource with only the first level elements (i.e.,
// sub-elements and collections are not included)." MirrorMeterReading is
// minOccurs="0" maxOccurs="unbounded" on MirrorUsagePoint (sep.xsd:6472),
// so it is a collection, not a first-level scalar, and omitting it on
// GET is spec-mandated, not merely a workaround for the ReadingType/mRID
// defect. This is a serving-only change: storage is untouched, so a POST
// still stores whatever MirrorMeterReading children the client submitted.
//
// s.Get and s.List (see pkg/store/memory) already return independent
// copies, so mutating the returned value's slice field here does not
// affect the stored record.
func stripMirrorMeterReadings(mup sep2.MirrorUsagePoint) sep2.MirrorUsagePoint {
	mup.MirrorMeterReading = nil
	return mup
}

// authorizeMirrorOwner resolves the caller's certificate-derived LFDI and
// confirms the caller is the client that CREATED the MirrorUsagePoint at id.
//
// IEEE 2030.5-2018 section 10.11.3 rule (e): "The Metering Mirror server
// SHOULD only accept POSTs to a given MirrorUsagePoint from the client that
// created the mirror."
//
// The scope key is the CREATOR, not the device whose readings the mirror
// describes. HandleCreateMirrorUsagePoint stamps MirrorUsagePoint.DeviceLFDI
// from the caller's identity, overriding whatever the client claimed in the
// body, so the stored DeviceLFDI IS the creator's identity and comparing the
// caller against it implements rule (e) directly rather than by proxy.
//
// This composes with CSIP aggregators, which is the case a naive
// one-mirror-per-certificate rule would break. An aggregator acting for many
// DERs legitimately creates many mirrors; each is stamped with the
// aggregator's own LFDI at creation, so the aggregator retains access to all
// of them. Nothing here assumes a single mirror per certificate.
//
// The check lives in core rather than in a consumer's middleware because core
// owns protocol behavior: /mup is not /edev-scoped, so no path-shaped ACL rule
// a server writes can express "the creator of this record", which is a fact
// only the stored resource carries. An authorization rule enforced solely in a
// consumer is a rule core cannot guarantee, and that is exactly the shape that
// lets one route drift out of compliance with its sibling.
//
// Fails closed on every indeterminate case: a nil provider, no identity, an
// empty caller LFDI, or a stored record carrying no DeviceLFDI. A mirror with
// no recorded creator has nobody who can claim it, so it accepts no writes and
// serves no reads; treating an empty stored DeviceLFDI as "matches anyone"
// would be the unsafe fallback that converts a missing value into open access.
//
// Denial status is 403, not 404, on both the write and the read path:
//
//   - 403 is "authenticated but not authorized", which is precisely the
//     condition here, and it matches the ownership precedent already set by
//     registration.go for GET /edev/{id}/rg.
//   - 404 would be the choice if denial had to avoid confirming the resource
//     exists, but it buys nothing today: GET /mup is a server-wide list that
//     already enumerates every MirrorUsagePoint to every authenticated client
//     (deliberately, per 6.2.3.1). Masking existence on one route while the
//     adjacent list discloses it is theater. If the list is ever scoped per
//     client, revisit 404 for the read path at the same time so the two stay
//     consistent.
//
// On denial the response body is a fixed plain-text string. It never echoes
// the request body, the stored record, or either LFDI, so a probe learns
// nothing beyond "denied".
//
// On success the already-fetched record is returned so callers need not
// re-read the store.
func authorizeMirrorOwner(
	w http.ResponseWriter,
	r *http.Request,
	s store.ResourceStore[sep2.MirrorUsagePoint],
	lfdiProvider LFDIProvider,
	id string,
) (sep2.MirrorUsagePoint, bool) {
	var zero sep2.MirrorUsagePoint

	if lfdiProvider == nil {
		// A consumer that wired no identity source gets a closed door, not a
		// nil-func panic and not an open one.
		log.Printf("mup: nil LFDIProvider, denying (path=%s)", r.URL.Path)
		http.Error(w, "identity required", http.StatusForbidden)
		return zero, false
	}

	lfdi, ok := lfdiProvider(r.Context())
	if !ok || lfdi == "" {
		http.Error(w, "identity required", http.StatusForbidden)
		return zero, false
	}

	mup, err := s.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.Error(w, "mirror usage point not found", http.StatusNotFound)
			return zero, false
		}
		log.Printf("mup: ownership lookup id=%q: %v (path=%s)", id, err, r.URL.Path)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return zero, false
	}

	if mup.DeviceLFDI == "" || mup.DeviceLFDI != lfdi {
		http.Error(w, "forbidden", http.StatusForbidden)
		return zero, false
	}

	return mup, true
}

// BuildMirrorUsagePointList constructs a MirrorUsagePointList from store results.
func BuildMirrorUsagePointList(href string, result store.ListResult[sep2.MirrorUsagePoint], pollRate uint32) sep2.MirrorUsagePointList {
	items := make([]sep2.MirrorUsagePoint, len(result.Items))
	for i, mup := range result.Items {
		items[i] = stripMirrorMeterReadings(mup)
	}
	return sep2.MirrorUsagePointList{
		ListResource: sep2.ListResource{
			SubscribableResource: sep2.SubscribableResource{
				Resource: sep2.Resource{Href: href},
			},
			All:      result.All,
			Results:  result.Results,
			PollRate: pollRate,
		},
		MirrorUsagePoint: items,
	}
}

// mirrorIDBytes is the length in bytes of the derived MirrorUsagePoint id,
// rendered as 2*mirrorIDBytes uppercase hex characters. 16 bytes matches the
// width of a sep2 hexBinary128 mRID, so a derived id is indistinguishable in
// shape from the identifiers already carried on this resource, and 128 bits of
// a SHA-256 digest keeps accidental collision between two owners' ids beyond
// any realistic device population.
const mirrorIDBytes = 16

// MirrorStoreID derives the server-assigned MirrorUsagePoint id from the
// identity of the client that creates it and the mRID that client supplied.
//
// MirrorUsagePoint identity is per owning device, not global. The mRID on a
// POSTed MirrorUsagePoint is client-supplied and nothing coordinates it across
// devices: observed in the field, nine devices holding nine distinct
// certificates POSTed only three distinct mRIDs between them. Keying the store
// on that mRID alone made the second device to use a given mRID collide with
// the first device's record, and the create path answered the collision with
// 200 plus a Location pointing at the FIRST device's MirrorUsagePoint. The
// second device would then post its readings into another device's resource.
// That is an ownership-isolation defect independent of the rule (e) gate: the
// gate compares a caller against a stored DeviceLFDI, and on the create path
// the collision happens before any such comparison is reachable.
//
// Deriving rather than scoping the store is deliberate. /mup carries no owner
// path segment, so a URL must identify exactly one resource server-wide:
//
//   - GET /mup/{id} resolves from the path alone. If two owners shared a URL,
//     the same URL would mean different resources to different callers.
//   - The MirrorMeterReading ScopedStore is keyed by the {id} path segment, so
//     two owners sharing an id would share a readings namespace, which is the
//     original defect moved one level down rather than fixed.
//   - GET /mup is a deliberately server-wide list over one flat store
//     (section 6.2.3.1), which a per-owner ScopedStore cannot serve: it has no
//     cross-parent iteration.
//
// Hashing rather than concatenating owner and mRID also bounds the URI:
//
//   - The output is always 32 characters, so Location is 37 characters
//     whatever the client sends. A client-supplied mRID is unvalidated in
//     length and charset (sep2.MirrorUsagePoint.MRID is a plain string), so
//     using it verbatim let a client size and shape the URI we hand back: a
//     4000-character mRID produced a 4005-character Location, and an mRID of
//     "../../etc/passwd" produced a Location with path traversal in it. The
//     EPRI reference client parses Location into a 127-byte buffer with no
//     length guard and dereferences the result unguarded, so an over-long or
//     unparseable Location crashes it.
//   - The owner LFDI stays out of the URL. GET /mup hands every mirror's href
//     to every authenticated client, so a concatenated id would publish which
//     certificate created which mirror in the path itself.
//
// The digest input is length-prefixed rather than separator-joined so no pair
// of (owner, mRID) values can be framed two ways into the same digest,
// whatever characters either string contains.
//
// The derivation is deterministic, which is what preserves the idempotent
// re-POST behavior: the same device POSTing the same mRID lands on the same
// id, takes the ErrAlreadyExists branch, and is served its own record.
func MirrorStoreID(ownerLFDI, clientMRID string) string {
	h := sha256.New()
	_, _ = io.WriteString(h, strconv.Itoa(len(ownerLFDI)))
	_, _ = io.WriteString(h, ":")
	_, _ = io.WriteString(h, ownerLFDI)
	_, _ = io.WriteString(h, strconv.Itoa(len(clientMRID)))
	_, _ = io.WriteString(h, ":")
	_, _ = io.WriteString(h, clientMRID)
	sum := h.Sum(nil)
	return strings.ToUpper(hex.EncodeToString(sum[:mirrorIDBytes]))
}

// MirrorHref returns the canonical MirrorUsagePoint href for a store id.
// One function mints the href so the stored Resource.Href, the Location header
// on 201, and the Location header on the ErrAlreadyExists branch cannot drift
// into three different strings for one resource.
func MirrorHref(id string) string {
	return "/mup/" + id
}

// HandleCreateMirrorUsagePoint returns a handler for POST /mup.
// Inverters create MirrorUsagePoints to register for metering data reporting.
// lfdiProvider extracts the client LFDI from the request context; the server
// passes a closure over auth.GetIdentity so that the auth package does not
// become a dependency of core.
//
// The created resource's id is derived from the caller's certificate identity
// and its mRID together (see MirrorStoreID), so two devices POSTing the same
// mRID each get their own MirrorUsagePoint and neither is ever handed the
// other's Location.
func HandleCreateMirrorUsagePoint(s store.ResourceStore[sep2.MirrorUsagePoint], lfdiProvider LFDIProvider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			encoding.MethodNotAllowed(w, "POST")
			return
		}

		lfdi, ok := lfdiProvider(r.Context())
		if !ok || lfdi == "" {
			// An empty-but-present identity would key every such caller onto
			// one derived id, collapsing them back into the shared-record
			// defect this handler exists to prevent. Fail closed instead.
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

		// IEEE 2030.5-2018 section 10.11.3 rule (a)(1): the POST "SHALL
		// contain at least... the MirrorUsagePoint mRID". IdentifiedObject.mRID
		// (sep.xsd:5331) is minOccurs="1", inherited by MirrorUsagePoint via
		// UsagePointBase (sep.xsd:6472-6477, 6571-6576); the EPRI reference
		// client's own generated schema table (se_schema.c:428) carries
		// .min=1 on the MirrorUsagePoint mRID entry directly, not merely by
		// inheritance. A client that omits mRID has by definition given the
		// server nothing to dedupe future POSTs against (rule (a)(4) can
		// never be satisfied for it) and no identity it can later
		// re-address (rule (c) and the mandatory PUT/DELETE /mup/{id}
		// presume an addressable resource), so this is refused outright
		// rather than papered over with a synthetic per-request key.
		if mup.MRID == "" {
			http.Error(w, "MirrorUsagePoint mRID is required", http.StatusBadRequest)
			return
		}

		// Set DeviceLFDI from cert (override client-supplied value)
		mup.DeviceLFDI = lfdi

		// The resource identity is (creating device, client mRID).
		id := MirrorStoreID(lfdi, mup.MRID)
		mup.Href = MirrorHref(id)

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
				// The caller already holds a mirror under this mRID, or
				// another goroutine created it for the same caller between
				// this Create and the Get below. If the record was deleted in
				// that window, return 5xx rather than a zero-value 200
				// (silent data loss).
				existing, getErr := s.Get(r.Context(), id)
				if getErr != nil {
					log.Printf("mup: race-loss after ErrAlreadyExists for id=%q: %v", id, getErr)
					http.Error(w, "registration race", http.StatusInternalServerError)
					return
				}

				// Ownership is re-checked rather than inferred from the
				// derivation. Per-owner keying already means only this
				// caller's own record can occupy this id, but the store is
				// also writable by a consumer that seeds it directly, and
				// handing a caller a resource it does not own is precisely
				// the defect being fixed. Two independent barriers, not one.
				if existing.DeviceLFDI == "" || existing.DeviceLFDI != lfdi {
					log.Printf("mup: create collided with a record owned by another device (id=%q)", id)
					http.Error(w, "forbidden", http.StatusForbidden)
					return
				}

				// IEEE 2030.5-2018 section 10.11.3 rule (a)(4): "...the new
				// data SHALL be written over the existing MirrorUsagePoint."
				// mup already carries this POST's data with every
				// server-owned field stamped the same way the create path
				// stamps it: DeviceLFDI from the caller's certificate (not
				// the body, set above), Href derived from the same id this
				// (owner, mRID) pair always resolves to, and any inline
				// MirrorMeterReading elements already re-stamped with
				// fresh server-owned href/lastUpdateTime values. Persisting
				// mup as-is overwrites every other field verbatim from what
				// the client just sent, mRID and Description included.
				//
				// This is a full write-over, not a merge: an inline
				// MirrorMeterReading this POST omits is cleared from the
				// stored record, matching "written over" rather than
				// "append". Rule (a)(4) does not describe a partial-update
				// mode, and a merge would leave stale inline readings that
				// this POST deliberately dropped served back on the next
				// GET.
				//
				// The out-of-band POST /mup/{id}/mr route persists into
				// mmrStore, a separate collection this s.Update call never
				// touches. Rule (a)(4)'s "written over" language is scoped
				// to the MirrorUsagePoint resource itself, and neither
				// source cited for this fix speaks to the out-of-band
				// readings collection; rather than silently discard data
				// outside the rule's stated scope, this overwrite leaves
				// mmrStore untouched.
				if err := s.Update(r.Context(), id, mup); err != nil {
					log.Printf("mup: overwrite id=%q: %v (path=%s)", id, err, r.URL.Path)
					http.Error(w, "internal error", http.StatusInternalServerError)
					return
				}

				// Location is minted from the id just resolved, never
				// echoed from storage. An empty Location is not merely
				// wrong: the EPRI client takes strlen of it with no guard
				// and dereferences the NULL that its failed URI parse
				// returns.
				//
				// No representation is written on 204: the EPRI client's
				// se_receive (se_connection.c) schema-parses ANY response
				// body ahead of process_response regardless of status code,
				// so a 204 carrying content is both non-conformant and a
				// needless parse surface the reference client never reads.
				w.Header().Set("Location", mup.Href)
				w.WriteHeader(http.StatusNoContent)
				return
			}
			log.Printf("mup: create id=%q: %v (path=%s)", id, err, r.URL.Path)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		// IEEE 2030.5-2018 section 10.11.3 rule (a)(3) mandates only that
		// the Location header carry the new MirrorUsagePoint URI on 201;
		// it does not require a representation in the body, and the text
		// is identical in the 2018 and 2023 editions. The EPRI client's
		// process_response (retrieve.c) never reads a POST response body
		// via se_body(): for HTTP_POST it reads the Location header and
		// issues a fresh GET to fetch the created resource. Independent
		// of that, se_receive (se_connection.c) attempts to schema-parse
		// ANY response body before process_response is even reached, POST
		// included, so serving a body here re-creates a second parse
		// point the client does not use for anything. Dropping the body
		// removes that surface entirely rather than relying on item 1 and
		// item 2 to keep it well-formed forever.
		w.Header().Set("Location", mup.Href)
		w.WriteHeader(http.StatusCreated)
	}
}

// HandleMirrorUsagePoint returns a handler for GET /mup/{id}.
//
// Scoped to the creating client, same rule as the POST routes: see
// authorizeMirrorOwner. Rule (e) governs POSTs only, and the WADL marks this
// GET Optional (section 4.2 item (c) makes those modes normative), so scoping
// it costs nothing in conformance while metering readings are customer data
// that an unscoped GET hands to any authenticated peer.
//
// Rule (c) still holds on the owner's own record: the response carries only
// first-level elements, MirrorMeterReading children stripped.
func HandleMirrorUsagePoint(s store.ResourceStore[sep2.MirrorUsagePoint], lfdiProvider LFDIProvider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			encoding.MethodNotAllowed(w, "GET, HEAD")
			return
		}

		mup, ok := authorizeMirrorOwner(w, r, s, lfdiProvider, r.PathValue("id"))
		if !ok {
			return
		}

		stripped := stripMirrorMeterReadings(mup)
		encoding.WriteXML(w, http.StatusOK, &stripped)
	}
}

// HandlePostMirrorMeterReading returns a handler for POST /mup/{id}/mr and,
// mounted identically, POST /mup/{id}. Inverters POST metering data (power,
// energy, etc.) to this endpoint.
//
// Both routes share this one handler so the ownership gate, the href shape,
// and the server-owned field stamping cannot drift between them. Enforcing
// rule (e) on one route and not its sibling is the failure shape this
// arrangement exists to prevent.
//
// Ownership: the caller's LFDI must equal the parent MirrorUsagePoint's stored
// DeviceLFDI, which is the LFDI of the client that created the mirror. See
// authorizeMirrorOwner for the rule, the CSIP aggregator case, and the choice
// of 403 over 404.
func HandlePostMirrorMeterReading(
	mupStore store.ResourceStore[sep2.MirrorUsagePoint],
	mmrStore *memory.ScopedStore[sep2.MirrorMeterReading],
	lfdiProvider LFDIProvider,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			encoding.MethodNotAllowed(w, "POST")
			return
		}

		parentID := r.PathValue("id")

		// Ownership gate: resolves identity, confirms the parent exists, and
		// confirms the caller created it. Runs before the body is read so an
		// unauthorized caller's payload is never parsed, let alone stored.
		if _, ok := authorizeMirrorOwner(w, r, mupStore, lfdiProvider, parentID); !ok {
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
