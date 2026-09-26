package whatsapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
)

// Templates are Meta-approved messages. They are the only way to write to
// a customer after the 24-hour window, and they belong to a business
// account: every device of that account can use them.

func (s *Service) template(ctx context.Context, id uint) (*models.WATemplate, error) {
	var t models.WATemplate
	err := s.db.WithContext(ctx).First(&t, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errs.NotFound("Şablon bulunamadı.")
	}
	if err != nil {
		return nil, errs.Internal(err)
	}
	return &t, nil
}

// TemplateView is a template for the panel.
type TemplateView struct {
	ID             uint            `json:"id"`
	Name           string          `json:"name"`
	Language       string          `json:"language"`
	Category       string          `json:"category"`
	Status         string          `json:"status"`
	Components     json.RawMessage `json:"components"`
	RejectedReason string          `json:"rejectedReason,omitempty"`
	Quality        string          `json:"quality,omitempty"`
	Fill           []string        `json:"fill"` // what fills each body blank when sent
	UpdatedAt      time.Time       `json:"updatedAt"`
}

// fillWords are what a template blank can be filled with on sending.
var fillWords = map[string]bool{"": true, "customer": true, "agent": true, "agent_full": true}

func parseFill(raw string) []string {
	var out []string
	_ = json.Unmarshal([]byte(raw), &out)
	if out == nil {
		out = []string{}
	}
	return out
}

func templateView(t *models.WATemplate) TemplateView {
	return TemplateView{ID: t.ID, Name: t.Name, Language: t.Language, Category: t.Category, Status: t.Status, Components: json.RawMessage(t.Components),
		RejectedReason: t.RejectedReason, Quality: t.Quality, Fill: parseFill(t.Fill), UpdatedAt: t.UpdatedAt}
}

// SetTemplateFill says what fills each blank of a template when it is sent.
func (s *Service) SetTemplateFill(ctx context.Context, actorID, id uint, fill []string) (*TemplateView, error) {
	if _, err := s.require(ctx, actorID, enums.WATemplateManage, "Şablonları düzenleme yetkiniz yok."); err != nil {
		return nil, err
	}
	for _, f := range fill {
		if !fillWords[f] {
			return nil, errs.Invalid("Otomatik doldurma seçeneği tanınmadı.", nil)
		}
	}
	if fill == nil {
		fill = []string{}
	}
	if err := s.db.WithContext(ctx).Exec("UPDATE wa_templates SET fill = ? WHERE id = ?", jsonString(fill), id).Error; err != nil {
		return nil, errs.Internal(err)
	}
	t, err := s.template(ctx, id)
	if err != nil {
		return nil, err
	}
	v := templateView(t)
	return &v, nil
}

// Templates lists a device's templates. Agents see only approved ones.
func (s *Service) Templates(ctx context.Context, actorID, channelID uint) ([]TemplateView, error) {
	v, err := s.viewerOf(ctx, actorID)
	if err != nil {
		return nil, err
	}
	ch, err := s.channel(ctx, channelID)
	if err != nil {
		return nil, err
	}
	manage := v.can(enums.WATemplateManage)
	if !manage && !(v.can(enums.WATemplateSend) && v.seesChannel(ch.ID)) {
		return nil, errs.Forbidden("Şablonları görme yetkiniz yok.")
	}
	q := s.db.WithContext(ctx).Where("waba_id = ?", ch.WABAID)
	if !manage {
		q = q.Where("status = 'APPROVED'")
	}
	var list []models.WATemplate
	if err := q.Order("name, language").Find(&list).Error; err != nil {
		return nil, errs.Internal(err)
	}
	out := make([]TemplateView, 0, len(list))
	for i := range list {
		out = append(out, templateView(&list[i]))
	}
	return out, nil
}

// SyncTemplates reads every template of the device's account from Meta.
func (s *Service) SyncTemplates(ctx context.Context, actorID, channelID uint) (int, error) {
	if _, err := s.require(ctx, actorID, enums.WATemplateManage, "Şablon yönetme yetkiniz yok."); err != nil {
		return 0, err
	}
	ch, err := s.channel(ctx, channelID)
	if err != nil {
		return 0, err
	}
	n, err := s.syncTemplates(ctx, ch)
	if err != nil {
		return 0, errs.Invalid("Şablonlar Meta'dan alınamadı. "+friendlyError(err), err)
	}
	return n, nil
}

