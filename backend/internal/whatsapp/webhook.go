package whatsapp

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
)

// How Meta tells us about things. Every call is written down first and
// answered right away; the work happens afterwards in order, so a slow or
// failing step never loses a message. When writing itself fails we answer
// with an error and Meta sends the call again.

func (s *Service) channelByHook(ctx context.Context, key string) (*models.WAChannel, error) {
	var ch models.WAChannel
	err := s.db.WithContext(ctx).Where("hook_key = ?", key).First(&ch).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errs.NotFound("Bilinmeyen adres.")
	}
	if err != nil {
		return nil, errs.Internal(err)
	}
	return &ch, nil
}

// Verify answers Meta's check when the webhook address is saved.
func (s *Service) Verify(ctx context.Context, key, mode, token, challenge string) (string, error) {
	ch, err := s.channelByHook(ctx, key)
	if err != nil {
		return "", err
	}
	if mode != "subscribe" || !hmac.Equal([]byte(token), []byte(ch.VerifyToken)) {
		return "", errs.Forbidden("Doğrulama anahtarı eşleşmedi.")
	}
	return challenge, nil
}

// Receive stores one webhook call after checking Meta's signature.
func (s *Service) Receive(ctx context.Context, key, signature string, body []byte) error {
	ch, err := s.channelByHook(ctx, key)
	if err != nil {
		return err
	}
	secret := s.open(ch.AppSecretEnc)
	if secret == "" {
		return errs.Forbidden("Cihazın uygulama gizli anahtarı girilmemiş.")
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	want := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(strings.TrimSpace(signature)), []byte(want)) {
		return errs.Unauthorized("İmza doğrulanamadı.")
	}
	if !json.Valid(body) {
		return errs.Invalid("Geçersiz içerik.", nil)
	}
	ev := &models.WAWebhookEvent{ChannelID: uintPtr(ch.ID), Payload: string(body), Status: "pending", NextTryAt: time.Now()}
	if err := s.db.WithContext(ctx).Create(ev).Error; err != nil {
		return errs.Internal(err)
	}
	_ = s.db.WithContext(ctx).Exec("UPDATE wa_channels SET last_webhook_at = now() WHERE id = ?", ch.ID).Error
	wake(s.wakeWebhook)
	return nil
}

// webhookWorker processes stored calls oldest first.
func (s *Service) webhookWorker(ctx context.Context) {
	tick := time.NewTicker(5 * time.Second)
	defer tick.Stop()
	for {
		for s.processBatch(ctx) {
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		case <-s.wakeWebhook:
		}
	}
}

// processBatch handles up to a page of waiting calls; true when there may
// be more.
func (s *Service) processBatch(ctx context.Context) bool {
	var list []models.WAWebhookEvent
	if err := s.db.WithContext(ctx).Where("status = 'pending' AND next_try_at <= now()").Order("id").Limit(50).Find(&list).Error; err != nil {
		slog.ErrorContext(ctx, "whatsapp webhook events could not be read", "error", err)
		return false
	}
	for i := range list {
		ev := &list[i]
		err := s.processEvent(ctx, ev)
		if err == nil {
			_ = s.db.WithContext(ctx).Exec("UPDATE wa_webhook_events SET status = 'done', processed_at = now(), attempts = attempts + 1, last_error = '' WHERE id = ?", ev.ID).Error
			continue
		}
		attempts := ev.Attempts + 1
		status := "pending"
		wait := time.Duration(30*(1<<min(attempts, 6))) * time.Second
		if attempts >= 8 {
			status = "failed"
		}
		_ = s.db.WithContext(ctx).Exec("UPDATE wa_webhook_events SET status = ?, attempts = ?, last_error = ?, next_try_at = ? WHERE id = ?",
			status, attempts, err.Error(), time.Now().Add(wait), ev.ID).Error
		slog.WarnContext(ctx, "whatsapp webhook event failed", "event", ev.ID, "attempt", attempts, "error", err)
		if status == "failed" && ev.ChannelID != nil {
			s.alert(ctx, *ev.ChannelID, "Meta'dan gelen bir bildirim işlenemedi. Ayarlar > İşlenemeyen bildirimler ekranından tekrar deneyebilirsiniz.")
		}
	}
	return len(list) == 50
}

// ---------------------------------------------------------------- payload

type hookPayload struct {
	Object string `json:"object"`
	Entry  []struct {
		ID      string `json:"id"`
		Changes []struct {
			Field string          `json:"field"`
			Value json.RawMessage `json:"value"`
		} `json:"changes"`
	} `json:"entry"`
}

type hookMessages struct {
	Metadata struct {
		DisplayPhoneNumber string `json:"display_phone_number"`
		PhoneNumberID      string `json:"phone_number_id"`
	} `json:"metadata"`
	Contacts []struct {
		WAID    string `json:"wa_id"`
		Profile struct {
			Name string `json:"name"`
		} `json:"profile"`
	} `json:"contacts"`
	Messages []hookMessage `json:"messages"`
	Statuses []hookStatus  `json:"statuses"`
	Errors   []hookError   `json:"errors"`
}

