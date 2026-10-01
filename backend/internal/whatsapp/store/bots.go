package store

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
)

// LoadBot reads a chatbot and returns the database error as it is, so the
// caller decides what a missing chatbot means.
func (r *Repository) LoadBot(ctx context.Context, id uint) (*models.WABot, error) {
	var b models.WABot
	if err := r.db.WithContext(ctx).First(&b, id).Error; err != nil {
		return nil, err
	}
	return &b, nil
}

// Bots lists every chatbot in the order they were made.
func (r *Repository) Bots(ctx context.Context) ([]models.WABot, error) {
	var list []models.WABot
	if err := r.db.WithContext(ctx).Order("id").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// ActiveBotsOnChannel lists the switched on, published chatbots placed on a
// device, in the order they were made.
func (r *Repository) ActiveBotsOnChannel(ctx context.Context, channelID uint) ([]models.WABot, error) {
	var bots []models.WABot
	if err := r.db.WithContext(ctx).Where("active AND published_version > 0 AND channel_ids @> ?::jsonb", fmt.Sprintf("[%d]", channelID)).Order("id").Find(&bots).Error; err != nil {
		return nil, err
	}
	return bots, nil
}

// ConflictingBots lists the other switched on chatbots with the same
// trigger that always run, the ones a new always-on chatbot would clash with.
func (r *Repository) ConflictingBots(ctx context.Context, trigger string, exceptID uint) ([]models.WABot, error) {
	var others []models.WABot
	if err := r.db.WithContext(ctx).Where("active AND trigger = ? AND id <> ? AND (trigger <> 'entry' OR COALESCE(schedule->>'mode', 'always') = 'always')", trigger, exceptID).Find(&others).Error; err != nil {
		return nil, err
	}
	return others, nil
}

// CreateBot stores a new chatbot and fills in its id.
func (r *Repository) CreateBot(ctx context.Context, b *models.WABot) error {
	return r.db.WithContext(ctx).Create(b).Error
}

// UpdateBot changes the given columns of a chatbot.
func (r *Repository) UpdateBot(ctx context.Context, id uint, fields map[string]any) error {
	return r.db.WithContext(ctx).Model(&models.WABot{}).Where("id = ?", id).Updates(fields).Error
}

// SetBotDraft replaces a chatbot's draft drawing.
func (r *Repository) SetBotDraft(ctx context.Context, id uint, graph string) error {
	return r.db.WithContext(ctx).Exec("UPDATE wa_bots SET draft = ?, updated_at = now() WHERE id = ?", graph, id).Error
}

// BotDraft reads a chatbot's draft drawing as text.
func (r *Repository) BotDraft(ctx context.Context, id uint) (string, error) {
	var draft string
	err := r.db.WithContext(ctx).Raw("SELECT draft::text FROM wa_bots WHERE id = ?", id).Scan(&draft).Error
	return draft, err
}

// PublishBot stores the draft as the next published version and makes it
// the one customers meet, both in one transaction.
func (r *Repository) PublishBot(ctx context.Context, id uint, version int, graph string, actorID uint) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("INSERT INTO wa_bot_versions (bot_id, version, graph, published_by) VALUES (?, ?, ?, ?)", id, version, graph, actorID).Error; err != nil {
			return err
		}
		return tx.Exec("UPDATE wa_bots SET published_version = ?, published_at = now(), updated_at = now() WHERE id = ?", version, id).Error
	})
}

// BotHasHistory reports whether a chatbot was ever published or run.
func (r *Repository) BotHasHistory(ctx context.Context, id uint) (bool, error) {
	return r.exists(ctx, `SELECT 1 FROM wa_bot_versions WHERE bot_id = ?
		UNION ALL SELECT 1 FROM wa_bot_events WHERE bot_id = ? LIMIT 1`, id, id)
}

// DeactivateBot turns a chatbot off.
func (r *Repository) DeactivateBot(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Model(&models.WABot{}).Where("id = ?", id).Update("active", false).Error
}

// DeleteBot removes a chatbot.
func (r *Repository) DeleteBot(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Delete(&models.WABot{}, id).Error
}

// BotSessionConversations lists the conversations inside a chatbot's flow.
func (r *Repository) BotSessionConversations(ctx context.Context, botID uint) ([]uint, error) {
	var convs []uint
	if err := r.db.WithContext(ctx).Raw("SELECT conversation_id FROM wa_bot_sessions WHERE bot_id = ?", botID).Scan(&convs).Error; err != nil {
		return nil, err
	}
	return convs, nil
}

// BotGraph reads the drawing of one published version of a chatbot, the
// one a running flow follows. It is empty when the version is missing.
func (r *Repository) BotGraph(ctx context.Context, botID uint, version int) (string, error) {
	var raw string
	err := r.db.WithContext(ctx).Raw("SELECT graph FROM wa_bot_versions WHERE bot_id = ? AND version = ?", botID, version).Scan(&raw).Error
	return raw, err
}

