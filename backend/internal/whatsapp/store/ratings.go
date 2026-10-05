package store

import (
	"context"
	"fmt"
	"time"
)

// singleQuestion is how a one-question survey (the list inside WhatsApp,
// the buttons after a call) is named among the form's questions.
const singleQuestion = "Tek soruluk anket"

// ratingsSQL is every score in one list. Chat scores belong to whoever
// closed the conversation, or else to its owner. Each row carries its
// answers per question; a one-question survey counts as "Tek soruluk anket".
// The slots are: what to select, what to join (the answers), and the tail.
const ratingsSQL = `
WITH r AS (
	SELECT 'chat' AS source, t.id, t.rated_at AS at, t.rating AS score, t.rating_comment AS comment,
		t.conversation_id, t.number AS ticket_number, t.channel_id, COALESCE(t.resolved_by, t.owner_id) AS agent_id,
		c.wa_id, COALESCE(NULLIF(c.name, ''), NULLIF(c.profile_name, ''), '') AS name, 0 AS talk_seconds,
		CASE WHEN jsonb_array_length(t.rating_answers) > 0 THEN t.rating_answers
			ELSE jsonb_build_array(jsonb_build_object('question', '` + singleQuestion + `', 'score', t.rating)) END AS answers,
		t.rating_texts AS texts
	FROM wa_tickets t JOIN wa_contacts c ON c.id = t.contact_id
	WHERE t.rating IS NOT NULL AND t.rated_at >= @from AND t.rated_at < @to
	UNION ALL
	SELECT 'call', s.id, s.answered_at, s.score, s.comment,
		s.conversation_id, NULL, s.channel_id, s.user_id,
		s.wa_id, COALESCE((SELECT COALESCE(NULLIF(c.name, ''), NULLIF(c.profile_name, ''), '') FROM wa_contacts c WHERE c.wa_id = s.wa_id LIMIT 1), ''), s.talk_seconds,
		CASE WHEN jsonb_array_length(s.answers) > 0 THEN s.answers
			ELSE jsonb_build_array(jsonb_build_object('question', '` + singleQuestion + `', 'score', s.score)) END,
		s.texts
	FROM wa_call_surveys s
	WHERE s.status = 'answered' AND s.score IS NOT NULL AND s.answered_at >= @from AND s.answered_at < @to
)
SELECT %s FROM r %s WHERE
	(@channel = 0 OR r.channel_id = @channel)
	AND (@agent = 0 OR r.agent_id = @agent)
	AND (@source = '' OR r.source = @source)
	AND (@lo = 0 OR r.score BETWEEN @lo AND @hi)
	AND (NOT @comment OR r.comment <> '')
	AND (@q = '' OR r.name ILIKE @like OR r.wa_id LIKE @digits OR r.comment ILIKE @like)
%s`

// answerJoin turns each score row into one row per answered question.
const answerJoin = "CROSS JOIN LATERAL jsonb_array_elements(r.answers) a"

// The functions below take the filter as named values: from and to (the
// time range), channel and agent (zero for any), source ("chat", "call" or
// empty for both), lo and hi (the score range, lo zero for any), comment
// (only scores with a comment), q, like and digits (the search).

// Rating is one score with what it belongs to.
type Rating struct {
	Source string
	// ID is the ticket of a chat score, the survey of a call score.
	ID             uint
	At             time.Time
	Score          int
	Comment        string
	ConversationID *uint
	TicketNumber   *int64
	ChannelID      *uint
	AgentID        *uint
	WAID           string
	Name           string
	TalkSeconds    int
	Answers        string
	Texts          string
}

// ratingColumns are the columns of one score row.
const ratingColumns = `r.source, r.id, r.at, r.score, r.comment, r.conversation_id, r.ticket_number, r.channel_id, r.agent_id, r.wa_id, r.name, r.talk_seconds, r.answers::text AS answers, r.texts::text AS texts`

// Ratings reads the scores matching the filter, latest first, a page of
// limit rows after skipping offset.
func (r *Repository) Ratings(ctx context.Context, args map[string]any, limit, offset int) ([]Rating, error) {
	return r.ratingRows(ctx, args, fmt.Sprintf(" ORDER BY r.at DESC LIMIT %d OFFSET %d", limit, offset))
}

// LatestRatings reads up to limit scores matching the filter, latest first.
func (r *Repository) LatestRatings(ctx context.Context, args map[string]any, limit int) ([]Rating, error) {
	return r.ratingRows(ctx, args, fmt.Sprintf(" ORDER BY r.at DESC LIMIT %d", limit))
}

