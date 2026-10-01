package whatsapp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/internal/whatsapp/hours"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
	"github.com/toprakgureli/santral-c/backend/pkg/phone"
)

// After a phone call the customer can get a short survey on WhatsApp: an
// approved template with answer buttons, or with a link to a Tally form.
// A tap on a button is recorded as the call's score and never opens a
// support conversation, so agents are not bothered by it.

const callSurveyKey = "call_survey"

// CallSurveySettings is the stored form.
type CallSurveySettings struct {
	Enabled      bool     `json:"enabled"`
	ChannelID    uint     `json:"channelId"`
	Template     string   `json:"template"`
	TemplateLang string   `json:"templateLang"`
	Params       []string `json:"params"`       // body blanks in order; {musteri} {temsilci} {tarih} {link}
	Mode         string   `json:"mode"`         // buttons | link
	ButtonScores []int    `json:"buttonScores"` // score of each answer button, in order
	LinkURL      string   `json:"linkUrl"`      // the Tally form, for link mode
	Directions   string   `json:"directions"`   // inbound | outbound | both
	MinSeconds   int      `json:"minSeconds"`
	DelayMinutes int      `json:"delayMinutes"`
	QuietDays    int      `json:"quietDays"`
	AlertBelow   int      `json:"alertBelow"`
	ThankYou     string   `json:"thankYou"`
}

func defaultCallSurvey() CallSurveySettings {
	return CallSurveySettings{Mode: "buttons", ButtonScores: []int{5, 3, 1}, Directions: "both", MinSeconds: 30, DelayMinutes: 2, QuietDays: 7, AlertBelow: 2,
		ThankYou: "Değerlendirmeniz için teşekkür ederiz."}
}

func (s *Service) callSurveySettings(ctx context.Context) CallSurveySettings {
	set := defaultCallSurvey()
	s.loadGlobal(ctx, callSurveyKey, &set)
	if len(set.ButtonScores) == 0 {
		set.ButtonScores = []int{5, 3, 1}
	}
	if set.Params == nil {
		// The settings screen reads the list; an empty one must not be null.
		set.Params = []string{}
	}
	return set
}

// CallSurvey returns the settings.
func (s *Service) CallSurvey(ctx context.Context, actorID uint) (*CallSurveySettings, error) {
	if _, err := s.require(ctx, actorID, enums.WACallSurvey, "Çağrı sonrası anket ayarlarını görme yetkiniz yok."); err != nil {
		return nil, err
	}
	set := s.callSurveySettings(ctx)
	return &set, nil
}

