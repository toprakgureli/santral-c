package store

import (
	"context"
	"time"

	"gorm.io/gorm/clause"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
)

// LoadCallSurvey reads an after-call survey.
func (r *Repository) LoadCallSurvey(ctx context.Context, id uint) (*models.WACallSurvey, error) {
	var cs models.WACallSurvey
	if err := r.db.WithContext(ctx).First(&cs, id).Error; err != nil {
		return nil, err
	}
	return &cs, nil
}

// RecentCallSurveys counts the surveys of a number queued, being sent,
// sent or answered since a moment.
func (r *Repository) RecentCallSurveys(ctx context.Context, peerKey string, since time.Time) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&models.WACallSurvey{}).
		Where("peer_key = ? AND status IN ('queued','sending','sent','answered') AND created_at > ?", peerKey, since).Count(&n).Error
	return n, err
}

// QueueCallSurvey stores a survey for a call, unless the call has one already.
func (r *Repository) QueueCallSurvey(ctx context.Context, row *models.WACallSurvey) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "call_id"}}, DoNothing: true}).Create(row).Error
}

// TakeDueCallSurveys marks up to 20 queued surveys whose time has come as
// being sent and returns them. Rows another worker holds are skipped, so a
// survey is taken once.
func (r *Repository) TakeDueCallSurveys(ctx context.Context) ([]models.WACallSurvey, error) {
	var rows []models.WACallSurvey
	err := r.db.WithContext(ctx).Raw(`WITH due AS (
			SELECT id FROM wa_call_surveys WHERE status = 'queued' AND send_at <= now()
			ORDER BY send_at LIMIT 20 FOR UPDATE SKIP LOCKED)
		UPDATE wa_call_surveys SET status = 'sending', claimed_at = now()
		FROM due WHERE wa_call_surveys.id = due.id
		RETURNING wa_call_surveys.*`).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// FailStaleCallSurveys marks the surveys taken for sending before a moment
// as failed with a note, and returns how many it marked.
func (r *Repository) FailStaleCallSurveys(ctx context.Context, note string, takenBefore time.Time) (int64, error) {
	res := r.db.WithContext(ctx).Exec(`UPDATE wa_call_surveys SET status = 'failed', note = ?
		WHERE status = 'sending' AND claimed_at < ?`, note, takenBefore)
	return res.RowsAffected, res.Error
}

// FinishCallSurvey changes the given columns of a survey that is still
// being sent.
func (r *Repository) FinishCallSurvey(ctx context.Context, id uint, fields map[string]any) error {
	return r.db.WithContext(ctx).Model(&models.WACallSurvey{}).Where("id = ? AND status = 'sending'", id).Updates(fields).Error
}

// UpdateCallSurvey changes the given columns of a survey.
func (r *Repository) UpdateCallSurvey(ctx context.Context, id uint, fields map[string]any) error {
	return r.db.WithContext(ctx).Model(&models.WACallSurvey{}).Where("id = ?", id).Updates(fields).Error
}

// SetCallSurveyTexts stores the written answers of a survey form as JSON.
func (r *Repository) SetCallSurveyTexts(ctx context.Context, id uint, texts string) error {
	return r.db.WithContext(ctx).Exec("UPDATE wa_call_surveys SET texts = ? WHERE id = ?", texts, id).Error
}

// ---------------------------------------------------------------- report

// CallSurveyTotals counts surveys by outcome and averages their scores.
type CallSurveyTotals struct {
	Queued, Sent, Answered, Failed, Skipped int64
	Average                                 float64
}

// CallSurveyTotals counts the surveys of calls made in a time range.
func (r *Repository) CallSurveyTotals(ctx context.Context, from, to time.Time) (CallSurveyTotals, error) {
	var totals CallSurveyTotals
	err := r.db.WithContext(ctx).Raw(`SELECT
		count(*) FILTER (WHERE cs.status IN ('queued','sending')) AS queued,
		count(*) FILTER (WHERE cs.status IN ('sent','answered') AND COALESCE(m.status,'') <> 'failed') AS sent,
		count(*) FILTER (WHERE cs.status = 'answered') AS answered,
		count(*) FILTER (WHERE cs.status = 'failed' OR m.status = 'failed') AS failed,
		count(*) FILTER (WHERE cs.status = 'skipped') AS skipped,
		COALESCE(avg(cs.score) FILTER (WHERE cs.score IS NOT NULL), 0) AS average
		FROM wa_call_surveys cs LEFT JOIN wa_messages m ON m.id = cs.message_id
		WHERE cs.created_at >= ? AND cs.created_at < ?`, from, to).Scan(&totals).Error
	return totals, err
}

// CallSurveyAgent is one agent's survey results.
type CallSurveyAgent struct {
	UserID   uint
	Sent     int64
	Answered int64
	Average  float64
	Low      int64
}

// CallSurveyAgents sums the surveys of calls in a time range by agent,
// the agent with the most answers first.
func (r *Repository) CallSurveyAgents(ctx context.Context, from, to time.Time) ([]CallSurveyAgent, error) {
	var agents []CallSurveyAgent
	if err := r.db.WithContext(ctx).Raw(`SELECT cs.user_id,
		count(*) FILTER (WHERE cs.status IN ('sent','answered')) AS sent,
		count(*) FILTER (WHERE cs.status = 'answered') AS answered,
		COALESCE(avg(cs.score) FILTER (WHERE cs.score IS NOT NULL), 0) AS average,
		count(*) FILTER (WHERE cs.score <= 2) AS low
		FROM wa_call_surveys cs WHERE cs.user_id IS NOT NULL AND cs.created_at >= ? AND cs.created_at < ?
		GROUP BY cs.user_id ORDER BY answered DESC`, from, to).Scan(&agents).Error; err != nil {
		return nil, err
	}
	return agents, nil
}

// CallSurveyAnswer is one answered survey.
type CallSurveyAnswer struct {
	ID             uint
	UserID         *uint
	WAID           string
	Score          int
	Comment        string
	ConversationID *uint
	AnsweredAt     time.Time
}

// RecentCallSurveyAnswers reads the 30 latest answers to the surveys of
// calls in a time range, latest first.
func (r *Repository) RecentCallSurveyAnswers(ctx context.Context, from, to time.Time) ([]CallSurveyAnswer, error) {
	var recent []CallSurveyAnswer
	if err := r.db.WithContext(ctx).Raw(`SELECT id, user_id, wa_id, score, comment, conversation_id, answered_at FROM wa_call_surveys
		WHERE status = 'answered' AND created_at >= ? AND created_at < ? ORDER BY answered_at DESC LIMIT 30`, from, to).Scan(&recent).Error; err != nil {
		return nil, err
	}
	return recent, nil
}
