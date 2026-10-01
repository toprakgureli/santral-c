package whatsapp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/toprakgureli/santral-c/backend/internal/whatsapp/device"
	"github.com/toprakgureli/santral-c/backend/internal/whatsapp/outside"

	"github.com/toprakgureli/santral-c/backend/internal/whatsapp/varfill"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
)

// Automatic messages are one-step rules: when something happens on a
// device and the conditions hold, the actions run. They are set per
// device; a new device has none.

// RuleCondition is one test of a rule.
type RuleCondition struct {
	Kind  string `json:"kind"` // hours_open | hours_closed | text_contains | text_equals | text_regex | tag_has | status_is | no_owner | after_minutes
	Value string `json:"value"`
}

// RuleAction is one thing a rule does.
type RuleAction struct {
	Kind       string   `json:"kind"` // send_text | send_template | assign_team | assign_user | add_tag | set_priority | set_category | resolve | send_survey | note | webhook
	Text       string   `json:"text,omitempty"`
	TemplateID uint     `json:"templateId,omitempty"`
	Params     []string `json:"params,omitempty"`
	TeamID     uint     `json:"teamId,omitempty"`
	UserID     uint     `json:"userId,omitempty"`
	Value      string   `json:"value,omitempty"`
	URL        string   `json:"url,omitempty"`
}

var ruleTriggers = map[string]string{
	"message_in":      "Müşteri mesaj yazınca",
	"first_message":   "Müşteri ilk kez yazınca",
	"ticket_created":  "Yeni sohbet açılınca",
	"ticket_reopened": "Çözülmüş sohbet yeniden açılınca",
	"ticket_assigned": "Sohbet birine atanınca",
	"ticket_resolved": "Sohbet çözülünce",
	"outside_hours":   "Mesai dışında mesaj gelince",
	"no_reply":        "Müşteri belli bir süre cevap alamayınca",
}

func (s *Service) rulesFor(ctx context.Context, channelID uint, trigger string) []models.WAAutomation {
	list, err := s.repo.RulesFor(ctx, channelID, trigger)
	warnDB(ctx, err)
	return list
}

// runAutomations runs every rule of a trigger for one conversation.
func (s *Service) runAutomations(ctx context.Context, ch *models.WAChannel, trigger string, conv *models.WAConversation, ticket *models.WATicket, msg *models.WAMessage) {
	if ticket == nil {
		return
	}
	for _, r := range s.rulesFor(ctx, ch.ID, trigger) {
		s.runRule(ctx, ch, &r, conv, ticket, msg)
	}
}

func (s *Service) runRule(ctx context.Context, ch *models.WAChannel, r *models.WAAutomation, conv *models.WAConversation, ticket *models.WATicket, msg *models.WAMessage) {
	var conds []RuleCondition
	var acts []RuleAction
	_ = json.Unmarshal([]byte(r.Conditions), &conds)
	_ = json.Unmarshal([]byte(r.Actions), &acts)
	if !s.conditionsHold(ctx, ch, conds, ticket, msg) {
		return
	}
	if r.CooldownMin > 0 {
		recent, err := s.repo.RecentRuleRuns(ctx, r.ID, conv.ContactID, r.CooldownMin)
		warnDB(ctx, err)
		if recent > 0 {
			return
		}
	}
	// The run is written before the rule acts: the cooldown and the timed
	// rules count on it, so a rule whose run cannot be stored does not act,
	// instead of acting again and again.
	runID, err := s.repo.AddRuleRun(ctx, r.ID, conv.ContactID, ticket.ID)
	if err != nil {
		slog.WarnContext(ctx, "whatsapp rule run could not be stored; the rule did not act", "rule", r.ID, "error", err)
		return
	}
	ok, detail := true, ""
	for _, a := range acts {
		if err := s.runAction(ctx, ch, r, a, conv, ticket); err != nil {
			ok, detail = false, err.Error()
			slog.WarnContext(ctx, "whatsapp rule action failed", "rule", r.ID, "action", a.Kind, "error", err)
		}
	}
	if !ok {
		warnDB(ctx, s.repo.FailRuleRun(ctx, runID, detail))
	}
}