func (s *Service) syncTemplates(ctx context.Context, ch *models.WAChannel) (int, error) {
	cl, err := s.cloudFor(ch)
	if err != nil {
		return 0, err
	}
	list, err := cl.Templates(ctx)
	if err != nil {
		return 0, err
	}
	seen := []string{}
	for _, t := range list {
		comps := string(t.Components)
		if comps == "" || comps == "null" {
			comps = "[]"
		}
		if err := s.db.WithContext(ctx).Exec(`INSERT INTO wa_templates (waba_id, meta_id, name, language, category, status, components, rejected_reason, quality, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, now())
			ON CONFLICT (waba_id, name, language) DO UPDATE SET meta_id = EXCLUDED.meta_id, category = EXCLUDED.category, status = EXCLUDED.status,
				components = EXCLUDED.components, rejected_reason = EXCLUDED.rejected_reason, quality = EXCLUDED.quality, updated_at = now()`,
			ch.WABAID, t.ID, t.Name, t.Language, t.Category, t.Status, comps, cleanReason(t.RejectedReason), t.QualityScore.Score).Error; err != nil {
			return 0, err
		}
		seen = append(seen, t.ID)
	}
	if len(seen) > 0 {
		_ = s.db.WithContext(ctx).Exec("DELETE FROM wa_templates WHERE waba_id = ? AND meta_id <> '' AND meta_id NOT IN ?", ch.WABAID, seen).Error
	}
	return len(list), nil
}

func cleanReason(r string) string {
	if r == "NONE" {
		return ""
	}
	return r
}

func (s *Service) syncAllTemplates(ctx context.Context) {
	var chans []models.WAChannel
	_ = s.db.WithContext(ctx).Where("active").Find(&chans).Error
	done := map[string]bool{}
	for i := range chans {
		if done[chans[i].WABAID] {
			continue
		}
		done[chans[i].WABAID] = true
		if _, err := s.syncTemplates(ctx, &chans[i]); err != nil {
			slog.WarnContext(ctx, "whatsapp templates could not be synced", "channel", chans[i].ID, "error", err)
		}
	}
}

// TemplateButton is a button on a template.
type TemplateButton struct {
	Type    string `json:"type"` // QUICK_REPLY | URL | PHONE_NUMBER
	Text    string `json:"text"`
	URL     string `json:"url"`
	Phone   string `json:"phone"`
	Example string `json:"example"`
}

// TemplateInput is the template form.
type TemplateInput struct {
	ChannelID     uint             `json:"channelId"`
	Name          string           `json:"name"`
	Language      string           `json:"language"`
	Category      string           `json:"category"` // MARKETING | UTILITY | AUTHENTICATION
	HeaderFormat  string           `json:"headerFormat"`
	HeaderText    string           `json:"headerText"`
	HeaderExample string           `json:"headerExample"`
	HeaderHandle  string           `json:"headerHandle"`
	Body          string           `json:"body"`
	BodyExamples  []string         `json:"bodyExamples"`
	Footer        string           `json:"footer"`
	Buttons       []TemplateButton `json:"buttons"`
	Fill          []string         `json:"fill"`
}

var (
	templateName = regexp.MustCompile(`^[a-z0-9_]{1,512}$`)
	varPattern   = regexp.MustCompile(`\{\{(\d+)\}\}`)
)

func countVars(text string) int {
	n := 0
	for _, m := range varPattern.FindAllStringSubmatch(text, -1) {
		var k int
		fmt.Sscan(m[1], &k)
		if k > n {
			n = k
		}
	}
	return n
}

