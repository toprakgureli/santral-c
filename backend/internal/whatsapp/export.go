package whatsapp

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
)

var unsafeName = regexp.MustCompile(`[^\p{L}\p{N}_-]+`)

// ExportConversation writes a whole conversation as plain text, oldest
// first: who wrote, when, what, including internal notes and events.
func (s *Service) ExportConversation(ctx context.Context, actorID, conversationID uint) ([]byte, string, error) {
	v, conv, _, err := s.reachable(ctx, actorID, conversationID)
	if err != nil {
		return nil, "", err
	}
	if !v.can(enums.WAExport) {
		return nil, "", errs.Forbidden("Yazışmayı dışa aktarma yetkiniz yok.")
	}
	contact, err := s.contact(ctx, conv.ContactID)
	if err != nil {
		return nil, "", err
	}
	ch, err := s.channel(ctx, conv.ChannelID)
	if err != nil {
		return nil, "", err
	}
	var msgs []models.WAMessage
	if err := s.db.WithContext(ctx).Where("conversation_id = ? AND kind <> 'reaction'", conv.ID).Order("id").Find(&msgs).Error; err != nil {
		return nil, "", errs.Internal(err)
	}
	names := map[uint]string{}
	nameOf := func(id uint) string {
		if n, ok := names[id]; ok {
			return n
		}
		n := "Temsilci"
		if u, err := s.users.GetByID(ctx, id); err == nil {
			n = u.Name
		}
		names[id] = n
		return n
	}
	cv := contactView(contact)
	var b strings.Builder
	fmt.Fprintf(&b, "WhatsApp yazışması\nMüşteri: %s (+%s)\nNumara: %s\n\n", cv.Display, contact.WAID, ch.Name)
	for i := range msgs {
		m := &msgs[i]
		at := m.CreatedAt.In(istanbul).Format("02.01.2006 15:04")
		who := cv.Display
		switch m.Direction {
		case "event":
			fmt.Fprintf(&b, "[%s] --- %s ---\n", at, m.Body)
			continue
		case "note":
			who = "İç not"
			if m.SenderUserID != nil {
				who = "İç not, " + nameOf(*m.SenderUserID)
			}
		case "out":
			switch {
			case m.SenderUserID != nil:
				who = nameOf(*m.SenderUserID)
			case m.SenderKind == "bot":
				who = "Chatbot"
			default:
				who = "Otomatik mesaj"
			}
			if m.SenderLabel != "" && m.SenderUserID == nil {
				who += " (" + m.SenderLabel + ")"
			}
		}
		text := strings.TrimSpace(m.Body)
		if m.Kind != "text" && m.Kind != "template" && m.Kind != "interactive" && m.Kind != "button" {
			label := "[" + kindWord(m.Kind) + "]"
			if ref := s.mediaRef(m); ref.Name != "" {
				label = "[" + kindWord(m.Kind) + ": " + ref.Name + "]"
			}
			text = strings.TrimSpace(label + " " + text)
		}
		if m.Status == "failed" {
			text += " (gönderilemedi)"
		}
		fmt.Fprintf(&b, "[%s] %s: %s\n", at, who, text)
	}
	name := unsafeName.ReplaceAllString(strings.TrimSpace(cv.Display), "_")
	if name == "" {
		name = contact.WAID
	}
	return []byte(b.String()), fmt.Sprintf("whatsapp_%s_%d.txt", name, conv.ID), nil
}
