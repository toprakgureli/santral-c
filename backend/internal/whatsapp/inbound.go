package whatsapp

import (
	"context"
	"encoding/json"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/toprakgureli/santral-c/backend/internal/whatsapp/device"
	"github.com/toprakgureli/santral-c/backend/internal/whatsapp/meta"
	"github.com/toprakgureli/santral-c/backend/internal/whatsapp/store"

	"gorm.io/gorm"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/pkg/safe"
)

// MediaRef is a media file as stored on a message.
type MediaRef struct {
	MetaID   string `json:"metaId,omitempty"`
	StoreID  string `json:"storeId,omitempty"` // file id in our storage
	Mime     string `json:"mime,omitempty"`
	Name     string `json:"name,omitempty"`
	Size     int64  `json:"size,omitempty"`
	SHA256   string `json:"sha256,omitempty"`
	Voice    bool   `json:"voice,omitempty"`
	Animated bool   `json:"animated,omitempty"`
	Failed   string `json:"failed,omitempty"` // why it could not be kept
}

func unixTime(s string) *time.Time {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n <= 0 {
		return nil
	}
	t := time.Unix(n, 0)
	return &t
}

// inboundShape turns Meta's message into our row's kind, body, media and
// payload.
func inboundShape(m *hookMessage) (kind, body string, media *MediaRef, payload any) {
	pick := func(k string, md *hookMedia) (string, string, *MediaRef, any) {
		if md == nil {
			return k, "", nil, nil
		}
		return k, md.Caption, &MediaRef{MetaID: md.ID, Mime: md.MimeType, SHA256: md.SHA256, Name: md.Filename, Voice: md.Voice, Animated: md.Animated}, nil
	}
	switch m.Type {
	case "text":
		if m.Text != nil {
			return "text", m.Text.Body, nil, nil
		}
	case "image":
		return pick("image", m.Image)
	case "video":
		return pick("video", m.Video)
	case "audio":
		return pick("audio", m.Audio)
	case "document":
		return pick("document", m.Document)
	case "sticker":
		return pick("sticker", m.Sticker)
	case "location":
		if m.Location != nil {
			label := strings.TrimSpace(m.Location.Name + " " + m.Location.Address)
			if label == "" {
				label = "Konum"
			}
			return "location", label, nil, m.Location
		}
	case "contacts":
		return "contacts", "Kişi kartı", nil, m.Contacts
	case "interactive":
		if m.Interactive != nil {
			if r := m.Interactive.ButtonReply; r != nil {
				return "interactive", r.Title, nil, map[string]string{"type": "button", "id": r.ID, "title": r.Title}
			}
			if r := m.Interactive.ListReply; r != nil {
				return "interactive", r.Title, nil, map[string]string{"type": "list", "id": r.ID, "title": r.Title, "description": r.Description}
			}
		}
	case "button":
		if m.Button != nil {
			return "button", m.Button.Text, nil, map[string]string{"payload": m.Button.Payload}
		}
	case "reaction":
		if m.Reaction != nil {
			return "reaction", m.Reaction.Emoji, nil, map[string]string{"messageId": m.Reaction.MessageID, "emoji": m.Reaction.Emoji}
		}
	}
	return "unsupported", "Bu mesaj türü panelde gösterilemiyor, müşterinin telefonunda görülebilir.", nil, nil
}

// inboundResult carries what the transaction learned to the steps after it.
type inboundResult struct {
	msg      *models.WAMessage
	conv     *models.WAConversation
	ticket   *models.WATicket
	contact  *models.WAContact
	created  bool // a new ticket was opened
	reopened bool // a resolved ticket was opened again
	first    bool // the first message ever from this customer on this device
	// a tap on a call survey button: the survey and the button
	surveyID  uint
	surveyIdx int
	// a score picked from the satisfaction survey list: its ticket
	rateTicket uint
	rateScore  int
	// the customer asked to leave marketing messages with this message
	optedOut bool
	// the device is turned off: the message is kept as history only
	inactive bool
	// when the ticket had been resolved before this message reopened it,
	// and who had it when the message came
	resolvedAt     *time.Time
	ownerAtMessage *uint
	// the agent the customer came back to in time, who could not take the
	// chat (off shift, on a break) so it is handed out afresh
	busyOwner *uint
}

