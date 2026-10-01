// Package store reads and writes the WhatsApp module's shared records:
// devices, customers, conversations, tickets and their participants,
// templates and chatbots. The functions that take a transaction run inside
// one the caller opened.
package store

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
	"github.com/toprakgureli/santral-c/backend/pkg/phone"
)

// Repository is the module's access to its shared records.
type Repository struct {
	db *gorm.DB
}

// New builds the repository.
func New(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// Channel loads a device.
func (r *Repository) Channel(ctx context.Context, id uint) (*models.WAChannel, error) {
	var ch models.WAChannel
	err := r.db.WithContext(ctx).First(&ch, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errs.NotFound("Cihaz bulunamadı.")
	}
	if err != nil {
		return nil, errs.Internal(err)
	}
	return &ch, nil
}

// Contact loads a customer.
func (r *Repository) Contact(ctx context.Context, id uint) (*models.WAContact, error) {
	var c models.WAContact
	if err := r.db.WithContext(ctx).First(&c, id).Error; err != nil {
		return nil, errs.NotFound("Müşteri bulunamadı.")
	}
	return &c, nil
}

// ContactOfConversation loads the customer of a conversation.
func (r *Repository) ContactOfConversation(ctx context.Context, conversationID uint) (*models.WAContact, error) {
	var c models.WAContact
	err := r.db.WithContext(ctx).Raw("SELECT c.* FROM wa_contacts c JOIN wa_conversations v ON v.contact_id = c.id WHERE v.id = ?", conversationID).Scan(&c).Error
	if err != nil || c.ID == 0 {
		return nil, fmt.Errorf("contact of conversation %d not found", conversationID)
	}
	return &c, nil
}

// Participants returns who took part in a ticket; a failed read counts as nobody.
func (r *Repository) Participants(ctx context.Context, ticketID uint) map[uint]bool {
	var ids []uint
	if err := r.db.WithContext(ctx).Raw("SELECT user_id FROM wa_ticket_participants WHERE ticket_id = ?", ticketID).Scan(&ids).Error; err != nil {
		slog.WarnContext(ctx, "ticket participants could not be read", "ticket", ticketID, "error", err)
	}
	out := make(map[uint]bool, len(ids))
	for _, id := range ids {
		out[id] = true
	}
	return out
}

// Conversation loads a conversation and its current ticket, if it has one.
func (r *Repository) Conversation(ctx context.Context, id uint) (*models.WAConversation, *models.WATicket, error) {
	var c models.WAConversation
	if err := r.db.WithContext(ctx).First(&c, id).Error; err != nil {
		return nil, nil, err
	}
	var t *models.WATicket
	if c.TicketID != nil {
		var tt models.WATicket
		if err := r.db.WithContext(ctx).First(&tt, *c.TicketID).Error; err == nil {
			t = &tt
		}
	}
	return &c, t, nil
}

// Template loads a message template.
func (r *Repository) Template(ctx context.Context, id uint) (*models.WATemplate, error) {
	var t models.WATemplate
	err := r.db.WithContext(ctx).First(&t, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errs.NotFound("Şablon bulunamadı.")
	}
	if err != nil {
		return nil, errs.Internal(err)
	}
	return &t, nil
}

// UserName is a panel user's name, or "Biri" when it cannot be read.
func (r *Repository) UserName(ctx context.Context, id uint) string {
	var name string
	if err := r.db.WithContext(ctx).Raw("SELECT name FROM users WHERE id = ?", id).Scan(&name).Error; err != nil {
		slog.WarnContext(ctx, "user name could not be read", "user", id, "error", err)
	}
	if name == "" {
		return "Biri"
	}
	return name
}

// Ticket reads a ticket as it is now, or nil when it cannot be read.
func (r *Repository) Ticket(ctx context.Context, id uint) *models.WATicket {
	var t models.WATicket
	if r.db.WithContext(ctx).First(&t, id).Error != nil {
		return nil
	}
	return &t
}

// Bot loads a chatbot.
func (r *Repository) Bot(ctx context.Context, id uint) (*models.WABot, error) {
	var b models.WABot
	err := r.db.WithContext(ctx).First(&b, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errs.NotFound("Chatbot bulunamadı.")
	}
	if err != nil {
		return nil, errs.Internal(err)
	}
	return &b, nil
}

// LockTicket reads a ticket's current state and holds its row until the
// transaction ends, so two people changing it at once take turns instead
// of overwriting each other.
func LockTicket(tx *gorm.DB, id uint) (*models.WATicket, error) {
	var t models.WATicket
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&t, id).Error; err != nil {
		return nil, fmt.Errorf("ticket %d could not be locked: %w", id, err)
	}
	return &t, nil
}

