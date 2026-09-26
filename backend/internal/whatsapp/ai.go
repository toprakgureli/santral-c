package whatsapp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
)

// The reply assistant writes a draft answer from the conversation so far,
// or tidies the agent's own draft. It never sends anything: the text lands
// in the agent's box to read and change first. The key is entered in the
// panel and stored encrypted; with no key the feature stays hidden.

const aiKey = "ai"

// AIModel is a model the panel offers.
type AIModel struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

var aiModels = []AIModel{
	{ID: "claude-sonnet-5", Label: "Dengeli (önerilen)"},
	{ID: "claude-haiku-4-5-20251001", Label: "Hızlı ve ucuz"},
	{ID: "claude-opus-5-5", Label: "En güçlü, daha yavaş"},
}

// AISettings is the stored form.
type AISettings struct {
	Enabled         bool   `json:"enabled"`
	Model           string `json:"model"`
	KeyEnc          string `json:"keyEnc,omitempty"`
	Instructions    string `json:"instructions"`
	UseQuickReplies bool   `json:"useQuickReplies"`
}

// AIView is what the settings screen gets; the key never leaves.
type AIView struct {
	Enabled         bool      `json:"enabled"`
	Model           string    `json:"model"`
	Instructions    string    `json:"instructions"`
	UseQuickReplies bool      `json:"useQuickReplies"`
	HasKey          bool      `json:"hasKey"`
	Models          []AIModel `json:"models"`
}

// AIInput is the settings form. APIKey "" keeps the stored key, "-" drops it.
type AIInput struct {
	Enabled         bool   `json:"enabled"`
	Model           string `json:"model"`
	Instructions    string `json:"instructions"`
	UseQuickReplies bool   `json:"useQuickReplies"`
	APIKey          string `json:"apiKey"`
}

func (s *Service) aiSettings(ctx context.Context) AISettings {
	set := AISettings{Model: aiModels[0].ID, UseQuickReplies: true}
	s.loadGlobal(ctx, aiKey, &set)
	if set.Model == "" {
		set.Model = aiModels[0].ID
	}
	return set
}

// AIStatus tells the inbox whether to show the suggestion button.
func (s *Service) AIStatus(ctx context.Context, actorID uint) (map[string]bool, error) {
	u, err := s.users.GetByID(ctx, actorID)
	if err != nil {
		return nil, err
	}
	set := s.aiSettings(ctx)
	return map[string]bool{"available": set.Enabled && set.KeyEnc != "" && u.Can(enums.WAAISuggest)}, nil
}

// AI returns the assistant's settings.
func (s *Service) AI(ctx context.Context, actorID uint) (*AIView, error) {
	if _, err := s.require(ctx, actorID, enums.WAAIManage, "Yapay zekâ ayarlarını görme yetkiniz yok."); err != nil {
		return nil, err
	}
	set := s.aiSettings(ctx)
	return &AIView{Enabled: set.Enabled, Model: set.Model, Instructions: set.Instructions, UseQuickReplies: set.UseQuickReplies, HasKey: set.KeyEnc != "", Models: aiModels}, nil
}

// SaveAI stores the assistant's settings.
func (s *Service) SaveAI(ctx context.Context, actorID uint, in AIInput) (*AIView, error) {
	if _, err := s.require(ctx, actorID, enums.WAAIManage, "Yapay zekâ ayarlarını değiştirme yetkiniz yok."); err != nil {
		return nil, err
	}
	set := s.aiSettings(ctx)
	known := false
	for _, m := range aiModels {
		if m.ID == in.Model {
			known = true
		}
	}
	if !known {
		return nil, errs.Invalid("Model tanınmadı.", nil)
	}
	set.Enabled, set.Model, set.Instructions, set.UseQuickReplies = in.Enabled, in.Model, strings.TrimSpace(in.Instructions), in.UseQuickReplies
	switch k := strings.TrimSpace(in.APIKey); k {
	case "":
	case "-":
		set.KeyEnc = ""
	default:
		enc, err := s.seal(k)
		if err != nil {
			return nil, errs.Internal(err)
		}
		set.KeyEnc = enc
	}
	if set.Enabled && set.KeyEnc == "" {
		return nil, errs.Invalid("Açmak için önce anahtarı girin.", nil)
	}
	if len([]rune(set.Instructions)) > 8000 {
		return nil, errs.Invalid("Şirket bilgisi en fazla 8000 karakter olabilir.", nil)
	}
	if err := s.saveGlobal(ctx, actorID, aiKey, set); err != nil {
		return nil, err
	}
	return s.AI(ctx, actorID)
}