// BotVersionText reads the drawing of one published version of a chatbot
// as text. It is empty when the version is missing.
func (r *Repository) BotVersionText(ctx context.Context, botID uint, version int) (string, error) {
	var raw string
	err := r.db.WithContext(ctx).Raw("SELECT graph::text FROM wa_bot_versions WHERE bot_id = ? AND version = ?", botID, version).Scan(&raw).Error
	return raw, err
}

// BotVersion is one published version of a chatbot and who published it.
type BotVersion struct {
	Version     int
	PublishedBy string
	CreatedAt   time.Time
}

// BotVersions lists a chatbot's published versions, newest first.
func (r *Repository) BotVersions(ctx context.Context, botID uint) ([]BotVersion, error) {
	var out []BotVersion
	if err := r.db.WithContext(ctx).Raw(`SELECT v.version, COALESCE(u.name, '') AS published_by, v.created_at FROM wa_bot_versions v
		LEFT JOIN users u ON u.id = v.published_by WHERE v.bot_id = ? ORDER BY v.version DESC`, botID).Scan(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

// BotSession reads where a conversation stands inside its flow.
func (r *Repository) BotSession(ctx context.Context, conversationID uint) (*models.WABotSession, error) {
	var sess models.WABotSession
	if err := r.db.WithContext(ctx).Where("conversation_id = ?", conversationID).First(&sess).Error; err != nil {
		return nil, err
	}
	return &sess, nil
}

// StartBotSession puts a conversation at the start of a chatbot's flow,
// replacing any flow it was in.
func (r *Repository) StartBotSession(ctx context.Context, conversationID, botID uint, version int) error {
	return r.db.WithContext(ctx).Exec(`INSERT INTO wa_bot_sessions (conversation_id, bot_id, version, node_id, vars, tries) VALUES (?, ?, ?, '', '{}', 0)
		ON CONFLICT (conversation_id) DO UPDATE SET bot_id = EXCLUDED.bot_id, version = EXCLUDED.version, node_id = '', vars = '{}', tries = 0, started_at = now(), updated_at = now()`,
		conversationID, botID, version).Error
}

// SaveBotSession records the step a conversation reached in its flow, the
// answers gathered so far and the failed tries on the current step.
func (r *Repository) SaveBotSession(ctx context.Context, conversationID uint, nodeID, vars string, tries int) error {
	return r.db.WithContext(ctx).Exec("UPDATE wa_bot_sessions SET node_id = ?, vars = ?, tries = ?, updated_at = now() WHERE conversation_id = ?",
		nodeID, vars, tries, conversationID).Error
}

// DeleteBotSession takes a conversation out of its flow.
func (r *Repository) DeleteBotSession(ctx context.Context, conversationID uint) error {
	return r.db.WithContext(ctx).Exec("DELETE FROM wa_bot_sessions WHERE conversation_id = ?", conversationID).Error
}

// IdleBotSession is a conversation inside a flow and when it last moved.
type IdleBotSession struct {
	ConversationID uint
	ChannelID      uint
	UpdatedAt      time.Time
}

// IdleBotSessions lists the flows that have not moved for five minutes.
func (r *Repository) IdleBotSessions(ctx context.Context) ([]IdleBotSession, error) {
	var rows []IdleBotSession
	err := r.db.WithContext(ctx).Raw(`SELECT s.conversation_id, c.channel_id, s.updated_at FROM wa_bot_sessions s JOIN wa_conversations c ON c.id = s.conversation_id
		WHERE s.updated_at < now() - interval '5 minutes'`).Scan(&rows).Error
	return rows, err
}

// AddBotEvent records a step of a flow (entering a box, failing, handing
// over, ending, timing out) for the chatbot's report.
func (r *Repository) AddBotEvent(ctx context.Context, botID uint, version int, conversationID uint, nodeID, kind string) error {
	return r.db.WithContext(ctx).Exec("INSERT INTO wa_bot_events (bot_id, version, conversation_id, node_id, kind) VALUES (?, ?, ?, ?, ?)",
		botID, version, conversationID, nodeID, kind).Error
}

// BotEventCount is how many times one kind of step happened on one box.
type BotEventCount struct {
	NodeID string
	Kind   string
	N      int64
}

// BotEventCounts counts a chatbot's flow steps since a moment, by box and kind.
func (r *Repository) BotEventCounts(ctx context.Context, botID uint, since time.Time) ([]BotEventCount, error) {
	var rows []BotEventCount
	if err := r.db.WithContext(ctx).Raw("SELECT node_id, kind, count(*) AS n FROM wa_bot_events WHERE bot_id = ? AND created_at >= ? GROUP BY node_id, kind", botID, since).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}