// optOutAsked reports whether a customer message asks to leave marketing
// messages: a typed keyword, or a tapped button or list choice whose text or
// payload is one.
func optOutAsked(m *hookMessage, kind, body string, words []string) bool {
	switch kind {
	case "text", "interactive":
		return matchesWord(strings.TrimSpace(body), words)
	case "button":
		if matchesWord(strings.TrimSpace(body), words) {
			return true
		}
		return m.Button != nil && matchesWord(strings.TrimSpace(m.Button.Payload), words)
	}
	return false
}

// ratingAnswer reads a score picked from the satisfaction survey list
// ("rate-<ticket>-<score>").
func ratingAnswer(m *hookMessage) (uint, int, bool) {
	if m.Type != "interactive" || m.Interactive == nil || m.Interactive.ListReply == nil {
		return 0, 0, false
	}
	id := strings.TrimPrefix(m.Interactive.ListReply.ID, "opt:")
	parts := strings.Split(id, "-")
	if len(parts) != 3 || parts[0] != "rate" {
		return 0, 0, false
	}
	tid, err1 := strconv.Atoi(parts[1])
	score, err2 := strconv.Atoi(parts[2])
	if err1 != nil || err2 != nil || tid <= 0 || score < 1 || score > 5 {
		return 0, 0, false
	}
	return uint(tid), score, true
}

func (s *Service) onInbound(ctx context.Context, ch *models.WAChannel, m *hookMessage, profileName string) error {
	kind, body, media, payload := inboundShape(m)
	at := unixTime(m.Timestamp)
	if at == nil {
		t := time.Now()
		at = &t
	}
	res := &inboundResult{}
	err := s.repo.Transaction(ctx, func(tx *gorm.DB) error {
		contact, err := store.UpsertContact(tx, m.From, profileName)
		if err != nil {
			return err
		}
		res.contact = contact
		if len(m.Referral) > 0 && string(m.Referral) != "null" {
			ref := string(m.Referral)
			if err := store.SetContactSource(tx, contact.ID, ref); err != nil {
				return err
			}
		}
		conv, first, err := store.UpsertConversation(tx, ch.ID, contact.ID)
		if err != nil {
			return err
		}
		res.first = first
		msg := &models.WAMessage{
			ChannelID: ch.ID, ConversationID: conv.ID, Direction: "in", Kind: kind, WAMID: strPtr(m.ID),
			SenderKind: "customer", Body: body, Status: "received", WATimestamp: at, CreatedAt: time.Now(),
		}
		if media != nil {
			msg.Media = strPtr(jsonString(media))
		}
		if payload != nil {
			msg.Payload = strPtr(jsonString(payload))
		}
		if len(m.Referral) > 0 && string(m.Referral) != "null" {
			msg.Referral = strPtr(string(m.Referral))
		}
		if m.Context != nil && m.Context.ID != "" {
			msg.ReplyToWAMID = strPtr(m.Context.ID)
		}
		stored, err := store.CreateMessageOnce(tx, msg)
		if err != nil {
			return err
		}
		if stored == 0 || msg.ID == 0 {
			// Seen before: Meta sent it again. Nothing more to do.
			res.msg = nil
			return nil
		}
		res.msg = msg
		// "DUR": leaving marketing messages is stored together with the
		// message, so it can never be lost to a later step failing.
		if optOutAsked(m, kind, body, device.Parse(ch.Settings).OptOutKeywords) {
			if err := store.OptOut(tx, contact.ID); err != nil {
				return err
			}
			res.optedOut = true
		}
		if !ch.Active {
			// A turned-off device keeps what customers still send, but
			// nothing is opened, handed out or answered automatically.
			res.conv, res.inactive = conv, true
			return store.SetLastInbound(tx, conv.ID, *at, msg.ID, time.Now())
		}
		if id, idx, ok := callSurveyAnswer(m); ok {
			// An answer to the survey after a phone call: kept in the
			// conversation, but it opens no support ticket.
			res.conv, res.surveyID, res.surveyIdx = conv, id, idx
			conv.LastInboundAt = at
			return store.SetLastInbound(tx, conv.ID, *at, msg.ID, time.Now())
		}
		if kind == "reaction" {
			res.conv = conv
			return nil
		}
		if tid, score, ok := ratingAnswer(m); ok {
			// A score for a closed conversation: kept in its history, but it
			// does not open the conversation again.
			owner, err := store.TicketConversationID(tx, tid)
			if err != nil {
				return err
			}
			if owner == conv.ID {
				res.conv, res.rateTicket, res.rateScore = conv, tid, score
				conv.LastInboundAt = at
				if err := store.SetMessageTicket(tx, msg.ID, tid); err != nil {
					return err
				}
				msg.TicketID = uintPtr(tid)
				return store.SetLastInbound(tx, conv.ID, *at, msg.ID, time.Now())
			}
		}
		// Ticket: open one, or bring a resolved one back.
		ticket, created, reopened, wasResolved, err := store.TouchTicket(tx, conv, at)
		if err != nil {
			return err
		}
		res.ticket, res.created, res.reopened, res.resolvedAt = ticket, created, reopened, wasResolved
		if reopened && ticket.OwnerID != nil {
			// A returning customer stays with their former agent only when
			// they came back soon enough and that agent can answer now;
			// otherwise the chat is handed out afresh.
			keep, err := keepsOwner(tx, ch, ticket, wasResolved, *at)
			if err != nil {
				return err
			}
			if !keep {
				if err := store.ReleaseOwner(tx, ticket.ID, *ticket.OwnerID); err != nil {
					return err
				}
				if withinReturn(device.Parse(ch.Settings).ReturnMinutes, wasResolved, *at) {
					res.busyOwner = ticket.OwnerID
				}
				ticket.OwnerID = nil
			}
		}
		if err := store.SetMessageTicket(tx, msg.ID, ticket.ID); err != nil {
			return err
		}
		msg.TicketID = uintPtr(ticket.ID)
		if err := store.RecordInbound(tx, conv.ID, *at, msg.ID, time.Now(), ticket.ID); err != nil {
			return err
		}
		// What follows (chatbot, automatic messages) must see this message:
		// it is what opens the 24-hour window.
		conv.LastInboundAt, conv.TicketID = at, uintPtr(ticket.ID)
		res.conv = conv
		// Record the work that follows, so a restart cannot lose it.
		return store.AddInboundJob(tx, store.NewInboundJob{MessageID: msg.ID, ConversationID: conv.ID, Created: res.created, Reopened: res.reopened,
			First: res.first, OptedOut: res.optedOut, ResolvedAt: res.resolvedAt, OwnerID: ticket.OwnerID})
	})
	if err != nil {
		return err
	}
	if res.msg == nil || res.inactive {
		return nil
	}
	if res.surveyID > 0 {
		s.onCallSurveyTap(ctx, ch, res.conv, res.contact, res.surveyID, res.surveyIdx)
		return nil
	}
	if res.rateTicket > 0 {
		s.recordRating(ctx, res.rateTicket, res.conv.ID, res.rateScore, "")
		s.queueSystem(ctx, ch, res.conv.ID, res.rateTicket, "automation", "Değerlendirme anketi", "Değerlendirmeniz için teşekkür ederiz.")
		s.publish(ctx, res.conv.ID, res.msg, nil)
		return nil
	}
	if res.msg.Kind == "reaction" {
		// A reaction is never a message of its own: the message it belongs
		// to goes out again with its reactions.
		s.publishReaction(ctx, res.conv.ID, res.msg)
		return nil
	}
	// The message shows at once; what follows it runs in the follow-up
	// workers, so receiving never waits for a chatbot or an outside system.
	s.publish(ctx, res.conv.ID, res.msg, nil)
	if res.busyOwner != nil {
		s.event(ctx, nil, res.conv, res.ticket.ID, 0, "Müşteri sohbet kapandıktan kısa süre sonra yeniden yazdı. "+s.repo.UserName(ctx, *res.busyOwner)+" şu an müsait olmadığı için sohbet başka birine verilecek.")
	}
	wake(s.wakeInbound)
	return nil
}