type hookError struct {
	Code      int    `json:"code"`
	Title     string `json:"title"`
	Message   string `json:"message"`
	ErrorData struct {
		Details string `json:"details"`
	} `json:"error_data"`
}

type hookMedia struct {
	ID       string `json:"id"`
	MimeType string `json:"mime_type"`
	SHA256   string `json:"sha256"`
	Caption  string `json:"caption"`
	Filename string `json:"filename"`
	Voice    bool   `json:"voice"`
	Animated bool   `json:"animated"`
}

type hookMessage struct {
	From      string `json:"from"`
	ID        string `json:"id"`
	Timestamp string `json:"timestamp"`
	Type      string `json:"type"`
	Context   *struct {
		From string `json:"from"`
		ID   string `json:"id"`
	} `json:"context"`
	Text *struct {
		Body string `json:"body"`
	} `json:"text"`
	Image    *hookMedia `json:"image"`
	Video    *hookMedia `json:"video"`
	Audio    *hookMedia `json:"audio"`
	Document *hookMedia `json:"document"`
	Sticker  *hookMedia `json:"sticker"`
	Location *struct {
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
		Name      string  `json:"name"`
		Address   string  `json:"address"`
	} `json:"location"`
	Contacts    json.RawMessage `json:"contacts"`
	Interactive *struct {
		Type        string `json:"type"`
		ButtonReply *struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		} `json:"button_reply"`
		ListReply *struct {
			ID          string `json:"id"`
			Title       string `json:"title"`
			Description string `json:"description"`
		} `json:"list_reply"`
	} `json:"interactive"`
	Button   *hookButton `json:"button"`
	Reaction *struct {
		MessageID string `json:"message_id"`
		Emoji     string `json:"emoji"`
	} `json:"reaction"`
	Referral json.RawMessage `json:"referral"`
	Errors   []hookError     `json:"errors"`
}

type hookStatus struct {
	ID          string          `json:"id"`
	Status      string          `json:"status"`
	Timestamp   string          `json:"timestamp"`
	RecipientID string          `json:"recipient_id"`
	Pricing     json.RawMessage `json:"pricing"`
	Errors      []hookError     `json:"errors"`
}

