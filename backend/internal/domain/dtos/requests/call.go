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

// AgentStatus toggles the agent's do-not-disturb state.
type AgentStatus struct {
	DND bool `json:"dnd"`
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
