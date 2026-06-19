package sep2time

import (
	"net/http"
	"time"

	"gitlab.pnnl.gov/arista/ieee-2030_5/ieee-2030_5-core/pkg/sep2"
	"gitlab.pnnl.gov/arista/ieee-2030_5/ieee-2030_5-core/pkg/sep2/encoding"
)

// nowFunc is the wall-clock source for the Time resource. It defaults to
// time.Now in all production builds and is bit-identical to a direct
// time.Now() call. The sibling file time_test_hook.go (built only with
// the csip_test_hooks build tag) reassigns nowFunc at init() to add a
// test-controlled offset, enabling CSIP V1.2 CORE-006 (TM_TIME_ADJUSTED).
// See IEEE-024 / IEEE-025.
var nowFunc = time.Now

// TimeParams holds the time-resource configuration values supplied by the
// server at construction. Callers build this from their config struct; the
// handler closes over it so core never imports the server-side config
// package (callback-injection pattern from Phase D3).
type TimeParams struct {
	TZOffset    int32
	DSTOffset   int32
	DSTStart    int64
	DSTEnd      int64
	TimeQuality uint8
}

// HandleTime returns a handler for GET /tm.
// Time is a mandatory function set per spec section 9.2.
func HandleTime(params TimeParams) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			encoding.MethodNotAllowed(w, "GET, HEAD")
			return
		}

		now := nowFunc()
		tm := sep2.Time{
			Resource:     sep2.Resource{Href: "/tm"},
			CurrentTime:  now.Unix(),
			DstEndTime:   params.DSTEnd,
			DstOffset:    params.DSTOffset,
			DstStartTime: params.DSTStart,
			Quality:      params.TimeQuality,
			TzOffset:     params.TZOffset,
		}

		encoding.WriteXML(w, http.StatusOK, &tm)
	}
}
