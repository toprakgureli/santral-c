package responses

import (
	"time"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
)

// CallEvent is one entry in a call's timeline.
type CallEvent struct {
	ID      uint      `json:"id"`
	Seq     int       `json:"seq"`
	Type    string    `json:"type"`
	Channel string    `json:"channel,omitempty"`
	At      time.Time `json:"at"`
	Detail  string    `json:"detail"`
}

// CallQuality is one RTCP sample of a call.
type CallQuality struct {
	ID       uint      `json:"id"`
	Leg      string    `json:"leg"`
	Codec    string    `json:"codec,omitempty"`
	At       time.Time `json:"at"`
	JitterMs *float64  `json:"jitterMs,omitempty"`
	RTTMs    *float64  `json:"rttMs,omitempty"`
	LossPct  *float64  `json:"lossPct,omitempty"`
	MOS      *float64  `json:"mos,omitempty"`
}

// Call is the summary view of a call.
type Call struct {
	ID              uint       `json:"id"`
	Direction       string     `json:"direction"`
	Disposition     string     `json:"disposition"`
	FromNumber      string     `json:"fromNumber"`
	ToNumber        string     `json:"toNumber"`
	FromUserID      *uint      `json:"fromUserId,omitempty"`
	ToUserID        *uint      `json:"toUserId,omitempty"`
	ContactID       *uint      `json:"contactId,omitempty"`
	StartedAt       time.Time  `json:"startedAt"`
	AnsweredAt      *time.Time `json:"answeredAt,omitempty"`
	EndedAt         *time.Time `json:"endedAt,omitempty"`
	RingSeconds     int        `json:"ringSeconds"`
	TalkSeconds     int        `json:"talkSeconds"`
	HangupCauseCode *int       `json:"hangupCauseCode,omitempty"`
	HangupCauseText string     `json:"hangupCauseText,omitempty"`
	HangupBy        *string    `json:"hangupBy,omitempty"`
}

// CallDetail is a call with its timeline and quality samples.
type CallDetail struct {
	Call
	Events  []CallEvent   `json:"events"`
	Quality []CallQuality `json:"quality"`
}

// CallList is a paginated page of calls.
type CallList struct {
	Items   []Call `json:"items"`
	Total   int64  `json:"total"`
	Page    int    `json:"page"`
	PerPage int    `json:"perPage"`
}

// NewCall maps a call model to its summary view.
func NewCall(c *models.Call) Call {
	return Call{
		ID:              c.ID,
		Direction:       c.Direction,
		Disposition:     c.Disposition,
		FromNumber:      c.FromNumber,
		ToNumber:        c.ToNumber,
		FromUserID:      c.FromUserID,
		ToUserID:        c.ToUserID,
		ContactID:       c.ContactID,
		StartedAt:       c.StartedAt,
		AnsweredAt:      c.AnsweredAt,
		EndedAt:         c.EndedAt,
		RingSeconds:     c.RingSeconds,
		TalkSeconds:     c.TalkSeconds,
		HangupCauseCode: c.HangupCauseCode,
		HangupCauseText: c.HangupCauseText,
		HangupBy:        c.HangupBy,
	}
}

// NewCallDetail maps a call with its timeline and quality samples.
func NewCallDetail(c *models.Call, events []models.CallEvent, quality []models.CallQuality) CallDetail {
	evs := make([]CallEvent, 0, len(events))
	for i := range events {
		evs = append(evs, CallEvent{
			ID:      events[i].ID,
			Seq:     events[i].Seq,
			Type:    events[i].Type,
			Channel: events[i].Channel,
			At:      events[i].At,
			Detail:  events[i].Detail,
		})
	}
	qs := make([]CallQuality, 0, len(quality))
	for i := range quality {
		qs = append(qs, CallQuality{
			ID:       quality[i].ID,
			Leg:      quality[i].Leg,
			Codec:    quality[i].Codec,
			At:       quality[i].At,
			JitterMs: quality[i].JitterMs,
			RTTMs:    quality[i].RTTMs,
			LossPct:  quality[i].LossPct,
			MOS:      quality[i].MOS,
		})
	}
	return CallDetail{Call: NewCall(c), Events: evs, Quality: qs}
}