// TestAI asks the model a one-line question to prove the key works.
func (s *Service) TestAI(ctx context.Context, actorID uint) (map[string]string, error) {
	if _, err := s.require(ctx, actorID, enums.WAAIManage, "Yapay zekâ ayarlarını değiştirme yetkiniz yok."); err != nil {
		return nil, err
	}
	set := s.aiSettings(ctx)
	key := s.open(set.KeyEnc)
	if key == "" {
		return nil, errs.Invalid("Önce anahtarı girip kaydedin.", nil)
	}
	out, err := askModel(ctx, key, set.Model, "Kısa cevap ver.", "Merhaba de, tek kelime.", 20)
	if err != nil {
		return nil, errs.Invalid("Bağlanılamadı: "+err.Error(), err)
	}
	return map[string]string{"message": "Çalışıyor. Modelin cevabı: " + out}, nil
}

var aiLast sync.Map // user id -> time of their last request

// Suggest writes a reply for the agent to check, or tidies their draft.
func (s *Service) Suggest(ctx context.Context, actorID, conversationID uint, draft string) (map[string]string, error) {
	v, conv, _, err := s.reachable(ctx, actorID, conversationID)
	if err != nil {
		return nil, err
	}
	if !v.can(enums.WAAISuggest) {
		return nil, errs.Forbidden("Yapay zekâ önerisi alma yetkiniz yok.")
	}
	set := s.aiSettings(ctx)
	key := s.open(set.KeyEnc)
	if !set.Enabled || key == "" {
		return nil, errs.Invalid("Yapay zekâ önerisi kapalı.", nil)
	}
	if last, ok := aiLast.Load(actorID); ok && time.Since(last.(time.Time)) < 3*time.Second {
		return nil, errs.Invalid("Biraz bekleyip yeniden deneyin.", nil)
	}
	aiLast.Store(actorID, time.Now())

	u, err := s.users.GetByID(ctx, actorID)
	if err != nil {
		return nil, err
	}
	var msgs []models.WAMessage
	if err := s.db.WithContext(ctx).Where("conversation_id = ? AND direction IN ('in','out') AND kind <> 'reaction'", conv.ID).
		Order("id DESC").Limit(30).Find(&msgs).Error; err != nil {
		return nil, errs.Internal(err)
	}
	if len(msgs) == 0 && strings.TrimSpace(draft) == "" {
		return nil, errs.Invalid("Öneri için sohbette henüz mesaj yok.", nil)
	}
	people := map[uint]string{}
	var lines []string
	for i := len(msgs) - 1; i >= 0; i-- {
		m := &msgs[i]
		who := "Müşteri"
		if m.Direction == "out" {
			switch m.SenderKind {
			case "bot":
				who = "Chatbot"
			case "automation":
				who = "Otomatik mesaj"
			default:
				who = "Temsilci"
				if m.SenderUserID != nil {
					name, ok := people[*m.SenderUserID]
					if !ok {
						if p, err := s.users.GetByID(ctx, *m.SenderUserID); err == nil {
							name = firstName(p.Name)
						}
						people[*m.SenderUserID] = name
					}
					if name != "" {
						who = "Temsilci " + name
					}
				}
			}
		}
		text := strings.TrimSpace(m.Body)
		if m.Kind != "text" && m.Kind != "template" && m.Kind != "interactive" && m.Kind != "button" {
			text = strings.TrimSpace("[" + kindWord(m.Kind) + "] " + text)
		}
		if text == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("%s (%s): %s", who, m.CreatedAt.In(istanbul).Format("02.01 15:04"), text))
	}

	var sys strings.Builder
	sys.WriteString("Bir müşteri destek ekibinde WhatsApp'tan müşterilere cevap yazan bir temsilciye yardım ediyorsun. ")
	sys.WriteString("Yazdığın metin temsilcinin mesaj kutusuna düşecek; temsilci okuyup gerekirse değiştirip kendisi gönderecek.\n\n")
	sys.WriteString("Kurallar:\n")
	sys.WriteString("- Günlük, sade, kibar Türkçe yaz. Çeviri gibi ya da resmi yazışma gibi durmasın.\n")
	sys.WriteString("- Kısa tut: çoğu zaman bir iki cümle yeter.\n")
	sys.WriteString("- Bilmediğin hiçbir bilgiyi uydurma (fiyat, tarih, sipariş durumu, kampanya, süre). Gerekirse temsilcinin dolduracağı yeri [köşeli parantez] içinde bırak.\n")
	sys.WriteString("- Kendini yapay zekâ ya da asistan olarak tanıtma; temsilcinin ağzından yaz.\n")
	sys.WriteString("- Sohbetin ortasındaysan yeniden selam verme.\n")
	sys.WriteString("- Sadece gönderilecek mesajı yaz; açıklama, seçenek listesi ya da tırnak ekleme.\n")
	sys.WriteString("- Kalın için *yıldız* kullanabilirsin, başka biçim kullanma.\n")
	if set.Instructions != "" {
		sys.WriteString("\nŞirket ve ürünler hakkında bilmen gerekenler:\n")
		sys.WriteString(set.Instructions)
		sys.WriteString("\n")
	}
	if set.UseQuickReplies {
		var qs []models.WAQuickReply
		_ = s.db.WithContext(ctx).Where("channel_ids @> ?::jsonb", fmt.Sprintf("[%d]", conv.ChannelID)).Order("shortcut").Limit(40).Find(&qs).Error
		if len(qs) > 0 {
			sys.WriteString("\nEkibin hazır cevapları (bilgi kaynağı ve üslup örneği olarak kullan):\n")
			for _, q := range qs {
				body := q.Body
				if r := []rune(body); len(r) > 600 {
					body = string(r[:600])
				}
				sys.WriteString("- " + strings.TrimSpace(q.Title+" "+body) + "\n")
			}
		}
	}

	var user strings.Builder
	user.WriteString("Temsilcinin adı: " + firstName(u.Name) + "\n\nSohbet (eskiden yeniye):\n")
	user.WriteString(strings.Join(lines, "\n"))
	if d := strings.TrimSpace(draft); d != "" {
		user.WriteString("\n\nTemsilcinin taslağı:\n" + d + "\n\nBu taslağı anlamını değiştirmeden düzelt: yazım hatalarını gider, daha anlaşılır ve kibar yap. Sadece düzeltilmiş mesajı yaz.")
	} else {
		user.WriteString("\n\nMüşterinin son mesajına temsilcinin göndereceği cevabı yaz.")
	}
	c, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	out, err := askModel(c, key, set.Model, sys.String(), user.String(), 600)
	if err != nil {
		return nil, errs.Invalid("Öneri alınamadı: "+err.Error(), err)
	}
	out = strings.Trim(strings.TrimSpace(out), "\"")
	if out == "" {
		return nil, errs.Invalid("Öneri boş geldi, yeniden deneyin.", nil)
	}
	return map[string]string{"text": out}, nil
}

