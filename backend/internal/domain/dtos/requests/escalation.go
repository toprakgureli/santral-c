package requests

// EscalationCategoryCreate creates a top-level escalation category.
type EscalationCategoryCreate struct {
	Name string `json:"name" validate:"required,min=2,max=160"`
}

// EscalationReasonCreate adds a reason under a category.
type EscalationReasonCreate struct {
	Name string `json:"name" validate:"required,min=2,max=240"`
}

// EscalationNone records that the agent looked at a call and decided no
// escalation was needed, so the decision itself is on the customer's record.
type EscalationNone struct {
	Number   string `json:"number" validate:"required,min=3,max=32"`
	CallUUID string `json:"callUuid" validate:"max=80"`
}

// EscalationCreate logs an escalation an agent recorded during a call.
type EscalationCreate struct {
	Number   string `json:"number" validate:"required,min=3,max=32"`
	ReasonID uint   `json:"reasonId" validate:"required"`
	Note     string `json:"note" validate:"max=2000"`
	CallUUID string `json:"callUuid" validate:"max=80"`
}
