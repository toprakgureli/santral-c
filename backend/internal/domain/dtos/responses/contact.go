package responses

import (
	"time"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
)

// ContactPhone is the public view of a contact phone number.
type ContactPhone struct {
	ID        uint   `json:"id"`
	Label     string `json:"label"`
	Number    string `json:"number"`
	IsPrimary bool   `json:"isPrimary"`
}

// Contact is the public view of a contact.
type Contact struct {
	ID        uint           `json:"id"`
	Name      string         `json:"name"`
	Company   string         `json:"company,omitempty"`
	Email     string         `json:"email,omitempty"`
	Notes     string         `json:"notes,omitempty"`
	Phones    []ContactPhone `json:"phones"`
	CreatedBy *uint          `json:"createdBy,omitempty"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
}

// ContactList is a paginated page of contacts.
type ContactList struct {
	Items   []Contact `json:"items"`
	Total   int64     `json:"total"`
	Page    int       `json:"page"`
	PerPage int       `json:"perPage"`
}

// NewContact maps a contact model to its response view.
func NewContact(c *models.Contact) Contact {
	phones := make([]ContactPhone, 0, len(c.Phones))
	for i := range c.Phones {
		phones = append(phones, ContactPhone{
			ID:        c.Phones[i].ID,
			Label:     c.Phones[i].Label,
			Number:    c.Phones[i].NumberE164,
			IsPrimary: c.Phones[i].IsPrimary,
		})
	}
	return Contact{
		ID:        c.ID,
		Name:      c.Name,
		Company:   c.Company,
		Email:     c.Email,
		Notes:     c.Notes,
		Phones:    phones,
		CreatedBy: c.CreatedBy,
		CreatedAt: c.CreatedAt,
		UpdatedAt: c.UpdatedAt,
	}
}