// SaveCallSurvey stores the settings after checking the template fits.
func (s *Service) SaveCallSurvey(ctx context.Context, actorID uint, in CallSurveySettings) (*CallSurveySettings, error) {
	if _, err := s.require(ctx, actorID, enums.WACallSurvey, "Çağrı sonrası anket ayarlarını değiştirme yetkiniz yok."); err != nil {
		return nil, err
	}
	switch in.Mode {
	case "buttons", "link":
	default:
		return nil, errs.Invalid("Anket türünü seçin.", nil)
	}
	switch in.Directions {
	case "inbound", "outbound", "both":
	default:
		in.Directions = "both"
	}
	if in.MinSeconds < 0 || in.MinSeconds > 3600 {
		return nil, errs.Invalid("En kısa görüşme süresi 0 ile 3600 saniye arasında olmalı.", nil)
	}
	if in.DelayMinutes < 0 || in.DelayMinutes > 24*60 {
		return nil, errs.Invalid("Bekleme süresi 0 ile 1440 dakika arasında olmalı.", nil)
	}
	if in.QuietDays < 0 || in.QuietDays > 365 {
		return nil, errs.Invalid("Tekrar sormama süresi 0 ile 365 gün arasında olmalı.", nil)
	}
	for i, sc := range in.ButtonScores {
		if sc < 1 || sc > 5 {
			return nil, errs.Invalid(fmt.Sprintf("%d. düğmenin puanı 1 ile 5 arasında olmalı.", i+1), nil)
		}
	}
	if in.Enabled {
		ch, err := s.repo.Channel(ctx, in.ChannelID)
		if err != nil {
			return nil, errs.Invalid("Anketin gideceği WhatsApp numarasını seçin.", nil)
		}
		tpl, err := s.repo.ApprovedTemplateIn(ctx, ch.WABAID, in.Template, in.TemplateLang)
		if err != nil {
			return nil, errs.Invalid("Onaylı bir şablon seçin.", nil)
		}
		quick, urlVar := templateButtons(tpl)
		if in.Mode == "buttons" && quick == 0 {
			return nil, errs.Invalid("Bu şablonda cevap düğmesi yok. Düğmeli anket için hızlı cevap düğmeli bir şablon seçin.", nil)
		}
		if in.Mode == "link" {
			if !strings.HasPrefix(strings.TrimSpace(in.LinkURL), "https://") {
				return nil, errs.Invalid("Anket linki https:// ile başlamalı.", nil)
			}
			usesLink := urlVar
			for _, p := range in.Params {
				if strings.Contains(p, "{link}") {
					usesLink = true
				}
			}
			if !usesLink {
				return nil, errs.Invalid("Link müşteriye ulaşmıyor: şablonda değişkenli bir link düğmesi olmalı ya da bir boşluğa {link} yazılmalı.", nil)
			}
		}
		if n := bodyVars(tpl); len(in.Params) < n {
			return nil, errs.Invalid(fmt.Sprintf("Şablondaki %d boşluğun hepsini doldurun.", n), nil)
		}
	}
	in.LinkURL = strings.TrimSpace(in.LinkURL)
	in.ThankYou = strings.TrimSpace(in.ThankYou)
	if err := s.saveGlobal(ctx, actorID, callSurveyKey, in); err != nil {
		return nil, err
	}
	return s.CallSurvey(ctx, actorID)
}

func templateButtons(t *models.WATemplate) (quick int, urlVar bool) {
	for _, c := range parseComponents(t) {
		if strings.ToUpper(c.Type) != "BUTTONS" {
			continue
		}
		for _, b := range c.Buttons {
			switch strings.ToUpper(b.Type) {
			case "QUICK_REPLY":
				quick++
			case "URL":
				if countVars(b.URL) > 0 {
					urlVar = true
				}
			}
		}
	}
	return
}

func bodyVars(t *models.WATemplate) int {
	for _, c := range parseComponents(t) {
		if strings.ToUpper(c.Type) == "BODY" {
			return countVars(c.Text)
		}
	}
	return 0
}

type tplComponent struct {
	Type    string `json:"type"`
	Format  string `json:"format"`
	Text    string `json:"text"`
	Buttons []struct {
		Type string `json:"type"`
		Text string `json:"text"`
		URL  string `json:"url"`
	} `json:"buttons"`
}

func parseComponents(t *models.WATemplate) []tplComponent {
	var out []tplComponent
	_ = json.Unmarshal([]byte(t.Components), &out)
	return out
}

// callSurveyLinkPurpose names the key that signs after-call survey links.
const callSurveyLinkPurpose = "call-survey-link"

func (s *Service) callSurveyToken(id uint) string {
	return linkToken(s.ring.MACKeys(callSurveyLinkPurpose)[0], fmt.Sprint(id))
}

// callSurveyTokenOK checks an after-call survey link, older links included.
func (s *Service) callSurveyTokenOK(id uint, token string) bool {
	keys := append(s.ring.MACKeys(callSurveyLinkPurpose), []byte(s.secret+":call"))
	return linkTokenOK(keys, fmt.Sprint(id), token)
}

