package whatsapp

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/internal/whatsapp/hours"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
	"github.com/toprakgureli/santral-c/backend/pkg/phone"
)

const missedCallRecoveryKey = "missed_call_recovery"

// MissedCallRecoverySettings configures automatic or manual WhatsApp recovery for missed calls.
type MissedCallRecoverySettings struct {
	Enabled      bool     `json:"enabled"`
	Mode         string   `json:"mode"` // "auto" (default) or "manual"
	ChannelID    uint     `json:"channelId"`
	Template     string   `json:"template"`
	TemplateLang string   `json:"templateLang"`
	Params       []string `json:"params"` // {musteri}, {tarih}, {saat}
	QuietHours   int      `json:"quietHours"`
}

func defaultMissedCallRecovery() MissedCallRecoverySettings {
	return MissedCallRecoverySettings{
		Enabled:      false,
		Mode:         "auto",
		ChannelID:    0,
		Template:     "",
		TemplateLang: "tr",
		Params:       []string{},
		QuietHours:   4,
	}
}

func (s *Service) missedCallRecoverySettings(ctx context.Context) MissedCallRecoverySettings {
	set := defaultMissedCallRecovery()
	s.loadGlobal(ctx, missedCallRecoveryKey, &set)
	if set.Params == nil {
		set.Params = []string{}
	}
	if set.QuietHours <= 0 {
		set.QuietHours = 4
	}
	if set.TemplateLang == "" {
		set.TemplateLang = "tr"
	}
	if set.Mode != "manual" {
		set.Mode = "auto"
	}
	return set
}

// MissedCallRecovery returns the stored settings.
func (s *Service) MissedCallRecovery(ctx context.Context, actorID uint) (*MissedCallRecoverySettings, error) {
	if _, err := s.require(ctx, actorID, enums.WAMissedCallRecovery, "Kaçan çağrı kurtarma ayarlarını görme yetkin yok."); err != nil {
		return nil, err
	}
	set := s.missedCallRecoverySettings(ctx)
	return &set, nil
}

// SaveMissedCallRecovery stores the settings after checking permissions and template.
func (s *Service) SaveMissedCallRecovery(ctx context.Context, actorID uint, in MissedCallRecoverySettings) (*MissedCallRecoverySettings, error) {
	if _, err := s.require(ctx, actorID, enums.WAMissedCallRecovery, "Kaçan çağrı kurtarma ayarlarını değiştirme yetkin yok."); err != nil {
		return nil, err
	}
	if in.Enabled {
		if in.ChannelID == 0 {
			return nil, errs.Invalid("Gönderen WhatsApp numarası seçilmelidir.", nil)
		}
		if in.Template == "" {
			return nil, errs.Invalid("Gönderilecek şablon seçilmelidir.", nil)
		}
		ch, err := s.repo.Channel(ctx, in.ChannelID)
		if err != nil || !ch.Active {
			return nil, errs.Invalid("Seçilen WhatsApp numarası bulunamadı ya da aktif değil.", nil)
		}
		if in.TemplateLang == "" {
			in.TemplateLang = "tr"
		}
		if _, err := s.repo.ApprovedTemplateIn(ctx, ch.WABAID, in.Template, in.TemplateLang); err != nil {
			return nil, errs.Invalid("Seçilen şablon onaylı değil ya da silinmiş.", nil)
		}
	}
	if in.QuietHours < 1 {
		in.QuietHours = 4
	}
	if in.Mode != "manual" {
		in.Mode = "auto"
	}
	if in.Params == nil {
		in.Params = []string{}
	}
	if err := s.saveGlobal(ctx, actorID, missedCallRecoveryKey, in); err != nil {
		return nil, err
	}
	return &in, nil
}