func (s *Service) conditionsHold(ctx context.Context, ch *models.WAChannel, conds []RuleCondition, ticket *models.WATicket, msg *models.WAMessage) bool {
	h := device.Parse(ch.Settings).Hours
	text := ""
	if msg != nil {
		text = strings.TrimSpace(msg.Body)
	}
	for _, c := range conds {
		v := strings.TrimSpace(c.Value)
		ok := true
		switch c.Kind {
		case "hours_open":
			ok = !h.Enabled || h.Open(time.Now())
		case "hours_closed":
			ok = h.Enabled && !h.Open(time.Now())
		case "text_contains":
			ok = false
			for _, w := range strings.Split(v, ",") {
				if w = strings.ToLower(strings.TrimSpace(w)); w != "" && strings.Contains(strings.ToLower(text), w) {
					ok = true
				}
			}
		case "text_equals":
			ok = strings.EqualFold(text, v)
		case "text_regex":
			re, err := regexp.Compile("(?i)" + v)
			ok = err == nil && re.MatchString(text)
		case "tag_has":
			ok = false
			for _, t := range parseTags(ticket.Tags) {
				if strings.EqualFold(t, v) {
					ok = true
				}
			}
			if !ok {
				if c2, err := s.repo.Contact(ctx, ticket.ContactID); err == nil {
					for _, t := range parseTags(c2.Tags) {
						if strings.EqualFold(t, v) {
							ok = true
						}
					}
				}
			}
		case "status_is":
			ok = ticket.Status == v
		case "no_owner":
			ok = ticket.OwnerID == nil
		case "after_minutes":
			// used by the timed sweep only
		}
		if !ok {
			return false
		}
	}
	return true
}

