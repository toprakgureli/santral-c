package store

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
)

// ContactsByIDs reads the customers with the given ids, in no particular order.
func (r *Repository) ContactsByIDs(ctx context.Context, ids []uint) ([]models.WAContact, error) {
	var contacts []models.WAContact
	if err := r.db.WithContext(ctx).Where("id IN ?", ids).Find(&contacts).Error; err != nil {
		return nil, err
	}
	return contacts, nil
}

// ContactByWAID reads a customer by WhatsApp id.
func (r *Repository) ContactByWAID(ctx context.Context, waID string) (*models.WAContact, error) {
	var c models.WAContact
	if err := r.db.WithContext(ctx).Where("wa_id = ?", waID).First(&c).Error; err != nil {
		return nil, err
	}
	return &c, nil
}

// SetContactSource records the advert or link a customer came from, as JSON.
func SetContactSource(tx *gorm.DB, id uint, source string) error {
	return tx.Exec("UPDATE wa_contacts SET source = ? WHERE id = ?", source, id).Error
}

// OptOut records that a customer does not want marketing messages.
func OptOut(tx *gorm.DB, id uint) error {
	return tx.Exec("UPDATE wa_contacts SET opted_out = true WHERE id = ?", id).Error
}

// UpdateContact changes the given columns of a customer.
func (r *Repository) UpdateContact(ctx context.Context, id uint, fields map[string]any) error {
	return r.db.WithContext(ctx).Model(&models.WAContact{}).Where("id = ?", id).Updates(fields).Error
}

// ConversationIDsOfContact lists a customer's conversations, by id.
func (r *Repository) ConversationIDsOfContact(ctx context.Context, contactID uint) ([]uint, error) {
	var convs []uint
	err := r.db.WithContext(ctx).Raw("SELECT id FROM wa_conversations WHERE contact_id = ?", contactID).Scan(&convs).Error
	return convs, err
}

// ConversationsOfNumber lists up to 20 conversations with a ticket the
// person sees of the customers whose number has the given key, latest
// message first.
func (r *Repository) ConversationsOfNumber(ctx context.Context, reach Reach, key string) ([]models.WAConversation, error) {
	seen, seenArgs := reach.tickets()
	var convs []models.WAConversation
	if err := r.db.WithContext(ctx).Where("contact_id IN (SELECT id FROM wa_contacts WHERE peer_key = ?) AND ticket_id IS NOT NULL", key).
		Where("ticket_id IN ("+seen+")", seenArgs...).
		Order("last_message_at DESC NULLS LAST").Limit(20).Find(&convs).Error; err != nil {
		return nil, err
	}
	return convs, nil
}

// RecentTicketsOfContact reads the fields that decide who sees a ticket
// for a customer's newest tickets, newest first.
func (r *Repository) RecentTicketsOfContact(ctx context.Context, contactID uint, limit int) ([]models.WATicket, error) {
	var tickets []models.WATicket
	if err := r.db.WithContext(ctx).
		Select("id", "channel_id", "status", "owner_id", "team_id", "waiting_listed_at").
		Where("contact_id = ?", contactID).Order("id DESC").Limit(limit).
		Find(&tickets).Error; err != nil {
		return nil, err
	}
	return tickets, nil
}

// JoinedTickets lists which of the given tickets a person takes part in.
func (r *Repository) JoinedTickets(ctx context.Context, userID uint, ticketIDs []uint) ([]uint, error) {
	var joined []uint
	if err := r.db.WithContext(ctx).Raw("SELECT ticket_id FROM wa_ticket_participants WHERE user_id = ? AND ticket_id IN ?", userID, ticketIDs).
		Scan(&joined).Error; err != nil {
		return nil, err
	}
	return joined, nil
}

// ContactTicket is one of a customer's tickets as their history shows it.
type ContactTicket struct {
	ConversationID uint
	TicketID       uint
	Number         int64
	ChannelID      uint
	ChannelName    string
	Status         string
	Owner          string
	CreatedAt      time.Time
	ResolvedAt     *time.Time
	Rating         *int
	Messages       int64
}

// ContactTickets reads a customer's newest tickets, newest first, with the
// device, the owner's name and how many messages went each way.
func (r *Repository) ContactTickets(ctx context.Context, contactID uint, limit int) ([]ContactTicket, error) {
	var rows []ContactTicket
	if err := r.db.WithContext(ctx).Raw(`SELECT t.conversation_id, t.id AS ticket_id, t.number, t.channel_id, c.name AS channel_name, t.status,
		COALESCE(u.name, '') AS owner, t.created_at, t.resolved_at, t.rating,
		(SELECT count(*) FROM wa_messages m WHERE m.ticket_id = t.id AND m.direction IN ('in','out')) AS messages
		FROM wa_tickets t JOIN wa_channels c ON c.id = t.channel_id LEFT JOIN users u ON u.id = t.owner_id
		WHERE t.contact_id = ? ORDER BY t.id DESC LIMIT ?`, contactID, limit).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// StartConversation stores the customer of a number and their conversation
// on a device in one transaction, naming a new customer. When the
// conversation has no ticket it gets an open one owned by ownerID. It
// returns the conversation's id.
func (r *Repository) StartConversation(ctx context.Context, channelID uint, waID, name string, ownerID uint) (uint, error) {
	var convID uint
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		c, err := UpsertContact(tx, waID, "")
		if err != nil {
			return err
		}
		if name != "" && c.Name == "" {
			if err := tx.Exec("UPDATE wa_contacts SET name = ? WHERE id = ?", name, c.ID).Error; err != nil {
				return err
			}
		}
		conv, _, err := UpsertConversation(tx, channelID, c.ID)
		if err != nil {
			return err
		}
		convID = conv.ID
		if conv.TicketID == nil {
			owner := ownerID
			t := models.WATicket{ConversationID: conv.ID, ChannelID: channelID, ContactID: c.ID, Status: "open", Priority: "normal", Tags: "[]", OwnerID: &owner}
			if err := tx.Create(&t).Error; err != nil {
				return err
			}
			if err := tx.Exec("UPDATE wa_conversations SET ticket_id = ?, last_message_at = now() WHERE id = ?", t.ID, conv.ID).Error; err != nil {
				return err
			}
			return AddOwner(tx, t.ID, ownerID)
		}
		return nil
	})
	return convID, err
}

// EnsureConversation stores the customer of a number and their
// conversation on a device in one transaction, and returns both.
func (r *Repository) EnsureConversation(ctx context.Context, channelID uint, waID string) (*models.WAContact, *models.WAConversation, error) {
	var conv *models.WAConversation
	var contact *models.WAContact
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		c, err := UpsertContact(tx, waID, "")
		if err != nil {
			return err
		}
		contact = c
		cv, _, err := UpsertConversation(tx, channelID, c.ID)
		conv = cv
		return err
	})
	if err != nil {
		return nil, nil, err
	}
	return contact, conv, nil
}
