package whatsapp

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
)

// cloudFor builds the Graph API client from a device's credentials.
func (s *Service) cloudFor(ch *models.WAChannel) (*Cloud, error) {
	token := s.open(ch.AccessTokenEnc)
	if token == "" {
		return nil, errors.New("Bu cihazın erişim anahtarı (token) girilmemiş.")
	}
	return &Cloud{PhoneNumberID: ch.PhoneNumberID, WABAID: ch.WABAID, AppID: ch.AppID, Token: token, Version: ch.GraphVersion}, nil
}

func (s *Service) channel(ctx context.Context, id uint) (*models.WAChannel, error) {
	var ch models.WAChannel
	err := s.db.WithContext(ctx).First(&ch, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errs.NotFound("Cihaz bulunamadı.")
	}
	if err != nil {
		return nil, errs.Internal(err)
	}
	return &ch, nil
}

// ChannelView is a device as the settings screen shows it. Secrets never
// leave the server; only whether they are set.
type ChannelView struct {
	ID              uint            `json:"id"`
	Name            string          `json:"name"`
	DisplayPhone    string          `json:"displayPhone"`
	PhoneNumberID   string          `json:"phoneNumberId"`
	WABAID          string          `json:"wabaId"`
	AppID           string          `json:"appId"`
	GraphVersion    string          `json:"graphVersion"`
	HasToken        bool            `json:"hasToken"`
	HasAppSecret    bool            `json:"hasAppSecret"`
	VerifyToken     string          `json:"verifyToken,omitempty"`
	HookPath        string          `json:"hookPath,omitempty"`
	ExistingHookURL string          `json:"existingHookUrl,omitempty"`
	ExistingToken   string          `json:"existingVerifyToken,omitempty"`
	AcceptUnsigned  bool            `json:"acceptUnsigned"`
	SurveyHookPath  string          `json:"surveyHookPath,omitempty"`
	Active          bool            `json:"active"`
	Settings        ChannelSettings `json:"settings"`
	HasSurveySecret bool            `json:"hasSurveySecret"`
	VerifiedName    string          `json:"verifiedName"`
	QualityRating   string          `json:"qualityRating"`
	MessagingLimit  string          `json:"messagingLimit"`
	LastWebhookAt   *time.Time      `json:"lastWebhookAt,omitempty"`
	LastError       string          `json:"lastError,omitempty"`
	LastErrorAt     *time.Time      `json:"lastErrorAt,omitempty"`
	MemberIDs       []uint          `json:"memberIds"`
	CreatedAt       time.Time       `json:"createdAt"`
}

func (s *Service) channelView(ctx context.Context, ch *models.WAChannel, full bool) ChannelView {
	set := parseSettings(ch.Settings)
	v := ChannelView{
		ID: ch.ID, Name: ch.Name, DisplayPhone: ch.DisplayPhone, PhoneNumberID: ch.PhoneNumberID, WABAID: ch.WABAID,
		AppID: ch.AppID, GraphVersion: ch.GraphVersion, HasToken: ch.AccessTokenEnc != "", HasAppSecret: ch.AppSecretEnc != "",
		Active: ch.Active, HasSurveySecret: set.Survey.SecretEnc != "", VerifiedName: ch.VerifiedName,
		QualityRating: ch.QualityRating, MessagingLimit: ch.MessagingLimit, LastWebhookAt: ch.LastWebhookAt,
		LastError: ch.LastError, LastErrorAt: ch.LastErrorAt, CreatedAt: ch.CreatedAt, MemberIDs: []uint{},
		ExistingHookURL: ch.ExistingHookURL, AcceptUnsigned: ch.AcceptUnsigned,
	}
	set.Survey.SecretEnc = ""
	v.Settings = set
	if full {
		v.VerifyToken = ch.VerifyToken
		v.HookPath = "/api/v1/wa/hook/" + ch.HookKey
		v.SurveyHookPath = "/api/v1/wa/survey/" + ch.HookKey
		v.ExistingToken = ch.ExistingVerifyToken
	}
	_ = s.db.WithContext(ctx).Raw("SELECT user_id FROM wa_channel_members WHERE channel_id = ? ORDER BY user_id", ch.ID).Scan(&v.MemberIDs).Error
	if v.MemberIDs == nil {
		v.MemberIDs = []uint{}
	}
	return v
}