// mobileWAID returns the WhatsApp id for a number that can have WhatsApp:
// Turkish mobiles and foreign numbers; Turkish landlines and extensions
// are left out.
func mobileWAID(raw string) (string, bool) {
	e164, err := phone.Normalize(raw)
	if err != nil {
		return "", false
	}
	d := strings.TrimPrefix(e164, "+")
	if len(d) < 10 {
		return "", false
	}
	if strings.HasPrefix(d, "90") && !strings.HasPrefix(d, "905") {
		return "", false
	}
	return d, true
}

// OnCallEnded hears every finished phone call and queues a survey when the
// settings ask for one.
func (s *Service) OnCallEnded(ctx context.Context, log models.CallLog) {
	set := s.callSurveySettings(ctx)
	if !set.Enabled || log.UserID == nil || log.Disposition != "answered" || log.DurationSeconds < set.MinSeconds {
		return
	}
	if set.Directions != "both" && set.Directions != log.Direction {
		return
	}
	// Only the calls of people whose role allows it get a survey.
	if u, err := s.users.GetByID(ctx, *log.UserID); err != nil || !u.Can(enums.WASurveyMyCalls) {
		return
	}
	waID, ok := mobileWAID(log.PeerNumber)
	if !ok {
		return
	}
	key := phone.Key(waID)
	if c, err := s.repo.ContactByWAID(ctx, waID); err == nil && c.Blocked {
		return
	}
	status, note := "queued", ""
	if set.QuietDays > 0 {
		n, err := s.repo.RecentCallSurveys(ctx, key, time.Now().AddDate(0, 0, -set.QuietDays))
		warnDB(ctx, err)
		if n > 0 {
			status, note = "skipped", fmt.Sprintf("Son %d günde zaten soruldu", set.QuietDays)
		}
	}
	row := models.WACallSurvey{CallID: log.CallID, UserID: log.UserID, PeerKey: key, WAID: waID, ChannelID: uintPtr(set.ChannelID), Direction: log.Direction,
		TalkSeconds: log.DurationSeconds, Status: status, Note: note, SendAt: time.Now().Add(time.Duration(set.DelayMinutes) * time.Minute), CreatedAt: time.Now()}
	if err := s.repo.QueueCallSurvey(ctx, &row); err != nil {
		slog.WarnContext(ctx, "call survey could not be queued", "call", log.CallID, "error", err)
	}
}

// staleCallSurvey is how long a survey may stay 'sending' before it is
// taken to have been cut off.
const staleCallSurvey = 10 * time.Minute

// sendDueCallSurveys sends the surveys whose time has come. Taking marks
// them 'sending' in the same statement, with rows another worker holds
// skipped, so a survey is taken once; one whose outcome could not be stored
// stays taken and is never sent a second time.
func (s *Service) sendDueCallSurveys(ctx context.Context) {
	s.flagStaleCallSurveys(ctx)
	rows, err := s.repo.TakeDueCallSurveys(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "call surveys could not be taken", "error", err)
		return
	}
	if len(rows) == 0 {
		return
	}
	set := s.callSurveySettings(ctx)
	for i := range rows {
		r := &rows[i]
		if !set.Enabled {
			s.finishCallSurvey(ctx, r.ID, "skipped", "Anket kapatıldı", nil, nil)
			continue
		}
		if r.UserID != nil {
			if u, err := s.users.GetByID(ctx, *r.UserID); err != nil || !u.Can(enums.WASurveyMyCalls) {
				s.finishCallSurvey(ctx, r.ID, "skipped", "Kişinin anket yetkisi kaldırıldı", nil, nil)
				continue
			}
		}
		if err := s.sendCallSurvey(ctx, set, r); err != nil {
			slog.WarnContext(ctx, "call survey could not be sent", "survey", r.ID, "error", err)
			s.finishCallSurvey(ctx, r.ID, "failed", truncate(err.Error(), 250), nil, nil)
		}
	}
}

