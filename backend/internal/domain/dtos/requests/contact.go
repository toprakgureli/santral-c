package requests

// ContactPhoneInput is one phone number on a contact.
type ContactPhoneInput struct {
	Label     string `json:"label" validate:"omitempty,oneof=mobile work home other"`
	Number    string `json:"number" validate:"required,min=4,max=32"`
	IsPrimary bool   `json:"isPrimary"`
}

// ContactCreate creates a contact with its initial phone numbers.
type ContactCreate struct {
	Name    string              `json:"name" validate:"required,min=2,max=160"`
	Company string              `json:"company" validate:"omitempty,max=160"`
	Email   string              `json:"email" validate:"omitempty,email,max=255"`
	Notes   string              `json:"notes" validate:"omitempty,max=5000"`
	Phones  []ContactPhoneInput `json:"phones" validate:"omitempty,dive"`
}

// ContactUpdate edits a contact's core fields. Phones are managed separately.
type ContactUpdate struct {
	Name    string `json:"name" validate:"required,min=2,max=160"`
	Company string `json:"company" validate:"omitempty,max=160"`
	Email   string `json:"email" validate:"omitempty,email,max=255"`
	Notes   string `json:"notes" validate:"omitempty,max=5000"`
}

// ContactFilter filters the contact listing.
type ContactFilter struct {
	Query   string
	Page    int
	PerPage int
}