// OnMissedCall receives an inbound missed call from the PBX and queues the recovery template if configured.
func (s *Service) OnMissedCall(ctx context.Context, peerNumber, callUUID string, startedAt time.Time) {
	if peerNumber == "" || callUUID == "" {
		return
	}
	set := s.missedCallRecoverySettings(ctx)
	if !set.Enabled || set.ChannelID == 0 || set.Template == "" {
		return
	}

	waID, ok := mobileWAID(peerNumber)
	if !ok {
		// Non-mobile numbers (landlines, extensions) cannot receive WhatsApp messages
		return
	}
	key := phone.Key(waID)

	// Deduplication: Has this call already been processed?
	already, err := s.repo.HasMissedCallRecovery(ctx, callUUID)
	if err != nil || already {
		return
	}

	// Cooldown: Has this customer received a recovery message within QuietHours?
	if set.QuietHours > 0 {
		since := time.Now().Add(-time.Duration(set.QuietHours) * time.Hour)
		recentCount, err := s.repo.RecentMissedCallRecoveries(ctx, key, since)
		if err == nil && recentCount > 0 {
			// Record skipped for audit
			_ = s.repo.LogMissedCallRecovery(ctx, &models.WAMissedCallRecovery{
				CallUUID:   callUUID,
				PeerNumber: peerNumber,
				PeerKey:    key,
				ChannelID:  &set.ChannelID,
				Template:   set.Template,
				Status:     "skipped",
				Note:       fmt.Sprintf("Son %d saatte zaten kurtarma mesajı gönderildi", set.QuietHours),
				CreatedAt:  time.Now(),
			})
			return
		}
	}

	if set.Mode == "manual" {
		_ = s.repo.LogMissedCallRecovery(ctx, &models.WAMissedCallRecovery{
			CallUUID:   callUUID,
			PeerNumber: peerNumber,
			PeerKey:    key,
			ChannelID:  &set.ChannelID,
			Template:   set.Template,
			Status:     "pending",
			Note:       "Temsilci onayı bekleniyor",
			CreatedAt:  startedAt,
		})
		slog.InfoContext(ctx, "missed call recovery queued for approval", "peer", peerNumber, "call", callUUID)
		return
	}

	ch, err := s.repo.Channel(ctx, set.ChannelID)
	if err != nil || !ch.Active {
		slog.WarnContext(ctx, "missed call recovery channel not active", "channelId", set.ChannelID)
		return
	}
	tpl, err := s.repo.ApprovedTemplateIn(ctx, ch.WABAID, set.Template, set.TemplateLang)
	if err != nil {
		slog.WarnContext(ctx, "missed call recovery template not approved", "template", set.Template, "error", err)
		return
	}

	contact, conv, err := s.repo.EnsureConversation(ctx, ch.ID, waID)
	if err != nil {
		slog.WarnContext(ctx, "missed call recovery conversation could not be ensured", "waId", waID, "error", err)
		return
	}
	if contact.Blocked {
		return
	}

	customer := firstName(contactView(contact).Display)
	if customer == "" || strings.HasPrefix(customer, "+") {
		customer = "Değerli Müşterimiz"
	}

	fill := strings.NewReplacer(
		"{musteri}", customer,
		"{tarih}", startedAt.In(hours.Zone).Format("02.01.2006"),
		"{saat}", startedAt.In(hours.Zone).Format("15:04"),
	)
	params := TemplateParams{}
	for _, p := range set.Params {
		v := oneLine(fill.Replace(p))
		if v == "" {
			v = "-"
		}
		params.Body = append(params.Body, v)
	}

	obj, preview, err := buildTemplate(tpl, params)
	if err != nil {
		slog.WarnContext(ctx, "missed call recovery template build failed", "error", err)
		return
	}

	ticket := &models.WATicket{}
	if conv.TicketID != nil {
		if t := s.repo.Ticket(ctx, *conv.TicketID); t != nil {
			ticket = t
		}
	}

	msg := &models.WAMessage{
		ChannelID:      ch.ID,
		ConversationID: conv.ID,
		Direction:      "out",
		Kind:           "template",
		SenderKind:     "automation",
		SenderLabel:    "Kaçan çağrı kurtarma",
		Body:           preview,
		Status:         "queued",
		CreatedAt:      time.Now(),
	}
	if ticket.ID > 0 && ticket.Status != "resolved" {
		msg.TicketID = uintPtr(ticket.ID)
	}

	if _, err := s.enqueue(ctx, ch, conv, ticket, msg, map[string]any{"type": "template", "template": obj}, 0); err != nil {
		slog.WarnContext(ctx, "missed call recovery enqueue failed", "error", err)
		_ = s.repo.LogMissedCallRecovery(ctx, &models.WAMissedCallRecovery{
			CallUUID:   callUUID,
			PeerNumber: peerNumber,
			PeerKey:    key,
			ChannelID:  &ch.ID,
			Template:   set.Template,
			Status:     "failed",
			Note:       truncate(err.Error(), 250),
			CreatedAt:  time.Now(),
		})
		return
	}

	_ = s.repo.LogMissedCallRecovery(ctx, &models.WAMissedCallRecovery{
		CallUUID:   callUUID,
		PeerNumber: peerNumber,
		PeerKey:    key,
		ChannelID:  &ch.ID,
		Template:   set.Template,
		Status:     "sent",
		CreatedAt:  time.Now(),
	})

	slog.InfoContext(ctx, "missed call recovery message sent", "peer", peerNumber, "call", callUUID)
}