// flagStaleCallSurveys marks surveys cut off while being sent as failed
// rather than sending them again.
func (s *Service) flagStaleCallSurveys(ctx context.Context) {
	n, err := s.repo.FailStaleCallSurveys(ctx, "Gönderilip gönderilmediği anlaşılamadı", time.Now().Add(-staleCallSurvey))
	if err != nil {
		slog.ErrorContext(ctx, "stale call surveys could not be checked", "error", err)
		return
	}
	if n > 0 {
		slog.WarnContext(ctx, "call surveys were cut off while sending", "count", n)
	}
}

// finishCallSurvey stores the outcome of a survey this sender took.
func (s *Service) finishCallSurvey(ctx context.Context, id uint, status, note string, convID, msgID *uint) {
	fields := map[string]any{"status": status, "note": note}
	if status == "sent" {
		fields["sent_at"] = time.Now()
	}
	if convID != nil {
		fields["conversation_id"] = *convID
	}
	if msgID != nil {
		fields["message_id"] = *msgID
	}
	if err := s.repo.FinishCallSurvey(ctx, id, fields); err != nil {
		slog.ErrorContext(ctx, "call survey outcome could not be stored; it stays taken and is not sent again", "survey", id, "status", status, "error", err)
	}
}

func (s *Service) sendCallSurvey(ctx context.Context, set CallSurveySettings, r *models.WACallSurvey) error {
	ch, err := s.repo.Channel(ctx, set.ChannelID)
	if err != nil || !ch.Active {
		return fmt.Errorf("anket numarası bulunamadı ya da kapalı")
	}
	tpl, err := s.repo.ApprovedTemplateIn(ctx, ch.WABAID, set.Template, set.TemplateLang)
	if err != nil {
		return fmt.Errorf("şablon onaylı değil ya da silinmiş")
	}
	contact, conv, err := s.repo.EnsureConversation(ctx, ch.ID, r.WAID)
	if err != nil {
		return err
	}
	if contact.Blocked || (contact.OptedOut && tpl.Category == "MARKETING") {
		s.finishCallSurvey(ctx, r.ID, "skipped", "Müşteri mesaj almak istemiyor", &conv.ID, nil)
		return nil
	}
	agent := ""
	if r.UserID != nil {
		if u, err := s.users.GetByID(ctx, *r.UserID); err == nil {
			agent = firstName(u.Name)
		}
	}
	customer := firstName(contactView(contact).Display)
	if customer == "" || strings.HasPrefix(customer, "+") {
		customer = "Değerli müşterimiz"
	}
	link := ""
	query := ""
	if set.Mode == "link" {
		u, err := url.Parse(set.LinkURL)
		if err != nil {
			return fmt.Errorf("anket linki okunamadı")
		}
		q := u.Query()
		q.Set("call", fmt.Sprint(r.ID))
		if r.UserID != nil {
			q.Set("agent", fmt.Sprint(*r.UserID))
		}
		q.Set("token", s.callSurveyToken(r.ID))
		u.RawQuery = q.Encode()
		link, query = u.String(), u.RawQuery
	}
	fill := strings.NewReplacer("{musteri}", customer, "{temsilci}", agent, "{tarih}", r.CreatedAt.In(hours.Zone).Format("02.01.2006"), "{link}", link)
	params := TemplateParams{}
	for _, p := range set.Params {
		v := oneLine(fill.Replace(p))
		if v == "" {
			v = "-"
		}
		params.Body = append(params.Body, v)
	}
	if query != "" {
		params.Buttons = []string{query}
	}
	if set.Mode == "buttons" {
		quick, _ := templateButtons(tpl)
		for i := 0; i < quick; i++ {
			params.quickPayloads = append(params.quickPayloads, fmt.Sprintf("csv:%d:%d", r.ID, i))
		}
	}
	obj, preview, err := buildTemplate(tpl, params)
	if err != nil {
		return err
	}
	ticket := &models.WATicket{}
	if conv.TicketID != nil {
		if t := s.repo.Ticket(ctx, *conv.TicketID); t != nil {
			ticket = t
		}
	}
	msg := &models.WAMessage{ChannelID: ch.ID, ConversationID: conv.ID, Direction: "out", Kind: "template", SenderKind: "automation",
		SenderLabel: "Çağrı sonrası anket", Body: preview, Status: "queued", CreatedAt: time.Now()}
	if ticket.ID > 0 && ticket.Status != "resolved" {
		msg.TicketID = uintPtr(ticket.ID)
	}
	if _, err := s.enqueue(ctx, ch, conv, ticket, msg, map[string]any{"type": "template", "template": obj}, 0); err != nil {
		return err
	}
	s.finishCallSurvey(ctx, r.ID, "sent", "", &conv.ID, &msg.ID)
	return nil
}

