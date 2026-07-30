package sep2

// EventStatus describes the current state of an event.
type EventStatus struct {
	CurrentStatus             uint8  `xml:"currentStatus"`
	DateTime                  int64  `xml:"dateTime"`
	PotentiallySuperseded     bool   `xml:"potentiallySuperseded"`
	PotentiallySupersededTime *int64 `xml:"potentiallySupersededTime,omitempty"`
	Reason                    string `xml:"reason,omitempty"`
}

// Copy returns an independent copy.
func (e EventStatus) Copy() EventStatus {
	c := e
	if e.PotentiallySupersededTime != nil {
		v := *e.PotentiallySupersededTime
		c.PotentiallySupersededTime = &v
	}
	return c
}

// Event is the base type for schedulable events.
//
// Spec reference: IEEE 2030.5 section 10.1.3 (Event rules). Event extends
// the XSD `RespondableResource` base, which contributes the `replyTo` and
// `responseRequired` child elements. Element order matches the 2023 XSD:
// `replyTo`, `responseRequired`, then identified-object fields (`mRID`,
// `description`), then `creationTime`, `EventStatus`, `interval`. Field
// declaration order in this struct controls `encoding/xml` marshal order:
// do not reorder without verifying the XSD.
type Event struct {
	SubscribableResource

	// ReplyTo is the URI a client POSTs its `Response` resource to when
	// acknowledging this event. Relative URIs resolve against the
	// server's base URL; absolute URIs are used verbatim. Spec: section
	// 10.1.3 and 2023 XSD `RespondableResource.replyTo` (HRef).
	ReplyTo string `xml:"replyTo,omitempty"`

	// ResponseRequired is a HexBinary8 bitmap selecting which transition
	// statuses require a Response POST from the client. Bits per
	// IEEE 2030.5 Table 32: bit 0 = message received, bit 1 = specific
	// response, bit 2 = response on transition. Spec: section 10.1.3 and
	// 2023 XSD `RespondableResource.responseRequired` (HexBinary8).
	ResponseRequired *uint8 `xml:"responseRequired,omitempty"`

	MRID        string `xml:"mRID,omitempty"`
	Description string `xml:"description,omitempty"`

	// CreationTime is the TimeType instant at which the server created
	// this event. It is a REQUIRED element: the 2023 XSD declares
	// `creationTime` minOccurs="1" maxOccurs="1" as the FIRST child
	// contributed by `Event` itself, before `EventStatus` and `interval`,
	// which is why it is declared here rather than appended (field order
	// drives `encoding/xml` marshal order).
	//
	// It is not decoration. A client uses creationTime to decide which of
	// two overlapping events of equal primacy supersedes the other: IEEE
	// 2030.5 defines a superseding event as a newer event from the same
	// program covering the same controls over an overlapping period, and
	// "newer" is creationTime. Concretely, the EPRI reference client's
	// block_supersede compares `x->creationTime > y->creationTime` when
	// primacies are equal, so with the element absent both events parse as
	// creationTime 0, neither can win, and the INCOMING event is the one
	// discarded. Omitting it therefore does not merely lose metadata: it
	// makes a server unable to replace a control it has already issued.
	//
	// No omitempty: zero is a legal-looking TimeType but a meaningless
	// creation instant, and the element is required, so a server that
	// leaves it unset should emit a visibly wrong `<creationTime>0</...>`
	// rather than a silently absent required element.
	CreationTime int64 `xml:"creationTime"`

	EventStatus *EventStatus      `xml:"EventStatus,omitempty"`
	Interval    *DateTimeInterval `xml:"interval,omitempty"`
}

// RandomizableEvent extends Event with randomization parameters.
type RandomizableEvent struct {
	Event
	RandomizeDuration *int32 `xml:"randomizeDuration,omitempty"`
	RandomizeStart    *int32 `xml:"randomizeStart,omitempty"`
}

// EventStatus current status values per spec.
const (
	EventStatusScheduled  uint8 = 0
	EventStatusActive     uint8 = 1
	EventStatusCancelled  uint8 = 2
	EventStatusSuperseded uint8 = 4
	EventStatusComplete   uint8 = 5
)