func (s *Service) processEvent(ctx context.Context, ev *models.WAWebhookEvent) error {
	var p hookPayload
	if err := json.Unmarshal([]byte(ev.Payload), &p); err != nil {
		return fmt.Errorf("bildirim okunamadı: %w", err)
	}
	for _, entry := range p.Entry {
		for _, change := range entry.Changes {
			var err error
			switch change.Field {
			case "messages":
				err = s.onMessages(ctx, change.Value)
			case "message_template_status_update":
				err = s.onTemplateStatus(ctx, entry.ID, change.Value)
			case "message_template_quality_update":
				err = s.onTemplateQuality(ctx, entry.ID, change.Value)
			case "phone_number_quality_update":
				err = s.onQuality(ctx, entry.ID, change.Value)
			case "account_update", "account_alerts", "business_capability_update", "phone_number_name_update":
				s.onAccountNotice(ctx, entry.ID, change.Field, change.Value)
			}
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) channelByNumber(ctx context.Context, phoneNumberID string) (*models.WAChannel, error) {
	var ch models.WAChannel
	err := s.db.WithContext(ctx).Where("phone_number_id = ?", phoneNumberID).First(&ch).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &ch, err
}

func (s *Service) onMessages(ctx context.Context, raw json.RawMessage) error {
	var v hookMessages
	if err := json.Unmarshal(raw, &v); err != nil {
		return fmt.Errorf("mesaj bildirimi okunamadı: %w", err)
	}
	// Several numbers can share one app and so one webhook address; the
	// number the event names decides where it goes.
	ch, err := s.channelByNumber(ctx, v.Metadata.PhoneNumberID)
	if err != nil {
		return err
	}
	if ch == nil {
		slog.WarnContext(ctx, "whatsapp event for an unknown number", "phoneNumberId", v.Metadata.PhoneNumberID)
		return nil
	}
	names := map[string]string{}
	for _, c := range v.Contacts {
		names[c.WAID] = c.Profile.Name
	}
	for i := range v.Messages {
		if err := s.onInbound(ctx, ch, &v.Messages[i], names[v.Messages[i].From]); err != nil {
			return err
		}
	}
	for i := range v.Statuses {
		if err := s.onStatus(ctx, ch, &v.Statuses[i]); err != nil {
			return err
		}
	}
	for _, e := range v.Errors {
		s.recordChannelError(ctx, ch.ID, fmt.Sprintf("%s (kod %d) %s", e.Title, e.Code, e.ErrorData.Details))
	}
	return nil
}

func (s *Service) recordChannelError(ctx context.Context, channelID uint, text string) {
	_ = s.db.WithContext(ctx).Exec("UPDATE wa_channels SET last_error = ?, last_error_at = now() WHERE id = ?", text, channelID).Error
	s.alert(ctx, channelID, "WhatsApp cihazında hata: "+text)
}

func (s *Service) onQuality(ctx context.Context, wabaID string, raw json.RawMessage) error {
	var v struct {
		DisplayPhoneNumber string `json:"display_phone_number"`
		Event              string `json:"event"`
		CurrentLimit       string `json:"current_limit"`
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil
	}
	var chans []models.WAChannel
	_ = s.db.WithContext(ctx).Where("waba_id = ?", wabaID).Find(&chans).Error
	for _, ch := range chans {
		if v.DisplayPhoneNumber != "" && digitsOnly(ch.DisplayPhone) != "" && !strings.HasSuffix(digitsOnly(v.DisplayPhoneNumber), digitsOnly(ch.DisplayPhone)) && !strings.HasSuffix(digitsOnly(ch.DisplayPhone), digitsOnly(v.DisplayPhoneNumber)) {
			continue
		}
		_ = s.db.WithContext(ctx).Exec("UPDATE wa_channels SET messaging_limit = ? WHERE id = ?", v.CurrentLimit, ch.ID).Error
		switch v.Event {
		case "DOWNGRADE", "FLAGGED":
			s.alert(ctx, ch.ID, fmt.Sprintf("%s numarasının kalitesi düştü (%s). Gönderim sınırı: %s.", ch.Name, qualityWord(v.Event), v.CurrentLimit))
		case "UPGRADE":
			s.alert(ctx, ch.ID, fmt.Sprintf("%s numarasının gönderim sınırı yükseldi: %s.", ch.Name, v.CurrentLimit))
		}
	}
	return nil
}

func qualityWord(event string) string {
	if event == "FLAGGED" {
		return "işaretlendi"
	}
	return "düşürüldü"
}

func (s *Service) onAccountNotice(ctx context.Context, wabaID, field string, raw json.RawMessage) {
	var chans []models.WAChannel
	_ = s.db.WithContext(ctx).Where("waba_id = ?", wabaID).Find(&chans).Error
	text := strings.TrimSpace(string(raw))
	if len(text) > 400 {
		text = text[:400]
	}
	for _, ch := range chans {
		_ = s.db.WithContext(ctx).Exec("UPDATE wa_channels SET last_error = ?, last_error_at = now() WHERE id = ?", "Meta hesap bildirimi ("+field+"): "+text, ch.ID).Error
		s.alert(ctx, ch.ID, "Meta, "+ch.Name+" hesabı hakkında bir bildirim gönderdi. Cihaz ayarlarında ayrıntısını görebilirsiniz.")
	}
}

func digitsOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// ---------------------------------------------------------------- admin

// EventView is a stored webhook call for the "işlenemeyen bildirimler"
// screen.
type EventView struct {
	ID          uint       `json:"id"`
	ChannelID   *uint      `json:"channelId,omitempty"`
	Status      string     `json:"status"`
	Attempts    int        `json:"attempts"`
	LastError   string     `json:"lastError"`
	ReceivedAt  time.Time  `json:"receivedAt"`
	ProcessedAt *time.Time `json:"processedAt,omitempty"`
	Summary     string     `json:"summary"`
}

// Events lists stored calls that are not done.
func (s *Service) Events(ctx context.Context, actorID uint) ([]EventView, error) {
	if _, err := s.require(ctx, actorID, enums.WAChannelManage, "Bu ekranı görme yetkiniz yok."); err != nil {
		return nil, err
	}
	var list []models.WAWebhookEvent
	if err := s.db.WithContext(ctx).Where("status <> 'done'").Order("id DESC").Limit(200).Find(&list).Error; err != nil {
		return nil, errs.Internal(err)
	}
	out := make([]EventView, 0, len(list))
	for _, e := range list {
		sum := e.Payload
		if len(sum) > 300 {
			sum = sum[:300] + "..."
		}
		out = append(out, EventView{ID: e.ID, ChannelID: e.ChannelID, Status: e.Status, Attempts: e.Attempts, LastError: e.LastError, ReceivedAt: e.ReceivedAt, ProcessedAt: e.ProcessedAt, Summary: sum})
	}
	return out, nil
}

// RetryEvent queues a stored call again.
func (s *Service) RetryEvent(ctx context.Context, actorID, id uint) error {
	if _, err := s.require(ctx, actorID, enums.WAChannelManage, "Bu işlem için yetkiniz yok."); err != nil {
		return err
	}
	if err := s.db.WithContext(ctx).Exec("UPDATE wa_webhook_events SET status = 'pending', next_try_at = now() WHERE id = ? AND status <> 'done'", id).Error; err != nil {
		return errs.Internal(err)
	}
	wake(s.wakeWebhook)
	return nil
}

// hookButton is a tap on a template's quick reply button.
type hookButton struct {
	Payload string `json:"payload"`
	Text    string `json:"text"`
}