// callSurveyAnswer reads a tap on a survey button: "csv:<survey>:<button>".
func callSurveyAnswer(m *hookMessage) (uint, int, bool) {
	if m.Type != "button" || m.Button == nil || !strings.HasPrefix(m.Button.Payload, "csv:") {
		return 0, 0, false
	}
	parts := strings.Split(m.Button.Payload, ":")
	if len(parts) != 3 {
		return 0, 0, false
	}
	id, err1 := strconv.Atoi(parts[1])
	idx, err2 := strconv.Atoi(parts[2])
	if err1 != nil || err2 != nil || id <= 0 || idx < 0 {
		return 0, 0, false
	}
	return uint(id), idx, true
}

// recordCallSurvey stores a score for a call, from a button or the form.
func (s *Service) recordCallSurvey(ctx context.Context, id uint, waID string, score int, comment string, answers ...RatingAnswer) (*models.WACallSurvey, bool) {
	r, err := s.repo.LoadCallSurvey(ctx, id)
	if err != nil {
		return nil, false
	}
	if waID != "" && r.WAID != waID {
		return nil, false
	}
	if score < 1 || score > 5 {
		return nil, false
	}
	first := r.AnsweredAt == nil
	if err := s.repo.UpdateCallSurvey(ctx, id,
		map[string]any{"status": "answered", "score": score, "comment": strings.TrimSpace(comment), "answers": jsonString(append([]RatingAnswer{}, answers...)), "answered_at": time.Now()}); err != nil {
		return nil, false
	}
	set := s.callSurveySettings(ctx)
	if first && set.AlertBelow > 0 && lowestScore(score, answers) <= set.AlertBelow {
		viewers, _ := s.loadViewers(ctx)
		var ids []uint
		for uid, v := range viewers {
			if v.can(enums.WAReports) {
				ids = append(ids, uid)
			}
		}
		agent := "Bir temsilci"
		if r.UserID != nil {
			if u, err := s.users.GetByID(ctx, *r.UserID); err == nil {
				agent = u.Name
			}
		}
		text := fmt.Sprintf("%s, +%s ile yaptığı telefon görüşmesi için %d/5 puan aldı.%s", agent, r.WAID, score, weakestText(answers))
		if c := strings.TrimSpace(comment); c != "" {
			text += " Yorum: " + c
		}
		ev := Event{Type: "wa.alert", Text: text, Level: "warning"}
		if r.ConversationID != nil {
			ev.ConversationID = *r.ConversationID
		}
		s.push.Push(ids, ev)
	}
	return r, first
}

// onCallSurveyTap handles a button tap after the message is stored.
func (s *Service) onCallSurveyTap(ctx context.Context, ch *models.WAChannel, conv *models.WAConversation, contact *models.WAContact, id uint, idx int) {
	set := s.callSurveySettings(ctx)
	if idx >= len(set.ButtonScores) {
		return
	}
	if _, first := s.recordCallSurvey(ctx, id, contact.WAID, set.ButtonScores[idx], ""); first && set.ThankYou != "" {
		s.queueSystem(ctx, ch, conv.ID, 0, "automation", "Çağrı sonrası anket", set.ThankYou)
	}
}

