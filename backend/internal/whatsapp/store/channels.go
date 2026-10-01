package store

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
)

// Channels reads every device in the order they were added.
func (r *Repository) Channels(ctx context.Context) ([]models.WAChannel, error) {
	var list []models.WAChannel
	if err := r.db.WithContext(ctx).Order("id").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// ActiveChannels reads the switched on devices.
func (r *Repository) ActiveChannels(ctx context.Context) ([]models.WAChannel, error) {
	var chans []models.WAChannel
	err := r.db.WithContext(ctx).Where("active").Find(&chans).Error
	return chans, err
}

// ChannelsOfAccount reads the devices of a business account.
func (r *Repository) ChannelsOfAccount(ctx context.Context, wabaID string) ([]models.WAChannel, error) {
	var chans []models.WAChannel
	err := r.db.WithContext(ctx).Where("waba_id = ?", wabaID).Find(&chans).Error
	return chans, err
}

// ChannelOfAccount reads one device of a business account.
func (r *Repository) ChannelOfAccount(ctx context.Context, wabaID string) (*models.WAChannel, error) {
	var ch models.WAChannel
	if err := r.db.WithContext(ctx).Where("waba_id = ?", wabaID).First(&ch).Error; err != nil {
		return nil, err
	}
	return &ch, nil
}

// ChannelByHook reads the device a webhook address key belongs to.
func (r *Repository) ChannelByHook(ctx context.Context, key string) (*models.WAChannel, error) {
	var ch models.WAChannel
	if err := r.db.WithContext(ctx).Where("hook_key = ?", key).First(&ch).Error; err != nil {
		return nil, err
	}
	return &ch, nil
}

// ChannelByPhoneNumberID reads the device of a Meta phone number id.
func (r *Repository) ChannelByPhoneNumberID(ctx context.Context, phoneNumberID string) (*models.WAChannel, error) {
	var ch models.WAChannel
	if err := r.db.WithContext(ctx).Where("phone_number_id = ?", phoneNumberID).First(&ch).Error; err != nil {
		return nil, err
	}
	return &ch, nil
}

// ChannelName reads a device's name; it is empty when the device does not exist.
func (r *Repository) ChannelName(ctx context.Context, id uint) (string, error) {
	var name string
	err := r.db.WithContext(ctx).Raw("SELECT name FROM wa_channels WHERE id = ?", id).Scan(&name).Error
	return name, err
}

// ChannelNames reads the id and name of the devices with the given ids;
// the other columns stay empty.
func (r *Repository) ChannelNames(ctx context.Context, ids []uint) ([]models.WAChannel, error) {
	var chans []models.WAChannel
	if err := r.db.WithContext(ctx).Select("id, name").Where("id IN ?", ids).Find(&chans).Error; err != nil {
		return nil, err
	}
	return chans, nil
}

// CreateChannel stores a new device and fills in its id.
func (r *Repository) CreateChannel(ctx context.Context, ch *models.WAChannel) error {
	return r.db.WithContext(ctx).Create(ch).Error
}

// UpdateChannel changes the given columns of a device.
func (r *Repository) UpdateChannel(ctx context.Context, id uint, fields map[string]any) error {
	return r.db.WithContext(ctx).Model(&models.WAChannel{}).Where("id = ?", id).Updates(fields).Error
}

// MarkWebhookSeen records that Meta just sent a device a notice.
func (r *Repository) MarkWebhookSeen(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Exec("UPDATE wa_channels SET last_webhook_at = now() WHERE id = ?", id).Error
}

// SetChannelError records the latest problem of a device.
func (r *Repository) SetChannelError(ctx context.Context, id uint, text string) error {
	return r.db.WithContext(ctx).Exec("UPDATE wa_channels SET last_error = ?, last_error_at = now() WHERE id = ?", text, id).Error
}

// SetMessagingLimit records how many customers a device may start
// conversations with a day, as Meta names the tier.
func (r *Repository) SetMessagingLimit(ctx context.Context, id uint, limit string) error {
	return r.db.WithContext(ctx).Exec("UPDATE wa_channels SET messaging_limit = ? WHERE id = ?", limit, id).Error
}

// ---------------------------------------------------------------- notices on a registered address

// ExistingHookPaths lists the paths of webhook addresses already registered
// at Meta that devices share with another system.
func (r *Repository) ExistingHookPaths(ctx context.Context) ([]string, error) {
	var list []string
	err := r.db.WithContext(ctx).Raw("SELECT DISTINCT existing_hook_path FROM wa_channels WHERE existing_hook_path <> ''").Scan(&list).Error
	return list, err
}

// ChannelsOnHookPath reads the devices that share a registered webhook
// path, in the order they were added. Capitals do not matter.
func (r *Repository) ChannelsOnHookPath(ctx context.Context, path string) ([]models.WAChannel, error) {
	var list []models.WAChannel
	if err := r.db.WithContext(ctx).Where("existing_hook_path <> '' AND lower(existing_hook_path) = lower(?)", path).Order("id").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// NoteRejection records on a device why a notice was turned away, at most
// once a minute for the same reason.
func (r *Repository) NoteRejection(ctx context.Context, id uint, why string) error {
	return r.db.WithContext(ctx).Exec("UPDATE wa_channels SET last_error = ?, last_error_at = now() WHERE id = ? AND (last_error_at IS NULL OR last_error_at < now() - interval '1 minute' OR last_error <> ?)", why, id, why).Error
}

// ---------------------------------------------------------------- removing

// ChannelHasConversations reports whether a device ever had a conversation.
func (r *Repository) ChannelHasConversations(ctx context.Context, id uint) (bool, error) {
	return r.exists(ctx, "SELECT 1 FROM wa_conversations WHERE channel_id = ? LIMIT 1", id)
}

// DeactivateChannel turns a device off.
func (r *Repository) DeactivateChannel(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Model(&models.WAChannel{}).Where("id = ?", id).Update("active", false).Error
}

// DeleteChannel removes a device.
func (r *Repository) DeleteChannel(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Delete(&models.WAChannel{}, id).Error
}

// exists reports whether query returns a row.
func (r *Repository) exists(ctx context.Context, query string, args ...any) (bool, error) {
	var hits []int
	if err := r.db.WithContext(ctx).Raw(query, args...).Scan(&hits).Error; err != nil {
		return false, fmt.Errorf("existence check failed: %w", err)
	}
	return len(hits) > 0, nil
}

// ---------------------------------------------------------------- members

// ChannelMemberIDs lists the people placed on a device, by id.
func (r *Repository) ChannelMemberIDs(ctx context.Context, channelID uint) ([]uint, error) {
	var ids []uint
	err := r.db.WithContext(ctx).Raw("SELECT user_id FROM wa_channel_members WHERE channel_id = ? ORDER BY user_id", channelID).Scan(&ids).Error
	return ids, err
}

// AddChannelMember places a person on a device.
func (r *Repository) AddChannelMember(ctx context.Context, channelID, userID uint) error {
	return r.db.WithContext(ctx).Exec("INSERT INTO wa_channel_members (channel_id, user_id) VALUES (?, ?) ON CONFLICT DO NOTHING", channelID, userID).Error
}

// SetChannelMembers places exactly the given people on a device, in one
// transaction.
func (r *Repository) SetChannelMembers(ctx context.Context, channelID uint, userIDs []uint) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		keep := append([]uint{0}, userIDs...)
		if err := tx.Exec("DELETE FROM wa_channel_members WHERE channel_id = ? AND user_id NOT IN ?", channelID, keep).Error; err != nil {
			return err
		}
		for _, uid := range userIDs {
			if err := tx.Exec("INSERT INTO wa_channel_members (channel_id, user_id) VALUES (?, ?) ON CONFLICT DO NOTHING", channelID, uid).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