// Channels lists the devices. Those who manage devices get everything;
// anyone else gets the devices they work on, for the inbox filters.
func (s *Service) Channels(ctx context.Context, actorID uint) ([]ChannelView, error) {
	v, err := s.viewerOf(ctx, actorID)
	if err != nil {
		return nil, err
	}
	full := v.can(enums.WAChannelManage)
	var list []models.WAChannel
	if err := s.db.WithContext(ctx).Order("id").Find(&list).Error; err != nil {
		return nil, errs.Internal(err)
	}
	out := make([]ChannelView, 0, len(list))
	for i := range list {
		ch := &list[i]
		settingsReader := full || v.can(enums.WASetGeneral) || v.can(enums.WASetGreeting) || v.can(enums.WASetDistribution) || v.can(enums.WASetReadReceipts)
		if !full && !settingsReader && !v.seesChannel(ch.ID) {
			continue
		}
		out = append(out, s.channelView(ctx, ch, full))
	}
	return out, nil
}

// ChannelInput is what the device form sends. Empty secrets keep the
// stored ones.
type ChannelInput struct {
	Name          string `json:"name"`
	DisplayPhone  string `json:"displayPhone"`
	PhoneNumberID string `json:"phoneNumberId"`
	WABAID        string `json:"wabaId"`
	AppID         string `json:"appId"`
	GraphVersion  string `json:"graphVersion"`
	AccessToken   string `json:"accessToken"`
	AppSecret     string `json:"appSecret"`
	Active        *bool  `json:"active"`
	// a webhook already registered in Meta; empty means the panel's own
	ExistingHookURL     string `json:"existingHookUrl"`
	ExistingVerifyToken string `json:"existingVerifyToken"`
	AcceptUnsigned      bool   `json:"acceptUnsigned"`
}

func cleanID(s string) string {
	return strings.TrimSpace(s)
}