// afterInbound runs everything that follows a stored customer message.
// None of it may lose the message, so failures are only logged.
func (s *Service) afterInbound(ctx context.Context, ch *models.WAChannel, res *inboundResult, steps *jobSteps) {
	msg := res.msg
	if msg.Media != nil {
		steps.run(ctx, "media", func() {
			bg, id := context.WithoutCancel(ctx), msg.ID
			safe.Go(bg, "whatsapp keep media", func() { s.keepMedia(bg, ch, id) })
		})
	}
	set := device.Parse(ch.Settings)

	// "DUR": the choice is already stored; confirm it to the customer.
	if res.optedOut && strings.TrimSpace(set.OptOutReply) != "" {
		steps.run(ctx, "optout", func() {
			s.queueSystem(ctx, ch, res.conv.ID, res.ticket.ID, "automation", "Kampanya izni", set.OptOutReply)
		})
	}

	ticket := res.ticket
	handled := false
	switch {
	case res.created || res.reopened:
		steps.run(ctx, "route", func() {
			// Someone already acted on the ticket since the message came
			// (took it, handed it over, closed it): work picked up again
			// after a restart leaves it as they left it.
			if !sameOwner(ticket.OwnerID, res.ownerAtMessage) || (ticket.Status != "open" && ticket.Status != "bot") {
				return
			}
			if s.returnsToAgent(set, res) {
				// Back within the set time: straight to the agent who had it.
				s.event(ctx, nil, res.conv, ticket.ID, 0, "Müşteri sohbet kapandıktan kısa süre sonra yeniden yazdı; sohbet chatbot'a girmeden "+s.repo.UserName(ctx, *ticket.OwnerID)+" ile devam ediyor.")
				return
			}
			if s.startBot(ctx, ch, res.conv, ticket, msg) {
				handled = true
			} else {
				s.distribute(ctx, ch, ticket.ID)
			}
		})
		if res.created {
			steps.run(ctx, "auto-created", func() { s.runAutomations(ctx, ch, "ticket_created", res.conv, ticket, msg) })
		} else {
			steps.run(ctx, "auto-reopened", func() {
				s.runAutomations(ctx, ch, "ticket_reopened", res.conv, ticket, msg)
				s.notifyReopen(ctx, ticket)
			})
		}
		if res.first {
			steps.run(ctx, "auto-first", func() { s.runAutomations(ctx, ch, "first_message", res.conv, ticket, msg) })
		}
	case ticket.Status == "bot":
		steps.run(ctx, "bot", func() { handled = s.continueBot(ctx, ch, res.conv, ticket, msg) })
	}
	// A chat still with the chatbot is the chatbot's, also when its step
	// was done before a restart.
	if ticket.Status == "bot" {
		handled = true
	}
	// A chat an agent closed after reading this message is done: its rules
	// for incoming messages no longer apply.
	if (!handled || ticket.Status != "bot") && ticket.Status != "resolved" {
		steps.run(ctx, "auto-in", func() {
			s.runAutomations(ctx, ch, "message_in", res.conv, ticket, msg)
			if set.Hours.Enabled && !set.Hours.Open(time.Now()) {
				s.runAutomations(ctx, ch, "outside_hours", res.conv, ticket, msg)
			}
		})
	}
	steps.run(ctx, "survey-reply", func() { s.handleSurveyReply(ctx, ch, res.conv, msg) })
	s.publish(ctx, res.conv.ID, nil, nil)
}