func kindWord(k string) string {
	switch k {
	case "image":
		return "görsel"
	case "video":
		return "video"
	case "audio":
		return "ses kaydı"
	case "document":
		return "belge"
	case "sticker":
		return "çıkartma"
	case "location":
		return "konum"
	case "contacts":
		return "kişi kartı"
	}
	return "mesaj"
}

var aiClient = &http.Client{Timeout: 70 * time.Second}

// askModel sends one question to the model and returns its text.
func askModel(ctx context.Context, key, model, system, prompt string, maxTokens int) (string, error) {
	body, _ := json.Marshal(map[string]any{
		"model":      model,
		"max_tokens": maxTokens,
		"system":     system,
		"messages":   []map[string]any{{"role": "user", "content": prompt}},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.anthropic.com/v1/messages", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", key)
	req.Header.Set("anthropic-version", "2023-06-01")
	resp, err := aiClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("servise ulaşılamadı")
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var out struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Error *struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.Unmarshal(raw, &out)
	if resp.StatusCode/100 != 2 {
		switch resp.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			return "", fmt.Errorf("anahtar geçersiz")
		case http.StatusTooManyRequests:
			return "", fmt.Errorf("çok fazla istek, biraz sonra deneyin")
		case http.StatusNotFound:
			return "", fmt.Errorf("model bulunamadı")
		}
		if out.Error != nil && out.Error.Message != "" {
			return "", fmt.Errorf("%s", out.Error.Message)
		}
		return "", fmt.Errorf("servis %d döndü", resp.StatusCode)
	}
	var sb strings.Builder
	for _, c := range out.Content {
		if c.Type == "text" {
			sb.WriteString(c.Text)
		}
	}
	return sb.String(), nil
}