// CreateTemplate sends a template to Meta for approval.
func (s *Service) CreateTemplate(ctx context.Context, actorID uint, in TemplateInput) (*TemplateView, error) {
	if _, err := s.require(ctx, actorID, enums.WATemplateManage, "Şablon oluşturma yetkiniz yok."); err != nil {
		return nil, err
	}
	ch, err := s.channel(ctx, in.ChannelID)
	if err != nil {
		return nil, err
	}
	in.Name = strings.ToLower(strings.TrimSpace(in.Name))
	if !templateName.MatchString(in.Name) {
		return nil, errs.Invalid("Şablon adı yalnızca küçük harf, rakam ve alt çizgi içerebilir. Örnek: siparis_hazir", nil)
	}
	if in.Language == "" {
		in.Language = "tr"
	}
	switch in.Category {
	case "MARKETING", "UTILITY", "AUTHENTICATION":
	default:
		return nil, errs.Invalid("Kategori seçin: Pazarlama, Hizmet ya da Doğrulama.", nil)
	}
	body := strings.TrimSpace(in.Body)
	if body == "" {
		return nil, errs.Invalid("Şablon metni boş olamaz.", nil)
	}
	var comps []map[string]any
	switch in.HeaderFormat {
	case "", "NONE":
	case "TEXT":
		h := map[string]any{"type": "HEADER", "format": "TEXT", "text": strings.TrimSpace(in.HeaderText)}
		if countVars(in.HeaderText) > 0 {
			h["example"] = map[string]any{"header_text": []string{in.HeaderExample}}
		}
		comps = append(comps, h)
	case "IMAGE", "VIDEO", "DOCUMENT":
		if in.HeaderHandle == "" {
			return nil, errs.Invalid("Başlık için örnek dosya yükleyin.", nil)
		}
		comps = append(comps, map[string]any{"type": "HEADER", "format": in.HeaderFormat, "example": map[string]any{"header_handle": []string{in.HeaderHandle}}})
	default:
		return nil, errs.Invalid("Başlık türü tanınmadı.", nil)
	}
	b := map[string]any{"type": "BODY", "text": body}
	if n := countVars(body); n > 0 {
		if len(in.BodyExamples) < n {
			return nil, errs.Invalid(fmt.Sprintf("Metinde %d değişken var. Her biri için örnek bir değer girin.", n), nil)
		}
		b["example"] = map[string]any{"body_text": [][]string{in.BodyExamples[:n]}}
	}
	comps = append(comps, b)
	if f := strings.TrimSpace(in.Footer); f != "" {
		comps = append(comps, map[string]any{"type": "FOOTER", "text": f})
	}
	if len(in.Buttons) > 0 {
		var btns []map[string]any
		for _, bt := range in.Buttons {
			text := strings.TrimSpace(bt.Text)
			if text == "" {
				continue
			}
			switch bt.Type {
			case "QUICK_REPLY":
				btns = append(btns, map[string]any{"type": "QUICK_REPLY", "text": text})
			case "URL":
				x := map[string]any{"type": "URL", "text": text, "url": strings.TrimSpace(bt.URL)}
				if countVars(bt.URL) > 0 {
					x["example"] = []string{bt.Example}
				}
				btns = append(btns, x)
			case "PHONE_NUMBER":
				btns = append(btns, map[string]any{"type": "PHONE_NUMBER", "text": text, "phone_number": strings.TrimSpace(bt.Phone)})
			}
		}
		if len(btns) > 0 {
			comps = append(comps, map[string]any{"type": "BUTTONS", "buttons": btns})
		}
	}
	cl, err := s.cloudFor(ch)
	if err != nil {
		return nil, errs.Invalid(err.Error(), nil)
	}
	metaID, status, err := cl.CreateTemplate(ctx, in.Name, in.Language, in.Category, comps)
	if err != nil {
		return nil, errs.Invalid("Meta şablonu kabul etmedi. "+friendlyError(err), err)
	}
	if status == "" {
		status = "PENDING"
	}
	t := &models.WATemplate{WABAID: ch.WABAID, MetaID: metaID, Name: in.Name, Language: in.Language, Category: in.Category, Status: status,
		Components: jsonString(comps), CreatedBy: uintPtr(actorID), UpdatedAt: time.Now()}
	if err := s.db.WithContext(ctx).Exec(`INSERT INTO wa_templates (waba_id, meta_id, name, language, category, status, components, created_by, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, now()) ON CONFLICT (waba_id, name, language) DO UPDATE SET meta_id = EXCLUDED.meta_id, status = EXCLUDED.status,
		components = EXCLUDED.components, category = EXCLUDED.category, updated_at = now()`,
		t.WABAID, t.MetaID, t.Name, t.Language, t.Category, t.Status, t.Components, actorID).Error; err != nil {
		return nil, errs.Internal(err)
	}
	fill := []string{}
	for _, f := range in.Fill {
		if fillWords[f] {
			fill = append(fill, f)
		}
	}
	_ = s.db.WithContext(ctx).Exec("UPDATE wa_templates SET fill = ? WHERE waba_id = ? AND name = ? AND language = ?", jsonString(fill), t.WABAID, t.Name, t.Language).Error
	_ = s.db.WithContext(ctx).Where("waba_id = ? AND name = ? AND language = ?", t.WABAID, t.Name, t.Language).First(t).Error
	v := templateView(t)
	return &v, nil
}

