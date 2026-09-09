package requests

import "time"

// CallOriginate starts an outbound call from the actor's extension.
type CallOriginate struct {
	To string `json:"to" validate:"required,min=3,max=32"`
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