// CreateChannel adds a device. It starts from the default settings and no
// chatbot, quick reply or rule of any other device.
func (s *Service) CreateChannel(ctx context.Context, actorID uint, in ChannelInput) (*ChannelView, error) {
	if _, err := s.require(ctx, actorID, enums.WAChannelManage, "Cihaz ekleme yetkiniz yok."); err != nil {
		return nil, err
	}
	in.PhoneNumberID, in.WABAID = cleanID(in.PhoneNumberID), cleanID(in.WABAID)
	if strings.TrimSpace(in.Name) == "" || in.PhoneNumberID == "" || in.WABAID == "" {
		return nil, errs.Invalid("Ad, numara kimliği (Phone number ID) ve işletme hesabı kimliği (WABA ID) zorunlu.", nil)
	}
	if in.AccessToken == "" {
		return nil, errs.Invalid("Erişim anahtarı (token) zorunlu.", nil)
	}
	hookPath, err := existingPath(in.ExistingHookURL)
	if err != nil {
		return nil, err
	}
	if in.AppSecret == "" && !(hookPath != "" && in.AcceptUnsigned) {
		return nil, errs.Invalid("Uygulama gizli anahtarı (App secret) zorunlu. Meta'da kayıtlı bir webhook kullanıyorsanız ve anahtar elinizde değilse imzasız bildirimleri kabul etmeyi seçebilirsiniz.", nil)
	}
	tok, err := s.seal(strings.TrimSpace(in.AccessToken))
	if err != nil {
		return nil, errs.Internal(err)
	}
	sec, err := s.seal(strings.TrimSpace(in.AppSecret))
	if err != nil {
		return nil, errs.Internal(err)
	}
	ch := &models.WAChannel{
		Name: strings.TrimSpace(in.Name), DisplayPhone: strings.TrimSpace(in.DisplayPhone), PhoneNumberID: in.PhoneNumberID,
		WABAID: in.WABAID, AppID: cleanID(in.AppID), GraphVersion: strings.TrimSpace(in.GraphVersion),
		AccessTokenEnc: tok, AppSecretEnc: sec, VerifyToken: randomKey(16), HookKey: randomKey(20),
		Active: true, Settings: defaultSettings().encode(), CreatedBy: uintPtr(actorID),
		ExistingHookURL: strings.TrimSpace(in.ExistingHookURL), ExistingHookPath: hookPath,
		ExistingVerifyToken: strings.TrimSpace(in.ExistingVerifyToken), AcceptUnsigned: hookPath != "" && in.AcceptUnsigned,
	}
	if err := s.db.WithContext(ctx).Create(ch).Error; err != nil {
		if strings.Contains(err.Error(), "wa_channels_phone_number_idx") {
			return nil, errs.Conflict("Bu numara zaten ekli.", err)
		}
		return nil, errs.Internal(err)
	}
	// The one who adds a device works on it from the start.
	_ = s.db.WithContext(ctx).Exec("INSERT INTO wa_channel_members (channel_id, user_id) VALUES (?, ?) ON CONFLICT DO NOTHING", ch.ID, actorID).Error
	s.forget()
	s.forgetHookPaths()
	s.refreshNumber(ctx, ch)
	v := s.channelView(ctx, ch, true)
	return &v, nil
}

// UpdateChannel changes a device's identity or credentials.
func (s *Service) UpdateChannel(ctx context.Context, actorID, id uint, in ChannelInput) (*ChannelView, error) {
	if _, err := s.require(ctx, actorID, enums.WAChannelManage, "Cihaz düzenleme yetkiniz yok."); err != nil {
		return nil, err
	}
	ch, err := s.channel(ctx, id)
	if err != nil {
		return nil, err
	}
	fields := map[string]any{"updated_at": time.Now()}
	if n := strings.TrimSpace(in.Name); n != "" {
		fields["name"] = n
	}
	fields["display_phone"] = strings.TrimSpace(in.DisplayPhone)
	if v := cleanID(in.PhoneNumberID); v != "" {
		fields["phone_number_id"] = v
	}
	if v := cleanID(in.WABAID); v != "" {
		fields["waba_id"] = v
	}
	fields["app_id"] = cleanID(in.AppID)
	fields["graph_version"] = strings.TrimSpace(in.GraphVersion)
	if t := strings.TrimSpace(in.AccessToken); t != "" {
		enc, err := s.seal(t)
		if err != nil {
			return nil, errs.Internal(err)
		}
		fields["access_token_enc"] = enc
	}
	if t := strings.TrimSpace(in.AppSecret); t != "" {
		enc, err := s.seal(t)
		if err != nil {
			return nil, errs.Internal(err)
		}
		fields["app_secret_enc"] = enc
	}
	if in.Active != nil {
		fields["active"] = *in.Active
	}
	hookPath, err := existingPath(in.ExistingHookURL)
	if err != nil {
		return nil, err
	}
	fields["existing_hook_url"] = strings.TrimSpace(in.ExistingHookURL)
	fields["existing_hook_path"] = hookPath
	fields["existing_verify_token"] = strings.TrimSpace(in.ExistingVerifyToken)
	fields["accept_unsigned"] = hookPath != "" && in.AcceptUnsigned
	if hookPath == "" && ch.AppSecretEnc == "" && strings.TrimSpace(in.AppSecret) == "" {
		return nil, errs.Invalid("Panelin kendi webhook adresi için uygulama gizli anahtarı (App secret) gerekli.", nil)
	}
	if err := s.db.WithContext(ctx).Model(&models.WAChannel{}).Where("id = ?", id).Updates(fields).Error; err != nil {
		if strings.Contains(err.Error(), "wa_channels_phone_number_idx") {
			return nil, errs.Conflict("Bu numara başka bir cihazda kayıtlı.", err)
		}
		return nil, errs.Internal(err)
	}
	s.forgetHookPaths()
	ch, _ = s.channel(ctx, id)
	s.refreshNumber(ctx, ch)
	v := s.channelView(ctx, ch, true)
	return &v, nil
}