// UploadTemplateMedia uploads an example file for a template header.
func (s *Service) UploadTemplateMedia(ctx context.Context, actorID, channelID uint, mime string, data []byte) (string, error) {
	if _, err := s.require(ctx, actorID, enums.WATemplateManage, "Şablon oluşturma yetkiniz yok."); err != nil {
		return "", err
	}
	ch, err := s.channel(ctx, channelID)
	if err != nil {
		return "", err
	}
	cl, err := s.cloudFor(ch)
	if err != nil {
		return "", errs.Invalid(err.Error(), nil)
	}
	h, err := cl.UploadHandle(ctx, mime, data)
	if err != nil {
		return "", errs.Invalid(err.Error(), err)
	}
	return h, nil
}

// DeleteTemplate removes a template at Meta and here.
func (s *Service) DeleteTemplate(ctx context.Context, actorID, id uint) error {
	if _, err := s.require(ctx, actorID, enums.WATemplateManage, "Şablon silme yetkiniz yok."); err != nil {
		return err
	}
	t, err := s.template(ctx, id)
	if err != nil {
		return err
	}
	var ch models.WAChannel
	if err := s.db.WithContext(ctx).Where("waba_id = ?", t.WABAID).First(&ch).Error; err == nil {
		if cl, err := s.cloudFor(&ch); err == nil {
			if err := cl.DeleteTemplate(ctx, t.Name); err != nil {
				return errs.Invalid("Meta şablonu silmedi. "+friendlyError(err), err)
			}
		}
	}
	if err := s.db.WithContext(ctx).Where("waba_id = ? AND name = ?", t.WABAID, t.Name).Delete(&models.WATemplate{}).Error; err != nil {
		return errs.Internal(err)
	}
	return nil
}

func (s *Service) onTemplateStatus(ctx context.Context, wabaID string, raw json.RawMessage) error {
	var v struct {
		Event                   string `json:"event"`
		MessageTemplateID       any    `json:"message_template_id"`
		MessageTemplateName     string `json:"message_template_name"`
		MessageTemplateLanguage string `json:"message_template_language"`
		Reason                  string `json:"reason"`
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil
	}
	id := fmt.Sprint(v.MessageTemplateID)
	res := s.db.WithContext(ctx).Exec("UPDATE wa_templates SET status = ?, rejected_reason = ?, updated_at = now() WHERE waba_id = ? AND (meta_id = ? OR (name = ? AND language = ?))",
		v.Event, cleanReason(v.Reason), wabaID, id, v.MessageTemplateName, v.MessageTemplateLanguage)
	if res.Error != nil {
		return res.Error
	}
	viewers, _ := s.loadViewers(ctx)
	var ids []uint
	for uid, vw := range viewers {
		if vw.can(enums.WATemplateManage) {
			ids = append(ids, uid)
		}
	}
	text := fmt.Sprintf("%s şablonu %s.", v.MessageTemplateName, templateStatusWord(v.Event))
	if v.Reason != "" && v.Reason != "NONE" {
		text += " Sebep: " + v.Reason
	}
	if len(ids) > 0 {
		s.push.Push(ids, Event{Type: "wa.alert", Text: text, Level: "info"})
	}
	return nil
}

func (s *Service) onTemplateQuality(ctx context.Context, wabaID string, raw json.RawMessage) error {
	var v struct {
		NewQualityScore         string `json:"new_quality_score"`
		MessageTemplateName     string `json:"message_template_name"`
		MessageTemplateLanguage string `json:"message_template_language"`
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil
	}
	return s.db.WithContext(ctx).Exec("UPDATE wa_templates SET quality = ? WHERE waba_id = ? AND name = ? AND language = ?", v.NewQualityScore, wabaID, v.MessageTemplateName, v.MessageTemplateLanguage).Error
}

func templateStatusWord(e string) string {
	switch e {
	case "APPROVED":
		return "onaylandı"
	case "REJECTED":
		return "reddedildi"
	case "PAUSED":
		return "duraklatıldı"
	case "DISABLED":
		return "kapatıldı"
	case "PENDING_DELETION":
		return "silinmek üzere"
	}
	return strings.ToLower(e)
}