// returnsToAgent reports whether a reopened chat goes straight back to the
// agent who had it: the device sets a return time, the customer wrote
// within it of the chat being resolved and the chat still has its agent;
// keepsOwner already took it from one who could not answer.
func (s *Service) returnsToAgent(set device.Settings, res *inboundResult) bool {
	if !res.reopened || res.ticket.OwnerID == nil {
		return false
	}
	return withinReturn(set.ReturnMinutes, res.resolvedAt, arrivedAt(res.msg))
}

// keepsOwner decides, while the customer's message is stored, whether a
// chat it reopens stays with the agent who had it: only when the customer
// wrote within the device's return time of the chat being resolved, and
// that agent is placed on the device, active, on shift and available now.
func keepsOwner(tx *gorm.DB, ch *models.WAChannel, t *models.WATicket, resolvedAt *time.Time, arrived time.Time) (bool, error) {
	if t.OwnerID == nil || !withinReturn(device.Parse(ch.Settings).ReturnMinutes, resolvedAt, arrived) {
		return false, nil
	}
	return store.AgentAvailable(tx, ch.ID, *t.OwnerID)
}

// withinReturn reports whether a customer who wrote at arrived came back
// within the return time of their chat being resolved. The time is measured
// to the message, not to when its follow-up work runs.
func withinReturn(minutes int, resolvedAt *time.Time, arrived time.Time) bool {
	if minutes <= 0 || resolvedAt == nil {
		return false
	}
	return arrived.Sub(*resolvedAt) <= time.Duration(minutes)*time.Minute
}

// arrivedAt is when a customer's message was written: WhatsApp's time, or
// when it was stored.
func arrivedAt(m *models.WAMessage) time.Time {
	if m.WATimestamp != nil {
		return *m.WATimestamp
	}
	return m.CreatedAt
}

