package whatsapp

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
)

// The satisfaction survey goes out when a ticket is resolved. With Tally
// the link carries the ticket, the agent and the device as hidden fields
// plus a signature, so a score can only land on the ticket it was sent
// for. The native survey asks inside WhatsApp with a 1-5 list.

func (s *Service) surveyToken(ticketID uint) string {
	mac := hmac.New(sha256.New, []byte(s.secret+":survey"))
	fmt.Fprintf(mac, "%d", ticketID)
	return hex.EncodeToString(mac.Sum(nil))[:24]
}

// surveyDue says whether a resolved ticket should get the survey: once per
// resolution, and not again to a customer who got one in the last
// RepeatHours hours (on any conversation). If the survey already went out
// and the customer has not written since (other than answering it), it is
// not sent again either.
func (s *Service) surveyDue(ctx context.Context, conv *models.WAConversation, t *models.WATicket, repeatHours int) bool {
	if repeatHours > 0 {
		var recent int64
		_ = s.db.WithContext(ctx).Raw("SELECT count(*) FROM wa_tickets WHERE contact_id = ? AND survey_sent_at > now() - make_interval(hours => ?)",
			t.ContactID, repeatHours).Scan(&recent).Error
		if recent > 0 {
			return false
		}
	}
	if t.SurveySentAt == nil {
		return true
	}
	var since int64
	_ = s.db.WithContext(ctx).Raw(`SELECT count(*) FROM wa_messages WHERE conversation_id = ? AND direction = 'in' AND created_at > ?
		AND kind <> 'reaction' AND COALESCE(payload->>'id', '') NOT LIKE '%rate-%'`, conv.ID, *t.SurveySentAt).Scan(&since).Error
	return since > 0
}

// sendSurvey sends the device's survey for a resolved ticket.
func (s *Service) sendSurvey(ctx context.Context, ch *models.WAChannel, conv *models.WAConversation, t *models.WATicket, agentID uint) {
	set := parseSettings(ch.Settings).Survey
	if !s.surveyDue(ctx, conv, t, set.RepeatHours) {
		return
	}
	switch set.Mode {
	case "tally":
		link, err := url.Parse(strings.TrimSpace(set.URL))
		if err != nil {
			return
		}
		q := link.Query()
		q.Set("ticket", fmt.Sprint(t.ID))
		q.Set("number", fmt.Sprint(t.Number))
		q.Set("agent", fmt.Sprint(agentID))
		q.Set("channel", fmt.Sprint(ch.ID))
		q.Set("token", s.surveyToken(t.ID))
		link.RawQuery = q.Encode()
		body := s.surveyText(ctx, set.Text, conv, agentID)
		text := strings.ReplaceAll(body, "{link}", link.String())
		if !strings.Contains(body, "{link}") {
			text = strings.TrimSpace(body + " " + link.String())
		}
		if windowOpen(conv) {
			s.queueSystem(ctx, ch, conv.ID, t.ID, "automation", "Değerlendirme anketi", text)
		} else if set.Template != "" {
			var tpl models.WATemplate
			if s.db.WithContext(ctx).Where("waba_id = ? AND name = ? AND status = 'APPROVED'", ch.WABAID, set.Template).First(&tpl).Error != nil {
				return
			}
			obj, preview, err := buildTemplate(&tpl, TemplateParams{Body: []string{link.String()}, Buttons: []string{link.RawQuery}})
			if err != nil {
				return
			}
			s.queueObject(ctx, ch, conv.ID, t.ID, "automation", "Değerlendirme anketi · "+tpl.Name, "template", preview, map[string]any{"type": "template", "template": obj})
		} else {
			return
		}
	case "native":
		s.sendNativeSurvey(ctx, ch, conv, t, s.surveyText(ctx, set.Text, conv, agentID))
	default:
		return
	}
	_ = s.db.WithContext(ctx).Exec("UPDATE wa_tickets SET survey_sent_at = now() WHERE id = ?", t.ID).Error
}

// surveyText fills {musteri} and {temsilci} in the survey message.
func (s *Service) surveyText(ctx context.Context, text string, conv *models.WAConversation, agentID uint) string {
	customer, agent := "", ""
	if c, err := s.contact(ctx, conv.ContactID); err == nil {
		customer = firstName(contactView(c).Display)
	}
	if agentID > 0 {
		if u, err := s.users.GetByID(ctx, agentID); err == nil {
			agent = firstName(u.Name)
		}
	}
	return strings.TrimSpace(strings.NewReplacer("{musteri}", customer, "{temsilci}", agent).Replace(text))
}

