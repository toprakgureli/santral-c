package store

import (
	"context"
	"fmt"
	"time"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
)

// Rules lists every automatic message rule in the order they run.
func (r *Repository) Rules(ctx context.Context) ([]models.WAAutomation, error) {
	var list []models.WAAutomation
	if err := r.db.WithContext(ctx).Order("position, id").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// RulesFor lists the switched on rules of a trigger placed on a device, in
// the order they run.
func (r *Repository) RulesFor(ctx context.Context, channelID uint, trigger string) ([]models.WAAutomation, error) {
	var list []models.WAAutomation
	err := r.db.WithContext(ctx).Where("active AND trigger = ? AND channel_ids @> ?::jsonb", trigger, fmt.Sprintf("[%d]", channelID)).Order("position, id").Find(&list).Error
	return list, err
}

// RulesOnChannel lists every rule placed on a device, switched on or not.
func (r *Repository) RulesOnChannel(ctx context.Context, channelID uint) ([]models.WAAutomation, error) {
	var list []models.WAAutomation
	if err := r.db.WithContext(ctx).Where("channel_ids @> ?::jsonb", fmt.Sprintf("[%d]", channelID)).Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// TimedRules lists the switched on rules that run when a customer gets no
// reply for a while.
func (r *Repository) TimedRules(ctx context.Context) ([]models.WAAutomation, error) {
	var rules []models.WAAutomation
	err := r.db.WithContext(ctx).Where("active AND trigger = 'no_reply'").Find(&rules).Error
	return rules, err
}

// CreateRule stores a new rule and fills in its id.
func (r *Repository) CreateRule(ctx context.Context, rule *models.WAAutomation) error {
	return r.db.WithContext(ctx).Create(rule).Error
}

// UpdateRule changes the given columns of a rule.
func (r *Repository) UpdateRule(ctx context.Context, id uint, fields map[string]any) error {
	return r.db.WithContext(ctx).Model(&models.WAAutomation{}).Where("id = ?", id).Updates(fields).Error
}

// ReloadRule reads a rule again into rule, by its id.
func (r *Repository) ReloadRule(ctx context.Context, rule *models.WAAutomation) error {
	return r.db.WithContext(ctx).First(rule, rule.ID).Error
}

// DeleteRule removes a rule.
func (r *Repository) DeleteRule(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Delete(&models.WAAutomation{}, id).Error
}

// SetRulePosition sets where a rule stands in the running order.
func (r *Repository) SetRulePosition(ctx context.Context, id uint, position int) error {
	return r.db.WithContext(ctx).Exec("UPDATE wa_automations SET position = ? WHERE id = ?", position, id).Error
}

// ---------------------------------------------------------------- runs

// AddRuleRun records that a rule runs for a customer's ticket, as a run
// that worked, and returns the run's id.
func (r *Repository) AddRuleRun(ctx context.Context, ruleID, contactID, ticketID uint) (uint, error) {
	var runID uint
	err := r.db.WithContext(ctx).Raw("INSERT INTO wa_automation_runs (automation_id, contact_id, ticket_id, ok, detail) VALUES (?, ?, ?, true, '') RETURNING id",
		ruleID, contactID, ticketID).Scan(&runID).Error
	return runID, err
}

// FailRuleRun marks a run as one where an action failed; detail says why.
func (r *Repository) FailRuleRun(ctx context.Context, runID uint, detail string) error {
	return r.db.WithContext(ctx).Exec("UPDATE wa_automation_runs SET ok = false, detail = ? WHERE id = ?", detail, runID).Error
}

// RecentRuleRuns counts how often a rule ran for a customer in the last
// minutes.
func (r *Repository) RecentRuleRuns(ctx context.Context, ruleID, contactID uint, minutes int) (int64, error) {
	var recent int64
	err := r.db.WithContext(ctx).Raw("SELECT count(*) FROM wa_automation_runs WHERE automation_id = ? AND contact_id = ? AND created_at > now() - make_interval(mins => ?)",
		ruleID, contactID, minutes).Scan(&recent).Error
	return recent, err
}

// RuleRunsSince counts how often a rule ran on a ticket since a moment.
func (r *Repository) RuleRunsSince(ctx context.Context, ruleID, ticketID uint, since time.Time) (int64, error) {
	var done int64
	err := r.db.WithContext(ctx).Raw("SELECT count(*) FROM wa_automation_runs WHERE automation_id = ? AND ticket_id = ? AND created_at >= ?", ruleID, ticketID, since).Scan(&done).Error
	return done, err
}

// RuleRunStats is how often a rule ran and when it last did.
type RuleRunStats struct {
	N    int64
	Last *time.Time
}

// RuleRunStats counts a rule's runs and finds the last one.
func (r *Repository) RuleRunStats(ctx context.Context, ruleID uint) (RuleRunStats, error) {
	var st RuleRunStats
	err := r.db.WithContext(ctx).Raw("SELECT count(*) AS n, max(created_at) AS last FROM wa_automation_runs WHERE automation_id = ?", ruleID).Scan(&st).Error
	return st, err
}

// RuleRun is the outcome of one run of a rule.
type RuleRun struct {
	OK     bool
	Detail string
}

// LastRuleRun reads the outcome of a rule's latest run; it is the zero
// value when the rule never ran.
func (r *Repository) LastRuleRun(ctx context.Context, ruleID uint) (RuleRun, error) {
	var last RuleRun
	err := r.db.WithContext(ctx).Raw("SELECT ok, detail FROM wa_automation_runs WHERE automation_id = ? ORDER BY id DESC LIMIT 1", ruleID).Scan(&last).Error
	return last, err
}