// ---------------------------------------------------------------- report

// CallSurveyAgent is one agent's survey results.
type CallSurveyAgent struct {
	User     PersonView `json:"user"`
	Sent     int64      `json:"sent"`
	Answered int64      `json:"answered"`
	Average  float64    `json:"average"`
	Low      int64      `json:"low"`
}

// CallSurveyAnswer is one answered survey.
type CallSurveyAnswer struct {
	ID             uint      `json:"id"`
	Agent          string    `json:"agent"`
	Phone          string    `json:"phone"`
	Score          int       `json:"score"`
	Comment        string    `json:"comment"`
	ConversationID uint      `json:"conversationId"`
	AnsweredAt     time.Time `json:"answeredAt"`
}

// CallSurveyReport is the report block.
type CallSurveyReport struct {
	Queued   int64              `json:"queued"`
	Sent     int64              `json:"sent"`
	Answered int64              `json:"answered"`
	Failed   int64              `json:"failed"`
	Skipped  int64              `json:"skipped"`
	Average  float64            `json:"average"`
	Agents   []CallSurveyAgent  `json:"agents"`
	Recent   []CallSurveyAnswer `json:"recent"`
}

// CallSurveyReport summarises the surveys of calls in the day range.
func (s *Service) CallSurveyReport(ctx context.Context, actorID uint, fromDay, toDay string) (*CallSurveyReport, error) {
	if _, err := s.require(ctx, actorID, enums.WAReports, "WhatsApp raporlarını görme yetkiniz yok."); err != nil {
		return nil, err
	}
	from, err := time.ParseInLocation("2006-01-02", fromDay, hours.Zone)
	if err != nil {
		return nil, errs.Invalid("Başlangıç tarihi geçersiz.", err)
	}
	toStart, err := time.ParseInLocation("2006-01-02", toDay, hours.Zone)
	if err != nil || toStart.Before(from) {
		return nil, errs.Invalid("Bitiş tarihi geçersiz.", err)
	}
	to := toStart.AddDate(0, 0, 1)
	out := &CallSurveyReport{Agents: []CallSurveyAgent{}, Recent: []CallSurveyAnswer{}}
	totals, err := s.repo.CallSurveyTotals(ctx, from, to)
	if err != nil {
		return nil, errs.Internal(err)
	}
	out.Queued, out.Sent, out.Answered, out.Failed, out.Skipped, out.Average = totals.Queued, totals.Sent, totals.Answered, totals.Failed, totals.Skipped, totals.Average

	agents, err := s.repo.CallSurveyAgents(ctx, from, to)
	if err != nil {
		return nil, errs.Internal(err)
	}
	ids := []uint{}
	for _, a := range agents {
		ids = append(ids, a.UserID)
	}
	people := s.people(ctx, ids)
	for _, a := range agents {
		p := people[a.UserID]
		out.Agents = append(out.Agents, CallSurveyAgent{User: p, Sent: a.Sent, Answered: a.Answered, Average: a.Average, Low: a.Low})
	}
	recent, err := s.repo.RecentCallSurveyAnswers(ctx, from, to)
	if err != nil {
		return nil, errs.Internal(err)
	}
	rids := []uint{}
	for _, r := range recent {
		if r.UserID != nil {
			rids = append(rids, *r.UserID)
		}
	}
	rpeople := s.people(ctx, rids)
	for _, r := range recent {
		a := CallSurveyAnswer{ID: r.ID, Phone: r.WAID, Score: r.Score, Comment: r.Comment, AnsweredAt: r.AnsweredAt}
		if r.UserID != nil {
			a.Agent = rpeople[*r.UserID].Name
		}
		if r.ConversationID != nil {
			a.ConversationID = *r.ConversationID
		}
		out.Recent = append(out.Recent, a)
	}
	return out, nil
}