// sendNativeSurvey asks for a 1-5 score with a list inside WhatsApp.
func (s *Service) sendNativeSurvey(ctx context.Context, ch *models.WAChannel, conv *models.WAConversation, t *models.WATicket, text string) {
	if !windowOpen(conv) {
		return
	}
	if strings.TrimSpace(text) == "" {
		text = "Görüşmemizi 1 ile 5 arasında puanlar mısınız?"
	}
	opts := []BotOption{
		{ID: fmt.Sprintf("rate-%d-5", t.ID), Label: "5 - Çok iyi"},
		{ID: fmt.Sprintf("rate-%d-4", t.ID), Label: "4 - İyi"},
		{ID: fmt.Sprintf("rate-%d-3", t.ID), Label: "3 - Orta"},
		{ID: fmt.Sprintf("rate-%d-2", t.ID), Label: "2 - Kötü"},
		{ID: fmt.Sprintf("rate-%d-1", t.ID), Label: "1 - Çok kötü"},
	}
	msg := menuMessage("list", text, "Puan ver", opts)
	s.queueObject(ctx, ch, conv.ID, t.ID, "automation", "Değerlendirme anketi", "interactive", text, msg)
	_ = s.db.WithContext(ctx).Exec("UPDATE wa_tickets SET survey_sent_at = now() WHERE id = ?", t.ID).Error
}

// handleSurveyReply records a score picked from the native survey list.
func (s *Service) handleSurveyReply(ctx context.Context, ch *models.WAChannel, conv *models.WAConversation, msg *models.WAMessage) {
	if msg.Kind != "interactive" || msg.Payload == nil {
		return
	}
	var p struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal([]byte(*msg.Payload), &p)
	id := strings.TrimPrefix(p.ID, "opt:")
	if !strings.HasPrefix(id, "rate-") {
		return
	}
	parts := strings.Split(id, "-")
	if len(parts) != 3 {
		return
	}
	tid, err1 := strconv.Atoi(parts[1])
	score, err2 := strconv.Atoi(parts[2])
	if err1 != nil || err2 != nil || score < 1 || score > 5 {
		return
	}
	s.recordRating(ctx, uint(tid), conv.ID, score, "")
	s.queueSystem(ctx, ch, conv.ID, uint(tid), "automation", "Değerlendirme anketi", "Değerlendirmeniz için teşekkür ederiz.")
}

// recordRating stores a score on its ticket and tells the managers when
// it is low.
func (s *Service) recordRating(ctx context.Context, ticketID, conversationID uint, score int, comment string, answers ...RatingAnswer) {
	var t models.WATicket
	if s.db.WithContext(ctx).First(&t, ticketID).Error != nil || t.ConversationID != conversationID && conversationID != 0 {
		return
	}
	if answers == nil {
		answers = []RatingAnswer{}
	}
	if err := s.db.WithContext(ctx).Exec("UPDATE wa_tickets SET rating = ?, rating_comment = ?, rating_answers = ?, rated_at = now() WHERE id = ?", score, strings.TrimSpace(comment), jsonString(answers), ticketID).Error; err != nil {
		return
	}
	conv, _, err := s.loadConv(ctx, t.ConversationID)
	if err != nil {
		return
	}
	line := fmt.Sprintf("Müşteri görüşmeyi %d/5 puanladı%s.", score, answersText(answers))
	if c := strings.TrimSpace(comment); c != "" {
		line += " Yorumu: " + c
	}
	s.event(ctx, nil, conv, ticketID, 0, line)
	if ch, err := s.channel(ctx, t.ChannelID); err == nil {
		if below := parseSettings(ch.Settings).Survey.AlertBelow; below > 0 && lowestScore(score, answers) <= below {
			viewers, _ := s.loadViewers(ctx)
			var ids []uint
			for id, v := range viewers {
				if v.can(enums.WAReports) {
					ids = append(ids, id)
				}
			}
			if t.OwnerID != nil {
				ids = append(ids, *t.OwnerID)
			}
			s.push.Push(ids, Event{Type: "wa.alert", ConversationID: conv.ID, Text: fmt.Sprintf("#%d numaralı sohbet %d/5 puan aldı%s.", t.Number, score, answersText(answers)), Level: "warning"})
		}
	}
	s.publish(ctx, conv.ID, nil, nil)
}

