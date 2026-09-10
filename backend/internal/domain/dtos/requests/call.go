package requests

import "time"

// CallOriginate starts an outbound call from the actor's extension.
type CallOriginate struct {
	To string `json:"to" validate:"required,min=3,max=32"`
}

// SIPCredentials sets a user's Bulutsantralim SIP extension and password.
type SIPCredentials struct {
	Extension string `json:"extension" validate:"required,min=2,max=32"`
	Password  string `json:"password" validate:"required,min=1,max=128"`
}

// AgentStatus sets the agent's presence. State is one of available, break,
// backoffice or dnd; any non-available state also engages do-not-disturb.
type AgentStatus struct {
	State string `json:"state" validate:"omitempty,oneof=available break backoffice dnd"`
	DND   bool   `json:"dnd"`
}

// CallLogEvent records a phase of a softphone call. Phase is start, answer or
// end; the panel generates callId and correlates the phases.
type CallLogEvent struct {
	CallID          string `json:"callId" validate:"required,max=80"`
	Phase           string `json:"phase" validate:"required,oneof=start answer end"`
	Direction       string `json:"direction" validate:"omitempty,oneof=inbound outbound internal"`
	Peer            string `json:"peer" validate:"max=40"`
	Disposition     string `json:"disposition" validate:"omitempty,oneof=answered no_answer missed busy failed canceled"`
	DurationSeconds int    `json:"durationSeconds" validate:"min=0"`
}

// CallFilter filters the call log.
type CallFilter struct {
	Direction   string
	Disposition string
	Number      string
	UserID      *uint
	From        *time.Time
	To          *time.Time
	OwnerID     *uint // restricts to calls involving this user (own-scope)
	Page        int
	PerPage     int
}