// buildTemplate prepares a template for sending and a readable preview.
func buildTemplate(t *models.WATemplate, p TemplateParams) (map[string]any, string, error) {
	var comps []struct {
		Type    string `json:"type"`
		Format  string `json:"format"`
		Text    string `json:"text"`
		Buttons []struct {
			Type  string `json:"type"`
			Text  string `json:"text"`
			URL   string `json:"url"`
			Phone string `json:"phone_number"`
		} `json:"buttons"`
	}
	_ = json.Unmarshal([]byte(t.Components), &comps)
	var out []map[string]any
	var preview []string
	fill := func(text string, vals []string) string {
		return varPattern.ReplaceAllStringFunc(text, func(m string) string {
			var k int
			fmt.Sscan(strings.Trim(m, "{}"), &k)
			if k >= 1 && k <= len(vals) {
				return vals[k-1]
			}
			return m
		})
	}
	params := func(vals []string) []map[string]any {
		var ps []map[string]any
		for _, v := range vals {
			ps = append(ps, map[string]any{"type": "text", "text": v})
		}
		return ps
	}
	for _, c := range comps {
		switch strings.ToUpper(c.Type) {
		case "HEADER":
			switch strings.ToUpper(c.Format) {
			case "TEXT":
				if n := countVars(c.Text); n > 0 {
					if len(p.Header) < n {
						return nil, "", errs.Invalid("Şablon başlığındaki değişkenleri doldurun.", nil)
					}
					out = append(out, map[string]any{"type": "header", "parameters": params(p.Header[:n])})
				}
				preview = append(preview, "*"+fill(c.Text, p.Header)+"*")
			case "IMAGE", "VIDEO", "DOCUMENT":
				kind := strings.ToLower(c.Format)
				var media map[string]any
				switch {
				case p.headerMediaID != "":
					media = map[string]any{"id": p.headerMediaID}
				case strings.TrimSpace(p.HeaderMedia) != "":
					media = map[string]any{"link": strings.TrimSpace(p.HeaderMedia)}
				default:
					return nil, "", errs.Invalid("Bu şablonun başlığı için bir dosya seçin.", nil)
				}
				out = append(out, map[string]any{"type": "header", "parameters": []map[string]any{{"type": kind, kind: media}}})
			}
		case "BODY":
			if n := countVars(c.Text); n > 0 {
				if len(p.Body) < n {
					return nil, "", errs.Invalid(fmt.Sprintf("Şablondaki %d değişkenin hepsini doldurun.", n), nil)
				}
				for i := 0; i < n; i++ {
					if strings.TrimSpace(p.Body[i]) == "" {
						return nil, "", errs.Invalid(fmt.Sprintf("{{%d}} değişkeni boş bırakılamaz.", i+1), nil)
					}
				}
				out = append(out, map[string]any{"type": "body", "parameters": params(p.Body[:n])})
			}
			preview = append(preview, fill(c.Text, p.Body))
		case "FOOTER":
			preview = append(preview, "_"+c.Text+"_")
		case "BUTTONS":
			bi, qi := 0, 0
			for i, b := range c.Buttons {
				if strings.ToUpper(b.Type) == "QUICK_REPLY" && qi < len(p.quickPayloads) {
					out = append(out, map[string]any{"type": "button", "sub_type": "quick_reply", "index": fmt.Sprint(i), "parameters": []map[string]any{{"type": "payload", "payload": p.quickPayloads[qi]}}})
					qi++
				}
				link := ""
				switch strings.ToUpper(b.Type) {
				case "URL":
					link = b.URL
					if countVars(b.URL) > 0 {
						val := ""
						if bi < len(p.Buttons) {
							val = p.Buttons[bi]
						}
						bi++
						out = append(out, map[string]any{"type": "button", "sub_type": "url", "index": fmt.Sprint(i), "parameters": []map[string]any{{"type": "text", "text": val}}})
						link = fill(b.URL, []string{val})
					}
				case "PHONE_NUMBER":
					link = "tel:" + b.Phone
				}
				// The panel draws these lines as the template's buttons:
				// "[Ara](tel:+90...)", "[Siteye git](https://...)", "[Evet]".
				if link != "" {
					preview = append(preview, "["+b.Text+"]("+link+")")
				} else {
					preview = append(preview, "["+b.Text+"]")
				}
			}
		}
	}
	obj := map[string]any{"name": t.Name, "language": map[string]any{"code": t.Language}}
	if len(out) > 0 {
		obj["components"] = out
	}
	return obj, strings.Join(preview, "\n"), nil
}