// TallyWebhook receives a Tally form answer.
func (s *Service) TallyWebhook(ctx context.Context, key, signature string, body []byte) error {
	ch, err := s.channelByHook(ctx, key)
	if err != nil {
		return err
	}
	set := parseSettings(ch.Settings).Survey
	secret := s.open(set.SecretEnc)
	if secret != "" {
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(body)
		want := base64.StdEncoding.EncodeToString(mac.Sum(nil))
		if !hmac.Equal([]byte(strings.TrimSpace(signature)), []byte(want)) {
			return errs.Unauthorized("Anket imzası doğrulanamadı.")
		}
	}
	var p struct {
		EventType string `json:"eventType"`
		Data      struct {
			Fields []struct {
				Key   string          `json:"key"`
				Label string          `json:"label"`
				Type  string          `json:"type"`
				Value json.RawMessage `json:"value"`
			} `json:"fields"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &p); err != nil {
		return errs.Invalid("Anket cevabı okunamadı.", err)
	}
	var ticket, call, token string
	var answers []RatingAnswer
	var texts []RatingText
	for _, f := range p.Data.Fields {
		label := strings.ToLower(strings.TrimSpace(f.Label))
		var str string
		_ = json.Unmarshal(f.Value, &str)
		var num float64
		isNum := json.Unmarshal(f.Value, &num) == nil
		switch {
		case label == "ticket":
			ticket = str
		case label == "call":
			call = str
		case label == "token":
			token = str
		case f.Type == "RATING" || f.Type == "LINEAR_SCALE" || (f.Type == "INPUT_NUMBER" && isNum && num >= 1 && num <= 5):
			// every score question counts on its own, under its title
			if isNum && num >= 1 {
				answers = append(answers, RatingAnswer{Question: questionTitle(f.Label, len(answers)+1), Score: min(5, int(num+0.5))})
			}
		case f.Type == "TEXTAREA" || f.Type == "INPUT_TEXT":
			if t := strings.TrimSpace(str); t != "" {
				texts = append(texts, RatingText{Question: strings.TrimSpace(f.Label), Text: t})
			}
		}
	}
	score := overallScore(answers)
	comment := joinTexts(texts)
	if call != "" {
		// the survey after a phone call
		cid, err := strconv.Atoi(call)
		if err != nil || cid <= 0 || !hmac.Equal([]byte(token), []byte(s.callSurveyToken(uint(cid)))) {
			return errs.Unauthorized("Anket bağlantısı bu görüşmeye ait değil.")
		}
		if score >= 1 {
			s.recordCallSurvey(ctx, uint(cid), "", score, comment, answers...)
		}
		return nil
	}
	tid, err := strconv.Atoi(ticket)
	if err != nil || tid <= 0 {
		return errs.Invalid("Ankette sohbet bilgisi yok.", nil)
	}
	if !hmac.Equal([]byte(token), []byte(s.surveyToken(uint(tid)))) {
		return errs.Unauthorized("Anket bağlantısı bu sohbete ait değil.")
	}
	if score < 1 {
		return nil
	}
	s.recordRating(ctx, uint(tid), 0, score, comment, answers...)
	return nil
}

// RatingAnswer is one score question of a survey form and its answer.
type RatingAnswer struct {
	Question string `json:"question"`
	Score    int    `json:"score"`
}

// RatingText is one written answer of a survey form.
type RatingText struct {
	Question string
	Text     string
}

// questionTitle is a question's title as the form gave it, trimmed to fit.
func questionTitle(label string, n int) string {
	t := strings.Join(strings.Fields(label), " ")
	if t == "" {
		return fmt.Sprintf("Soru %d", n)
	}
	if r := []rune(t); len(r) > 80 {
		t = string(r[:80]) + "…"
	}
	return t
}

// overallScore is the rounded average of the answers, 0 when none.
func overallScore(answers []RatingAnswer) int {
	if len(answers) == 0 {
		return 0
	}
	sum := 0
	for _, a := range answers {
		sum += a.Score
	}
	return int(float64(sum)/float64(len(answers)) + 0.5)
}

// lowestScore is the weakest answer, or the overall score when there are
// no separate questions; alerts go by it, so one bad area is not hidden
// by good ones.
func lowestScore(score int, answers []RatingAnswer) int {
	low := score
	for _, a := range answers {
		if a.Score < low {
			low = a.Score
		}
	}
	return low
}

func answersText(answers []RatingAnswer) string {
	if len(answers) < 2 {
		return ""
	}
	parts := make([]string, len(answers))
	for i, a := range answers {
		parts[i] = fmt.Sprintf("%s %d", a.Question, a.Score)
	}
	return " (" + strings.Join(parts, ", ") + ")"
}

// joinTexts puts written answers together; with several, each under its
// question.
func joinTexts(texts []RatingText) string {
	switch len(texts) {
	case 0:
		return ""
	case 1:
		return texts[0].Text
	}
	lines := make([]string, len(texts))
	for i, t := range texts {
		if t.Question != "" {
			lines[i] = t.Question + ": " + t.Text
		} else {
			lines[i] = t.Text
		}
	}
	return strings.Join(lines, "\n")
}
