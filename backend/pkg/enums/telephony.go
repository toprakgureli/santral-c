package enums

// Direction is the flow of a call relative to the contact center.
type Direction string

// Call directions.
const (
	DirectionInbound  Direction = "inbound"
	DirectionOutbound Direction = "outbound"
	DirectionInternal Direction = "internal"
)

// Disposition is the final outcome of a call.
type Disposition string

// Call dispositions.
const (
	DispositionInProgress Disposition = "in_progress"
	DispositionAnswered   Disposition = "answered"
	DispositionNoAnswer   Disposition = "no_answer"
	DispositionBusy       Disposition = "busy"
	DispositionFailed     Disposition = "failed"
	DispositionCanceled   Disposition = "canceled"
	DispositionVoicemail  Disposition = "voicemail"
)

// HangupBy identifies which side released a call.
type HangupBy string

// Hangup sources.
const (
	HangupByCaller HangupBy = "caller"
	HangupByCallee HangupBy = "callee"
	HangupBySystem HangupBy = "system"
)

// QualityLeg is the media leg a quality sample was taken from.
type QualityLeg string

// Media legs.
const (
	QualityLegAgent QualityLeg = "agent"
	QualityLegTrunk QualityLeg = "trunk"
)

// PhoneLabel classifies a contact phone number.
type PhoneLabel string

// Phone labels.
const (
	PhoneLabelMobile PhoneLabel = "mobile"
	PhoneLabelWork   PhoneLabel = "work"
	PhoneLabelHome   PhoneLabel = "home"
	PhoneLabelOther  PhoneLabel = "other"
)

// CallEventType names an entry in a call's event timeline.
type CallEventType string

// Call event types.
const (
	CallEventRinging      CallEventType = "ringing"
	CallEventAnswered     CallEventType = "answered"
	CallEventHold         CallEventType = "hold"
	CallEventUnhold       CallEventType = "unhold"
	CallEventTransfer     CallEventType = "transfer"
	CallEventDTMF         CallEventType = "dtmf"
	CallEventHangup       CallEventType = "hangup"
	CallEventRTPTimeout   CallEventType = "rtp_timeout"
	CallEventQualityAlert CallEventType = "quality_alert"
)