// DeleteChannel removes a device and everything on it.
func (s *Service) DeleteChannel(ctx context.Context, actorID, id uint) error {
	if _, err := s.require(ctx, actorID, enums.WAChannelManage, "Cihaz silme yetkiniz yok."); err != nil {
		return err
	}
	if err := s.db.WithContext(ctx).Delete(&models.WAChannel{}, id).Error; err != nil {
		return errs.Internal(err)
	}
	s.forget()
	return nil
}

// ChannelCheck is the result of "Bağlantıyı test et".
type ChannelCheck struct {
	OK             bool   `json:"ok"`
	Message        string `json:"message"`
	DisplayPhone   string `json:"displayPhone,omitempty"`
	VerifiedName   string `json:"verifiedName,omitempty"`
	QualityRating  string `json:"qualityRating,omitempty"`
	MessagingLimit string `json:"messagingLimit,omitempty"`
	Subscribed     bool   `json:"subscribed"`
	WebhookSeen    bool   `json:"webhookSeen"`
}

// TestChannel checks the credentials against Meta and reports what it
// found, in words.
func (s *Service) TestChannel(ctx context.Context, actorID, id uint) (*ChannelCheck, error) {
	if _, err := s.require(ctx, actorID, enums.WAChannelManage, "Cihaz yönetme yetkiniz yok."); err != nil {
		return nil, err
	}
	ch, err := s.channel(ctx, id)
	if err != nil {
		return nil, err
	}
	out := &ChannelCheck{WebhookSeen: ch.LastWebhookAt != nil}
	cl, err := s.cloudFor(ch)
	if err != nil {
		out.Message = err.Error()
		return out, nil
	}
	info, err := cl.Number(ctx)
	if err != nil {
		out.Message = "Meta bilgileri kabul etmedi. " + friendlyError(err)
		return out, nil
	}
	out.DisplayPhone, out.VerifiedName, out.QualityRating, out.MessagingLimit = info.DisplayPhoneNumber, info.VerifiedName, info.QualityRating, info.MessagingLimitTier
	out.Subscribed, _ = cl.Subscribed(ctx)
	s.saveNumber(ctx, ch, info)
	out.OK = true
	switch {
	case !out.Subscribed:
		out.Message = "Bilgiler doğru, ama Meta bu hesabın mesajlarını henüz bize göndermiyor. \"Mesajları almaya başla\" düğmesine basın."
	case !out.WebhookSeen:
		out.Message = "Bilgiler doğru. Meta'da webhook adresi ve doğrulama anahtarı girildikten sonra ilk mesaj burada görünür."
	default:
		out.Message = "Her şey yolunda. Mesajlar alınıyor."
	}
	return out, nil
}

// SubscribeChannel asks Meta to send this account's messages to us.
func (s *Service) SubscribeChannel(ctx context.Context, actorID, id uint) error {
	if _, err := s.require(ctx, actorID, enums.WAChannelManage, "Cihaz yönetme yetkiniz yok."); err != nil {
		return err
	}
	ch, err := s.channel(ctx, id)
	if err != nil {
		return err
	}
	cl, err := s.cloudFor(ch)
	if err != nil {
		return errs.Invalid(err.Error(), nil)
	}
	if err := cl.Subscribe(ctx); err != nil {
		return errs.Invalid("Meta isteği kabul etmedi. "+friendlyError(err), err)
	}
	return nil
}