// UpsertContact stores a customer by WhatsApp id, keeping the newest profile name.
func UpsertContact(tx *gorm.DB, waID, profileName string) (*models.WAContact, error) {
	var c models.WAContact
	err := tx.Raw(`INSERT INTO wa_contacts (wa_id, peer_key, profile_name) VALUES (?, ?, ?)
		ON CONFLICT (wa_id) DO UPDATE SET
			profile_name = CASE WHEN EXCLUDED.profile_name <> '' THEN EXCLUDED.profile_name ELSE wa_contacts.profile_name END,
			updated_at = now()
		RETURNING *`, waID, phone.Key(waID), profileName).Scan(&c).Error
	if err != nil {
		return nil, fmt.Errorf("müşteri kaydedilemedi: %w", err)
	}
	return &c, nil
}

// UpsertConversation stores the conversation of a customer on a device and
// locks it for the rest of the transaction; inserted is true the first time.
func UpsertConversation(tx *gorm.DB, channelID, contactID uint) (*models.WAConversation, bool, error) {
	var c models.WAConversation
	err := tx.Raw(`INSERT INTO wa_conversations (channel_id, contact_id) VALUES (?, ?)
		ON CONFLICT (channel_id, contact_id) DO UPDATE SET channel_id = EXCLUDED.channel_id
		RETURNING *`, channelID, contactID).Scan(&c).Error
	if err != nil {
		return nil, false, fmt.Errorf("sohbet kaydedilemedi: %w", err)
	}
	var inserted bool
	if err := tx.Raw("SELECT last_message_id IS NULL FROM wa_conversations WHERE id = ?", c.ID).Scan(&inserted).Error; err != nil {
		return nil, false, err
	}
	// Lock the row for the rest of the transaction.
	if err := tx.Raw("SELECT * FROM wa_conversations WHERE id = ? FOR UPDATE", c.ID).Scan(&c).Error; err != nil {
		return nil, false, err
	}
	return &c, inserted, nil
}

// TouchTicket makes sure the conversation has an open ticket and marks it
// as waiting for our answer.
func TouchTicket(tx *gorm.DB, conv *models.WAConversation, at *time.Time) (*models.WATicket, bool, bool, *time.Time, error) {
	var t models.WATicket
	created, reopened := false, false
	var wasResolved *time.Time
	if conv.TicketID != nil {
		if err := tx.Raw("SELECT * FROM wa_tickets WHERE id = ? FOR UPDATE", *conv.TicketID).Scan(&t).Error; err != nil {
			return nil, false, false, nil, err
		}
	}
	switch {
	case t.ID == 0:
		t = models.WATicket{ConversationID: conv.ID, ChannelID: conv.ChannelID, ContactID: conv.ContactID, Status: "open", Priority: "normal", Tags: "[]", AwaitingSince: at}
		if err := tx.Create(&t).Error; err != nil {
			return nil, false, false, nil, err
		}
		if err := tx.Raw("SELECT * FROM wa_tickets WHERE id = ?", t.ID).Scan(&t).Error; err != nil {
			return nil, false, false, nil, err
		}
		created = true
	case t.Status == "resolved":
		wasResolved = t.ResolvedAt
		// Only the customer writing (or us sending a template) brings a
		// resolved ticket back. It keeps its number and history.
		if err := tx.Exec(`UPDATE wa_tickets SET status = 'open', reopen_count = reopen_count + 1, resolved_at = NULL,
			awaiting_since = ?, waiting_listed_at = NULL, updated_at = now() WHERE id = ?`, *at, t.ID).Error; err != nil {
			return nil, false, false, nil, err
		}
		t.Status, t.ReopenCount, t.ResolvedAt, t.AwaitingSince, t.WaitingListedAt = "open", t.ReopenCount+1, nil, at, nil
		reopened = true
	case t.AwaitingSince == nil:
		if err := tx.Exec("UPDATE wa_tickets SET awaiting_since = ?, updated_at = now() WHERE id = ?", *at, t.ID).Error; err != nil {
			return nil, false, false, nil, err
		}
		t.AwaitingSince = at
	default:
		if err := tx.Exec("UPDATE wa_tickets SET updated_at = now() WHERE id = ?", t.ID).Error; err != nil {
			return nil, false, false, nil, err
		}
	}
	return &t, created, reopened, wasResolved, nil
}
