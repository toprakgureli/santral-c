package store

import (
	"context"
	"time"
)

// UserPrefs is a person's general notification preferences.
type UserPrefs struct {
	Sound      bool
	Desktop    bool
	MutedUntil *time.Time
}

// UserPrefs reads a person's preferences; found is false when they never
// saved any or they could not be read.
func (r *Repository) UserPrefs(ctx context.Context, userID uint) (prefs UserPrefs, found bool) {
	found = r.db.WithContext(ctx).Raw("SELECT sound, desktop, muted_until FROM wa_user_prefs WHERE user_id = ?", userID).Scan(&prefs).RowsAffected > 0
	return prefs, found
}

// SaveUserPrefs stores a person's preferences.
func (r *Repository) SaveUserPrefs(ctx context.Context, userID uint, sound, desktop bool, mutedUntil *time.Time) error {
	return r.db.WithContext(ctx).Exec(`INSERT INTO wa_user_prefs (user_id, sound, desktop, muted_until, updated_at) VALUES (?, ?, ?, ?, now())
		ON CONFLICT (user_id) DO UPDATE SET sound = EXCLUDED.sound, desktop = EXCLUDED.desktop, muted_until = EXCLUDED.muted_until, updated_at = now()`,
		userID, sound, desktop, mutedUntil).Error
}

// ConversationPref is how a person muted or pinned one conversation.
type ConversationPref struct {
	ConversationID uint
	MutedUntil     *time.Time
	PinnedAt       *time.Time
}

// ConversationPrefs lists the conversations a person pinned or still has muted.
func (r *Repository) ConversationPrefs(ctx context.Context, userID uint) ([]ConversationPref, error) {
	var convs []ConversationPref
	if err := r.db.WithContext(ctx).Raw(`SELECT conversation_id, muted_until, pinned_at FROM wa_user_conversations
		WHERE user_id = ? AND (pinned_at IS NOT NULL OR muted_until > now())`, userID).Scan(&convs).Error; err != nil {
		return nil, err
	}
	return convs, nil
}

// AddConversationPref makes room for a person's preferences on a conversation.
func (r *Repository) AddConversationPref(ctx context.Context, userID, conversationID uint) error {
	return r.db.WithContext(ctx).Exec("INSERT INTO wa_user_conversations (user_id, conversation_id) VALUES (?, ?) ON CONFLICT DO NOTHING", userID, conversationID).Error
}

// MuteConversation mutes a conversation for a person until a moment, or
// unmutes it with nil.
func (r *Repository) MuteConversation(ctx context.Context, userID, conversationID uint, until *time.Time) error {
	return r.db.WithContext(ctx).Exec("UPDATE wa_user_conversations SET muted_until = ? WHERE user_id = ? AND conversation_id = ?", until, userID, conversationID).Error
}

// PinConversation pins a conversation for a person at a moment, or unpins
// it with nil.
func (r *Repository) PinConversation(ctx context.Context, userID, conversationID uint, pinnedAt *time.Time) error {
	return r.db.WithContext(ctx).Exec("UPDATE wa_user_conversations SET pinned_at = ? WHERE user_id = ? AND conversation_id = ?", pinnedAt, userID, conversationID).Error
}

// DropIdleConversationPrefs removes a person's conversation preferences
// that are neither pinned nor muted any more.
func (r *Repository) DropIdleConversationPrefs(ctx context.Context, userID uint) error {
	return r.db.WithContext(ctx).Exec("DELETE FROM wa_user_conversations WHERE user_id = ? AND pinned_at IS NULL AND (muted_until IS NULL OR muted_until < now())", userID).Error
}