func sameOwner(a, b *uint) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func matchesWord(text string, words []string) bool {
	t := strings.ToLower(strings.TrimSpace(text))
	if t == "" {
		return false
	}
	for _, w := range words {
		if strings.ToLower(strings.TrimSpace(w)) == t {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------- statuses

var statusRank = map[string]int{"queued": 0, "sending": 0, "sent": 1, "delivered": 2, "read": 3}

func (s *Service) onStatus(ctx context.Context, ch *models.WAChannel, st *hookStatus) error {
	msg, err := s.repo.MessageByWAMID(ctx, st.ID)
	if err != nil {
		return err
	}
	if msg.ID == 0 {
		// The status came before we stored the message's id; keep it.
		raw, _ := json.Marshal(st)
		if err := s.repo.AddPendingStatus(ctx, st.ID, st.Status, string(raw)); err != nil {
			return err
		}
		// The send may have stored the id in the meantime, after its own
		// look at the waiting statuses; look once more so none is left.
		msg, err = s.repo.MessageByWAMID(ctx, st.ID)
		if err != nil {
			return err
		}
		if msg.ID != 0 {
			s.applyPending(ctx, msg)
		}
		return nil
	}
	s.applyStatus(ctx, msg, st)
	return nil
}

// applyStatus moves a message's ticks forward (never back) and records a
// failure with its reason.
func (s *Service) applyStatus(ctx context.Context, msg *models.WAMessage, st *hookStatus) {
	at := unixTime(st.Timestamp)
	if at == nil {
		t := time.Now()
		at = &t
	}
	fields := map[string]any{}
	if len(st.Pricing) > 0 && string(st.Pricing) != "null" {
		fields["pricing"] = string(st.Pricing)
	}
	switch st.Status {
	case "sent", "delivered", "read":
		if msg.Status != "failed" && statusRank[st.Status] > statusRank[msg.Status] {
			fields["status"] = st.Status
		}
		switch st.Status {
		case "sent":
			if msg.SentAt == nil {
				fields["sent_at"] = *at
			}
		case "delivered":
			if msg.DeliveredAt == nil {
				fields["delivered_at"] = *at
			}
		case "read":
			if msg.ReadAt == nil {
				fields["read_at"] = *at
			}
			if msg.DeliveredAt == nil {
				fields["delivered_at"] = *at
			}
		}
	case "failed":
		code, text := 0, "Mesaj iletilemedi."
		if len(st.Errors) > 0 {
			code = st.Errors[0].Code
			text = meta.Describe(code, st.Errors[0].Title+" "+st.Errors[0].ErrorData.Details)
		}
		fields["status"], fields["failed_at"], fields["error_code"], fields["error_text"] = "failed", *at, code, text
	}
	if len(fields) == 0 {
		return
	}
	if err := s.repo.UpdateMessage(ctx, msg.ID, fields); err != nil {
		slog.WarnContext(ctx, "whatsapp status could not be saved", "message", msg.ID, "error", err)
		return
	}
	// When a customer reads several messages at once, WhatsApp often reports
	// only the last one; the earlier ones that went out are read too.
	if st.Status == "delivered" || st.Status == "read" {
		lower := []string{"sent"}
		if st.Status == "read" {
			lower = []string{"sent", "delivered"}
		}
		warnDB(ctx, s.repo.CatchUpTicks(ctx, msg.ConversationID, msg.ID, st.Status, *at, lower))
	}
	warnDB(ctx, s.repo.ReloadMessage(ctx, msg))
	s.publish(ctx, msg.ConversationID, msg, nil)
}

// applyPending applies statuses that arrived before the message's id.
func (s *Service) applyPending(ctx context.Context, msg *models.WAMessage) {
	if msg.WAMID == nil {
		return
	}
	rows, err := s.repo.TakePendingStatuses(ctx, *msg.WAMID)
	warnDB(ctx, err)
	for _, r := range rows {
		var st hookStatus
		if json.Unmarshal([]byte(r.Payload), &st) == nil {
			s.applyStatus(ctx, msg, &st)
		}
	}
}

// publishReaction sends the reacted-to message again with its reactions.
func (s *Service) publishReaction(ctx context.Context, conversationID uint, reaction *models.WAMessage) {
	var p struct {
		MessageID string `json:"messageId"`
	}
	if reaction.Payload != nil {
		_ = json.Unmarshal([]byte(*reaction.Payload), &p)
	}
	var target *models.WAMessage
	if p.MessageID != "" {
		t, err := s.repo.MessageByWAMID(ctx, p.MessageID)
		warnDB(ctx, err)
		target = t
	}
	// Only a message of the same conversation: a reaction must not show
	// another customer's message to this conversation's people.
	if target == nil || target.ID == 0 || target.ConversationID != conversationID {
		s.publish(ctx, conversationID, nil, nil)
		return
	}
	s.publish(ctx, conversationID, target, nil)
}
