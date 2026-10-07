package responses

import (
	"time"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
)

// Shift is the public view of a work shift.
type Shift struct {
	ID        uint       `json:"id"`
	StartedAt time.Time  `json:"startedAt"`
	EndedAt   *time.Time `json:"endedAt,omitempty"`
	EndedBy   string     `json:"endedBy,omitempty"`
}

// ShiftStatus is the actor's current shift (nil when off shift) together with
// the day's cutoffs, so the panel can warn before the automatic close.
type ShiftStatus struct {
	Shift *Shift `json:"shift"`
	// ReminderAt is the end of the working day (18:30 Istanbul unless set
	// otherwise), of the open shift or of one started now.
	ReminderAt *time.Time `json:"reminderAt,omitempty"`
	// AutoEndAt is when the server closes the shift on its own, fifty
	// minutes after the day ends.
	AutoEndAt *time.Time `json:"autoEndAt,omitempty"`
	// SummaryAt is when the day's summary comes up, five minutes before
	// the day ends.
	SummaryAt *time.Time `json:"summaryAt,omitempty"`
}

// NewShift maps a shift model to its public view.
func NewShift(s *models.Shift) *Shift {
	if s == nil {
		return nil
	}
	out := &Shift{ID: s.ID, StartedAt: s.StartedAt, EndedAt: s.EndedAt}
	if s.EndedBy != nil {
		out.EndedBy = *s.EndedBy
	}
	return out
}