func (s *Service) checkMissedCallAccess(ctx context.Context, actorID uint) error {
	v, err := s.viewerOf(ctx, actorID)
	if err != nil {
		return err
	}
	if v.user.Can(enums.WAMissedCallRecovery) || v.user.Can(enums.WATemplateSend) {
		return nil
	}
	return errs.Forbidden("Kaçan çağrı kurtarma listesine erişim yetkin yok.")
}

// MissedCallItems retrieves missed call recovery attempts for review and action.
func (s *Service) MissedCallItems(ctx context.Context, actorID uint, status string, all bool) ([]models.WAMissedCallRecoveryItem, error) {
	if err := s.checkMissedCallAccess(ctx, actorID); err != nil {
		return nil, err
	}
	if !all && status == "" {
		status = "pending"
	}
	return s.repo.ListMissedCallRecoveries(ctx, status, 50)
}

// SendMissedCallRecovery triggers sending the recovery template for a pending recovery.
func (s *Service) SendMissedCallRecovery(ctx context.Context, actorID, id uint) (*models.WAMissedCallRecovery, error) {
	if err := s.checkMissedCallAccess(ctx, actorID); err != nil {
		return nil, err
	}
	item, err := s.repo.MissedCallRecoveryByID(ctx, id)
	if err != nil {
		return nil, errs.NotFound("Kayıt bulunamadı.")
	}
	if item.Status != "pending" && item.Status != "failed" {
		return nil, errs.Invalid("Bu çağrı için zaten işlem yapılmış.", nil)
	}

	set := s.missedCallRecoverySettings(ctx)
	channelID := set.ChannelID
	if item.ChannelID != nil && *item.ChannelID > 0 {
		channelID = *item.ChannelID
	}
	templateName := item.Template
	if templateName == "" {
		templateName = set.Template
	}

	waID, ok := mobileWAID(item.PeerNumber)
	if !ok {
		return nil, errs.Invalid("Geçerli bir cep telefonu numarası değil.", nil)
	}

	ch, err := s.repo.Channel(ctx, channelID)
	if err != nil || !ch.Active {
		return nil, errs.Invalid("WhatsApp hattı aktif değil.", err)
	}
	tpl, err := s.repo.ApprovedTemplateIn(ctx, ch.WABAID, templateName, set.TemplateLang)
	if err != nil {
		return nil, errs.Invalid("Seçilen şablon onaylı değil.", err)
	}

	contact, conv, err := s.repo.EnsureConversation(ctx, ch.ID, waID)
	if err != nil {
		return nil, errs.Invalid("Sohbet oluşturulamadı.", err)
	}
	if contact.Blocked {
		return nil, errs.Invalid("Numara engellenmiş.", nil)
	}

	customer := firstName(contactView(contact).Display)
	if customer == "" || strings.HasPrefix(customer, "+") {
		customer = "Değerli Müşterimiz"
	}

	fill := strings.NewReplacer(
		"{musteri}", customer,
		"{tarih}", item.CreatedAt.In(hours.Zone).Format("02.01.2006"),
		"{saat}", item.CreatedAt.In(hours.Zone).Format("15:04"),
	)
	params := TemplateParams{}
	for _, p := range set.Params {
		v := oneLine(fill.Replace(p))
		if v == "" {
			v = "-"
		}
		params.Body = append(params.Body, v)
	}

	obj, preview, err := buildTemplate(tpl, params)
	if err != nil {
		return nil, errs.Invalid("Şablon oluşturulamadı.", err)
	}

	ticket := &models.WATicket{}
	if conv.TicketID != nil {
		if t := s.repo.Ticket(ctx, *conv.TicketID); t != nil {
			ticket = t
		}
	}

	v, _ := s.viewerOf(ctx, actorID)
	senderLabel := "Kaçan çağrı kurtarma"
	if v != nil && v.user != nil && v.user.Name != "" {
		senderLabel = fmt.Sprintf("Kaçan çağrı kurtarma (%s)", v.user.Name)
	}

	msg := &models.WAMessage{
		ChannelID:      ch.ID,
		ConversationID: conv.ID,
		Direction:      "out",
		Kind:           "template",
		SenderKind:     "agent",
		SenderUserID:   &actorID,
		SenderLabel:    senderLabel,
		Body:           preview,
		Status:         "queued",
		CreatedAt:      time.Now(),
	}
	if ticket.ID > 0 && ticket.Status != "resolved" {
		msg.TicketID = uintPtr(ticket.ID)
	}

	if _, err := s.enqueue(ctx, ch, conv, ticket, msg, map[string]any{"type": "template", "template": obj}, actorID); err != nil {
		item.Status = "failed"
		item.Note = truncate(err.Error(), 250)
		_ = s.repo.UpdateMissedCallRecovery(ctx, item)
		return nil, errs.Invalid("Mesaj gönderilemedi.", err)
	}

	item.Status = "sent"
	if v != nil && v.user != nil && v.user.Name != "" {
		item.Note = fmt.Sprintf("%s tarafından onaylanıp gönderildi", v.user.Name)
	} else {
		item.Note = "Temsilci tarafından onaylanıp gönderildi"
	}
	if err := s.repo.UpdateMissedCallRecovery(ctx, item); err != nil {
		return nil, err
	}
	return item, nil
}

// DismissMissedCallRecovery marks a pending recovery as dismissed.
func (s *Service) DismissMissedCallRecovery(ctx context.Context, actorID, id uint) (*models.WAMissedCallRecovery, error) {
	if err := s.checkMissedCallAccess(ctx, actorID); err != nil {
		return nil, err
	}
	item, err := s.repo.MissedCallRecoveryByID(ctx, id)
	if err != nil {
		return nil, errs.NotFound("Kayıt bulunamadı.")
	}
	if item.Status != "pending" {
		return nil, errs.Invalid("Bu çağrı için zaten işlem yapılmış.", nil)
	}

	v, _ := s.viewerOf(ctx, actorID)
	item.Status = "dismissed"
	if v != nil && v.user != nil && v.user.Name != "" {
		item.Note = fmt.Sprintf("%s tarafından iptal edildi", v.user.Name)
	} else {
		item.Note = "İptal edildi"
	}
	if err := s.repo.UpdateMissedCallRecovery(ctx, item); err != nil {
		return nil, err
	}
	return item, nil
}