// refreshNumber reads the number's card in the background after a change.
func (s *Service) refreshNumber(ctx context.Context, ch *models.WAChannel) {
	if ch == nil {
		return
	}
	cl, err := s.cloudFor(ch)
	if err != nil {
		return
	}
	go func() {
		c, cancel := context.WithTimeout(context.WithoutCancel(ctx), 20*time.Second)
		defer cancel()
		if info, err := cl.Number(c); err == nil {
			s.saveNumber(c, ch, info)
		}
	}()
}

func (s *Service) saveNumber(ctx context.Context, ch *models.WAChannel, info *NumberInfo) {
	fields := map[string]any{"verified_name": info.VerifiedName, "quality_rating": info.QualityRating, "messaging_limit": info.MessagingLimitTier}
	if ch.DisplayPhone == "" && info.DisplayPhoneNumber != "" {
		fields["display_phone"] = info.DisplayPhoneNumber
	}
	_ = s.db.WithContext(ctx).Model(&models.WAChannel{}).Where("id = ?", ch.ID).Updates(fields).Error
}

// ---------------------------------------------------------------- settings

// SettingsInput is the settings screen's save. SurveySecret, when not
// empty, replaces the stored Tally secret; "-" clears it.
type SettingsInput struct {
	Settings     ChannelSettings `json:"settings"`
	SurveySecret string          `json:"surveySecret"`
}

// UpdateSettings saves a device's settings. Each section needs its own
// permission; a section nobody touched needs none.
func (s *Service) UpdateSettings(ctx context.Context, actorID, id uint, in SettingsInput) (*ChannelView, error) {
	u, err := s.users.GetByID(ctx, actorID)
	if err != nil {
		return nil, err
	}
	ch, err := s.channel(ctx, id)
	if err != nil {
		return nil, err
	}
	cur := parseSettings(ch.Settings)
	next := in.Settings
	next.Survey.SecretEnc = cur.Survey.SecretEnc
	need := func(changed bool, p enums.Permission, what string) error {
		if changed && !u.Can(p) {
			return errs.Forbidden(what + " değiştirme yetkiniz yok.")
		}
		return nil
	}
	if err := need(cur.ReadReceipts != next.ReadReceipts, enums.WASetReadReceipts, "Okundu bilgisi ayarını"); err != nil {
		return nil, err
	}
	if err := need(!reflect.DeepEqual(cur.Greeting, next.Greeting), enums.WASetGreeting, "Karşılama mesajını"); err != nil {
		return nil, err
	}
	if err := need(!reflect.DeepEqual(cur.Distribution, next.Distribution), enums.WASetDistribution, "Dağıtım ayarını"); err != nil {
		return nil, err
	}
	general := cur
	general.ReadReceipts, general.Greeting, general.Distribution = next.ReadReceipts, next.Greeting, next.Distribution
	if err := need(!reflect.DeepEqual(general, next) || in.SurveySecret != "", enums.WASetGeneral, "Cihaz ayarlarını"); err != nil {
		return nil, err
	}
	switch strings.TrimSpace(in.SurveySecret) {
	case "":
	case "-":
		next.Survey.SecretEnc = ""
	default:
		enc, err := s.seal(strings.TrimSpace(in.SurveySecret))
		if err != nil {
			return nil, errs.Internal(err)
		}
		next.Survey.SecretEnc = enc
	}
	if err := validateSettings(&next); err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Model(&models.WAChannel{}).Where("id = ?", id).Updates(map[string]any{"settings": next.encode(), "updated_at": time.Now()}).Error; err != nil {
		return nil, errs.Internal(err)
	}
	ch, _ = s.channel(ctx, id)
	v := s.channelView(ctx, ch, u.Can(enums.WAChannelManage))
	return &v, nil
}