func (r *Repository) ratingRows(ctx context.Context, args map[string]any, tail string) ([]Rating, error) {
	var rows []Rating
	if err := r.db.WithContext(ctx).Raw(fmt.Sprintf(ratingsSQL, ratingColumns, "", tail), args).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// RatingTotals counts the scores matching the filter, those with a
// comment and each score, and averages them.
type RatingTotals struct {
	Count, WithComment, S1, S2, S3, S4, S5 int64
	Average                                float64
}

// RatingTotals sums the scores matching the filter.
func (r *Repository) RatingTotals(ctx context.Context, args map[string]any) (RatingTotals, error) {
	var tot RatingTotals
	err := r.db.WithContext(ctx).Raw(fmt.Sprintf(ratingsSQL, `count(*) AS count, COALESCE(avg(r.score), 0) AS average,
		count(*) FILTER (WHERE r.comment <> '') AS with_comment,
		count(*) FILTER (WHERE r.score = 1) AS s1, count(*) FILTER (WHERE r.score = 2) AS s2, count(*) FILTER (WHERE r.score = 3) AS s3,
		count(*) FILTER (WHERE r.score = 4) AS s4, count(*) FILTER (WHERE r.score = 5) AS s5`, "", ""), args).Scan(&tot).Error
	return tot, err
}

// RatingAgent is one person's scores.
type RatingAgent struct {
	AgentID uint
	Count   int64
	Average float64
	Low     int64
}

// RatingAgents sums the scores matching the filter by person, the one with
// the most scores first.
func (r *Repository) RatingAgents(ctx context.Context, args map[string]any) ([]RatingAgent, error) {
	var agents []RatingAgent
	if err := r.db.WithContext(ctx).Raw(fmt.Sprintf(ratingsSQL, `r.agent_id, count(*) AS count, avg(r.score) AS average, count(*) FILTER (WHERE r.score <= 2) AS low`,
		"", ` AND r.agent_id IS NOT NULL GROUP BY r.agent_id ORDER BY count(*) DESC`), args).Scan(&agents).Error; err != nil {
		return nil, err
	}
	return agents, nil
}

// RatingQuestion is the answers to one question.
type RatingQuestion struct {
	Question           string
	Count              int64
	Average            float64
	S1, S2, S3, S4, S5 int64
}

// RatingQuestions sums the answers of the scores matching the filter by
// question, the most answered first.
func (r *Repository) RatingQuestions(ctx context.Context, args map[string]any) ([]RatingQuestion, error) {
	var qs []RatingQuestion
	if err := r.db.WithContext(ctx).Raw(fmt.Sprintf(ratingsSQL, `a->>'question' AS question, count(*) AS count, avg((a->>'score')::int) AS average,
		count(*) FILTER (WHERE (a->>'score')::int = 1) AS s1, count(*) FILTER (WHERE (a->>'score')::int = 2) AS s2,
		count(*) FILTER (WHERE (a->>'score')::int = 3) AS s3, count(*) FILTER (WHERE (a->>'score')::int = 4) AS s4,
		count(*) FILTER (WHERE (a->>'score')::int = 5) AS s5`, answerJoin, ` GROUP BY 1 ORDER BY count(*) DESC, 1`), args).Scan(&qs).Error; err != nil {
		return nil, err
	}
	return qs, nil
}

// RatingAgentQuestion is one person's answers to one question.
type RatingAgentQuestion struct {
	AgentID  uint
	Question string
	Count    int64
	Average  float64
}

// RatingAgentQuestions sums the answers of the scores matching the filter
// by person and question.
func (r *Repository) RatingAgentQuestions(ctx context.Context, args map[string]any) ([]RatingAgentQuestion, error) {
	var aq []RatingAgentQuestion
	if err := r.db.WithContext(ctx).Raw(fmt.Sprintf(ratingsSQL, `r.agent_id, a->>'question' AS question, count(*) AS count, avg((a->>'score')::int) AS average`,
		answerJoin, ` AND r.agent_id IS NOT NULL GROUP BY 1, 2`), args).Scan(&aq).Error; err != nil {
		return nil, err
	}
	return aq, nil
}

// RemoveTicketRating takes a chat score out of the ratings: its values move
// to rating_removed with who removed it, and the score is emptied. It
// reports the removed score, or false when the ticket has none.
func (r *Repository) RemoveTicketRating(ctx context.Context, id, by uint) (int, bool, error) {
	var score []int
	err := r.db.WithContext(ctx).Raw(`UPDATE wa_tickets SET
		rating_removed = jsonb_build_object('score', rating, 'comment', rating_comment, 'answers', rating_answers,
			'texts', rating_texts, 'ratedAt', rated_at, 'removedBy', ?::bigint, 'removedAt', now()),
		rating = NULL, rating_comment = '', rating_answers = '[]', rating_texts = '[]', rated_at = NULL
		WHERE id = ? AND rating IS NOT NULL
		RETURNING (rating_removed->>'score')::int`, by, id).Scan(&score).Error
	if err != nil || len(score) == 0 {
		return 0, false, err
	}
	return score[0], true, nil
}

// RemoveCallSurveyRating does the same for the survey after a call; the
// survey is marked removed, so it no longer counts as answered either.
func (r *Repository) RemoveCallSurveyRating(ctx context.Context, id, by uint) (int, bool, error) {
	var score []int
	err := r.db.WithContext(ctx).Raw(`UPDATE wa_call_surveys SET
		removed = jsonb_build_object('score', score, 'comment', comment, 'answers', answers, 'texts', texts,
			'answeredAt', answered_at, 'removedBy', ?::bigint, 'removedAt', now()),
		status = 'removed', score = NULL
		WHERE id = ? AND status = 'answered' AND score IS NOT NULL
		RETURNING (removed->>'score')::int`, by, id).Scan(&score).Error
	if err != nil || len(score) == 0 {
		return 0, false, err
	}
	return score[0], true, nil
}