func (s *Service) runAction(ctx context.Context, ch *models.WAChannel, r *models.WAAutomation, a RuleAction, conv *models.WAConversation, ticket *models.WATicket) error {
	contact, _ := s.repo.Contact(ctx, conv.ContactID)
	vars := map[string]string{}
	if contact != nil {
		vars["musteri"] = firstName(contactView(contact).Display)
		vars["numara"] = "+" + contact.WAID
	}
	vars["sohbet"] = fmt.Sprint(ticket.Number)
	switch a.Kind {
	case "send_text":
		text := strings.TrimSpace(varfill.Fill(a.Text, vars))
		if text == "" {
			return nil
		}
		// Read the conversation again: the message that triggered the rule
		// may have just opened the window.
		if fresh, _, err := s.repo.Conversation(ctx, conv.ID); err == nil {
			conv = fresh
		}
		if !windowOpen(conv) {
			return fmt.Errorf("müşterinin son mesajının üzerinden 24 saat geçmiş; düz metin gönderilemedi, şablon kullanın")
		}
		s.queueSystem(ctx, ch, conv.ID, ticket.ID, "automation", r.Name, text)
	case "send_template":
		tpl, err := s.repo.Template(ctx, a.TemplateID)
		if err != nil || tpl.Status != "APPROVED" || tpl.WABAID != ch.WABAID {
			return fmt.Errorf("şablon kullanılamıyor")
		}
		params := make([]string, len(a.Params))
		for i, p := range a.Params {
			params[i] = oneLine(varfill.Fill(p, vars))
		}
		obj, preview, err := buildTemplate(tpl, TemplateParams{Body: params})
		if err != nil {
			return err
		}
		s.queueObject(ctx, ch, conv.ID, ticket.ID, "automation", r.Name+" · "+tpl.Name, "template", preview, map[string]any{"type": "template", "template": obj})
	case "assign_team":
		if err := s.repo.SetTicketTeam(ctx, ticket.ID, a.TeamID); err != nil {
			return err
		}
		if ticket.OwnerID == nil {
			s.distribute(ctx, ch, ticket.ID)
		}
		s.publish(ctx, conv.ID, nil, nil)
	case "assign_user":
		before := s.audience(ctx, ticket)
		if err := s.repo.SetOwnerIfNone(ctx, ticket.ID, a.UserID); err != nil {
			return err
		}
		if err := s.repo.AddOwner(ctx, ticket.ID, a.UserID); err != nil {
			return err
		}
		s.publish(ctx, conv.ID, nil, before)
	case "add_tag":
		t := s.repo.Ticket(ctx, ticket.ID)
		if t != nil {
			if err := s.repo.SetTicketTags(ctx, ticket.ID, jsonString(cleanTags(append(parseTags(t.Tags), a.Value)))); err != nil {
				return err
			}
			s.publish(ctx, conv.ID, nil, nil)
		}
	case "set_priority":
		if err := s.repo.SetTicketPriority(ctx, ticket.ID, a.Value); err != nil {
			return err
		}
		s.publish(ctx, conv.ID, nil, nil)
	case "set_category":
		if err := s.repo.SetTicketCategory(ctx, ticket.ID, strings.TrimSpace(a.Value)); err != nil {
			return err
		}
		s.publish(ctx, conv.ID, nil, nil)
	case "note":
		m := &models.WAMessage{ChannelID: ch.ID, ConversationID: conv.ID, TicketID: uintPtr(ticket.ID), Direction: "note", Kind: "text", SenderKind: "automation",
			SenderLabel: r.Name, Body: varfill.Fill(a.Text, vars), Status: "received", CreatedAt: time.Now()}
		if err := s.repo.CreateMessage(ctx, m); err != nil {
			return err
		}
		s.publish(ctx, conv.ID, m, nil)
	case "resolve":
		fresh := s.repo.Ticket(ctx, ticket.ID)
		if fresh != nil && fresh.Status != "resolved" {
			s.event(ctx, nil, conv, ticket.ID, 0, r.Name+" kuralı sohbeti kapattı.")
			return s.resolve(ctx, conv, fresh, 0)
		}
	case "send_survey":
		fresh := s.repo.Ticket(ctx, ticket.ID)
		if fresh != nil {
			owner := uint(0)
			if fresh.OwnerID != nil {
				owner = *fresh.OwnerID
			}
			s.sendSurvey(ctx, ch, conv, fresh, owner)
		}
	case "webhook":
		payload := map[string]any{"rule": r.Name, "ticket": ticket.Number, "channel": ch.Name, "customer": vars["numara"], "status": ticket.Status}
		raw, _ := json.Marshal(payload)
		c, cancel := context.WithTimeout(ctx, 8*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(c, http.MethodPost, a.URL, bytes.NewReader(raw))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := outside.Client.Do(req)
		if err != nil {
			return outside.Explain(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode/100 != 2 {
			return fmt.Errorf("adres %d döndü", resp.StatusCode)
		}
	}
	return nil
}

// sweepTimedRules runs "no reply for N minutes" rules.
func (s *Service) sweepTimedRules(ctx context.Context) {
	rules, err := s.repo.TimedRules(ctx)
	warnDB(ctx, err)
	for i := range rules {
		r := &rules[i]
		var conds []RuleCondition
		_ = json.Unmarshal([]byte(r.Conditions), &conds)
		minutes := 0
		for _, c := range conds {
			if c.Kind == "after_minutes" {
				minutes, _ = strconv.Atoi(strings.TrimSpace(c.Value)) // not a number: no limit
			}
		}
		if minutes <= 0 {
			continue
		}
		for _, chID := range parseIDs(r.ChannelIDs) {
			ch, err := s.repo.Channel(ctx, chID)
			if err != nil {
				continue
			}
			h := device.Parse(ch.Settings).Hours
			list, err := s.repo.AwaitingTickets(ctx, chID)
			warnDB(ctx, err)
			for j := range list {
				t := &list[j]
				if h.Elapsed(*t.AwaitingSince, time.Now()) < time.Duration(minutes)*time.Minute {
					continue
				}
				done, err := s.repo.RuleRunsSince(ctx, r.ID, t.ID, *t.AwaitingSince)
				warnDB(ctx, err)
				if done > 0 {
					continue
				}
				conv, _, err := s.repo.Conversation(ctx, t.ConversationID)
				if err != nil {
					continue
				}
				s.runRule(ctx, ch, r, conv, t, nil)
			}
		}
	}
}

// ---------------------------------------------------------------- managing

// RuleView is a rule for the panel.
type RuleView struct {
	ID          uint            `json:"id"`
	Name        string          `json:"name"`
	Active      bool            `json:"active"`
	ChannelIDs  []uint          `json:"channelIds"`
	Trigger     string          `json:"trigger"`
	Conditions  []RuleCondition `json:"conditions"`
	Actions     []RuleAction    `json:"actions"`
	CooldownMin int             `json:"cooldownMin"`
	Position    int             `json:"position"`
	Runs        int64           `json:"runs"`
	LastRunAt   *time.Time      `json:"lastRunAt,omitempty"`
	LastError   string          `json:"lastError,omitempty"` // why the last run failed, if it did
	UpdatedAt   time.Time       `json:"updatedAt"`
}

func (s *Service) ruleView(ctx context.Context, r *models.WAAutomation) RuleView {
	v := RuleView{ID: r.ID, Name: r.Name, Active: r.Active, ChannelIDs: parseIDs(r.ChannelIDs), Trigger: r.Trigger, CooldownMin: r.CooldownMin, Position: r.Position, UpdatedAt: r.UpdatedAt}
	_ = json.Unmarshal([]byte(r.Conditions), &v.Conditions)
	_ = json.Unmarshal([]byte(r.Actions), &v.Actions)
	if v.Conditions == nil {
		v.Conditions = []RuleCondition{}
	}
	if v.Actions == nil {
		v.Actions = []RuleAction{}
	}
	st, err := s.repo.RuleRunStats(ctx, r.ID)
	warnDB(ctx, err)
	v.Runs, v.LastRunAt = st.N, st.Last
	if last, err := s.repo.LastRuleRun(ctx, r.ID); err == nil && !last.OK {
		v.LastError = last.Detail
		if v.LastError == "" {
			v.LastError = "Bilinmeyen bir hata oldu."
		}
	}
	return v
}

// Rules lists the automatic message rules.
func (s *Service) Rules(ctx context.Context, actorID uint) ([]RuleView, error) {
	if _, err := s.require(ctx, actorID, enums.WAAutomation, "Otomatik mesajları görme yetkiniz yok."); err != nil {
		return nil, err
	}
	list, err := s.repo.Rules(ctx)
	if err != nil {
		return nil, errs.Internal(err)
	}
	out := make([]RuleView, 0, len(list))
	for i := range list {
		out = append(out, s.ruleView(ctx, &list[i]))
	}
	return out, nil
}

// RuleInput is the rule form.
type RuleInput struct {
	Name        string          `json:"name"`
	Active      bool            `json:"active"`
	ChannelIDs  []uint          `json:"channelIds"`
	Trigger     string          `json:"trigger"`
	Conditions  []RuleCondition `json:"conditions"`
	Actions     []RuleAction    `json:"actions"`
	CooldownMin int             `json:"cooldownMin"`
}

func validateRule(in *RuleInput) error {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return errs.Invalid("Kurala bir ad verin.", nil)
	}
	if _, ok := ruleTriggers[in.Trigger]; !ok {
		return errs.Invalid("Ne zaman çalışacağını seçin.", nil)
	}
	if len(in.Actions) == 0 {
		return errs.Invalid("En az bir yapılacak iş ekleyin.", nil)
	}
	if in.Trigger == "no_reply" {
		ok := false
		for _, c := range in.Conditions {
			if c.Kind == "after_minutes" && strings.TrimSpace(c.Value) != "" {
				ok = true
			}
		}
		if !ok {
			return errs.Invalid("Kaç dakika sonra çalışacağını girin.", nil)
		}
	}
	for _, a := range in.Actions {
		if a.Kind == "webhook" && !strings.HasPrefix(a.URL, "https://") {
			return errs.Invalid("Dış adres https:// ile başlamalı.", nil)
		}
	}
	if in.ChannelIDs == nil {
		in.ChannelIDs = []uint{}
	}
	if in.CooldownMin < 0 {
		in.CooldownMin = 0
	}
	return nil
}

// SaveRule creates (id 0) or updates a rule.
func (s *Service) SaveRule(ctx context.Context, actorID, id uint, in RuleInput) (*RuleView, error) {
	if _, err := s.require(ctx, actorID, enums.WAAutomation, "Otomatik mesajları düzenleme yetkiniz yok."); err != nil {
		return nil, err
	}
	if err := validateRule(&in); err != nil {
		return nil, err
	}
	r := &models.WAAutomation{ID: id, Name: in.Name, Active: in.Active, ChannelIDs: jsonString(in.ChannelIDs), Trigger: in.Trigger,
		Conditions: jsonString(in.Conditions), Actions: jsonString(in.Actions), CooldownMin: in.CooldownMin, CreatedBy: uintPtr(actorID), UpdatedAt: time.Now()}
	var err error
	if id == 0 {
		err = s.repo.CreateRule(ctx, r)
	} else {
		err = s.repo.UpdateRule(ctx, id, map[string]any{
			"name": r.Name, "active": r.Active, "channel_ids": r.ChannelIDs, "trigger": r.Trigger, "conditions": r.Conditions,
			"actions": r.Actions, "cooldown_min": r.CooldownMin, "updated_at": time.Now()})
	}
	if err != nil {
		return nil, errs.Internal(err)
	}
	warnDB(ctx, s.repo.ReloadRule(ctx, r))
	v := s.ruleView(ctx, r)
	return &v, nil
}

// DeleteRule removes a rule.
func (s *Service) DeleteRule(ctx context.Context, actorID, id uint) error {
	if _, err := s.require(ctx, actorID, enums.WAAutomation, "Otomatik mesajları düzenleme yetkiniz yok."); err != nil {
		return err
	}
	return s.repo.DeleteRule(ctx, id)
}

// ReorderRules sets the order rules run in.
func (s *Service) ReorderRules(ctx context.Context, actorID uint, ids []uint) error {
	if _, err := s.require(ctx, actorID, enums.WAAutomation, "Otomatik mesajları düzenleme yetkiniz yok."); err != nil {
		return err
	}
	for i, id := range ids {
		if err := s.repo.SetRulePosition(ctx, id, i); err != nil {
			return errs.Internal(err)
		}
	}
	return nil
}