func validateSettings(s *ChannelSettings) error {
	if s.WaitingMinutes < 0 || s.WaitingMinutes > 24*60 {
		return errs.Invalid("Bekleme süresi 0 ile 1440 dakika arasında olmalı.", nil)
	}
	if s.Distribution.MaxOpen < 0 || s.Distribution.MaxOpen > 500 {
		return errs.Invalid("Kişi başı sohbet sınırı 0 ile 500 arasında olmalı.", nil)
	}
	if s.BotTimeoutMinutes <= 0 {
		s.BotTimeoutMinutes = 30
	}
	if s.Greeting.Enabled && strings.TrimSpace(s.Greeting.Text) == "" {
		return errs.Invalid("Karşılama açıksa metni boş olamaz.", nil)
	}
	switch s.Survey.Mode {
	case "", "off":
		s.Survey.Mode = "off"
	case "tally":
		if !strings.HasPrefix(strings.TrimSpace(s.Survey.URL), "https://") {
			return errs.Invalid("Anket linki https:// ile başlamalı.", nil)
		}
	case "native":
	default:
		return errs.Invalid("Anket türü tanınmadı.", nil)
	}
	for i := range s.Hours.Days {
		d := &s.Hours.Days[i]
		if d.Open && minuteOf(d.To) <= minuteOf(d.From) {
			return errs.Invalid(fmt.Sprintf("%s günü için kapanış saati açılıştan sonra olmalı.", dayNames[i]), nil)
		}
	}
	return nil
}

var dayNames = [7]string{"Pazartesi", "Salı", "Çarşamba", "Perşembe", "Cuma", "Cumartesi", "Pazar"}

// CopySettings copies chosen sections of another device's settings onto
// this one. The copy is independent from then on.
func (s *Service) CopySettings(ctx context.Context, actorID, id, from uint, sections []string) (*ChannelView, error) {
	src, err := s.channel(ctx, from)
	if err != nil {
		return nil, err
	}
	dst, err := s.channel(ctx, id)
	if err != nil {
		return nil, err
	}
	a, b := parseSettings(src.Settings), parseSettings(dst.Settings)
	for _, sec := range sections {
		switch sec {
		case "readReceipts":
			b.ReadReceipts = a.ReadReceipts
		case "greeting":
			b.Greeting = a.Greeting
		case "distribution":
			b.Distribution = a.Distribution
		case "waiting":
			b.WaitingMinutes = a.WaitingMinutes
		case "hours":
			b.Hours = a.Hours
		case "survey":
			secret := b.Survey.SecretEnc
			b.Survey = a.Survey
			b.Survey.SecretEnc = secret
		case "bot":
			b.BotTimeoutMinutes, b.HumanKeywords = a.BotTimeoutMinutes, a.HumanKeywords
		case "optout":
			b.OptOutKeywords, b.OptOutReply = a.OptOutKeywords, a.OptOutReply
		}
	}
	b.Survey.SecretEnc = parseSettings(dst.Settings).Survey.SecretEnc
	return s.UpdateSettings(ctx, actorID, id, SettingsInput{Settings: b})
}

// SetMembers replaces who works on a device.
func (s *Service) SetMembers(ctx context.Context, actorID, id uint, userIDs []uint) error {
	if _, err := s.require(ctx, actorID, enums.WATeamManage, "Cihaz üyelerini düzenleme yetkiniz yok."); err != nil {
		return err
	}
	if _, err := s.channel(ctx, id); err != nil {
		return err
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		keep := append([]uint{0}, userIDs...)
		if err := tx.Exec("DELETE FROM wa_channel_members WHERE channel_id = ? AND user_id NOT IN ?", id, keep).Error; err != nil {
			return err
		}
		for _, uid := range userIDs {
			if err := tx.Exec("INSERT INTO wa_channel_members (channel_id, user_id) VALUES (?, ?) ON CONFLICT DO NOTHING", id, uid).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return errs.Internal(err)
	}
	s.forget()
	return nil
}
