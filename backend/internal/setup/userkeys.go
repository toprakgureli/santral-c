package setup

import (
	"context"
	"fmt"
	"log/slog"

	"gorm.io/gorm"

	"github.com/toprakgureli/santral-c/backend/pkg/crypt"
)

// UserKeys are the keys that seal the per-user secrets: the TOTP secret
// (mfa) and the SIP password (sip). Previous is the key a value may still
// be sealed with after the key was replaced; empty when there is none.
type UserKeys struct {
	MFA, PreviousMFA string
	SIP, PreviousSIP string
}

// RewrapUserSecrets seals every stored TOTP secret and SIP password with
// the current key. A value the current key already opens is left alone; one
// the previous key opens is sealed again with the current key, so replacing
// a key costs nobody their authenticator or their phone line. A value
// neither key opens is logged and kept; that person sets up TOTP again, or
// the SIP password is pulled from the PBX again. Safe on every start.
func RewrapUserSecrets(ctx context.Context, db *gorm.DB, keys UserKeys) error {
	if err := rewrapUserColumn(ctx, db, "mfa_secret", keys.MFA, keys.PreviousMFA); err != nil {
		return err
	}
	return rewrapUserColumn(ctx, db, "sip_secret", keys.SIP, keys.PreviousSIP)
}

func rewrapUserColumn(ctx context.Context, db *gorm.DB, column, current, previous string) error {
	if current == "" {
		return nil
	}
	var rows []struct {
		ID    uint
		Value string
	}
	q := fmt.Sprintf("SELECT id, %s AS value FROM users WHERE %s IS NOT NULL AND %s <> ''", column, column, column)
	if err := db.WithContext(ctx).Raw(q).Scan(&rows).Error; err != nil {
		return fmt.Errorf("users.%s could not be read: %w", column, err)
	}
	moved, lost := 0, 0
	for _, r := range rows {
		if _, err := crypt.Decrypt(current, r.Value); err == nil {
			continue
		}
		plain, err := "", fmt.Errorf("no previous key")
		if previous != "" {
			plain, err = crypt.Decrypt(previous, r.Value)
		}
		if err != nil {
			lost++
			slog.ErrorContext(ctx, "stored user secret opens with neither the current nor the previous key; it has to be set again",
				"column", column, "user", r.ID)
			continue
		}
		sealed, err := crypt.Encrypt(current, plain)
		if err != nil {
			return fmt.Errorf("users.%s could not be sealed: %w", column, err)
		}
		if err := db.WithContext(ctx).Exec(fmt.Sprintf("UPDATE users SET %s = ? WHERE id = ?", column), sealed, r.ID).Error; err != nil {
			return fmt.Errorf("users.%s could not be saved: %w", column, err)
		}
		moved++
	}
	if moved > 0 {
		slog.InfoContext(ctx, "stored user secrets sealed with the current key", "column", column, "count", moved)
	}
	if lost > 0 {
		slog.WarnContext(ctx, "some stored user secrets could not be opened", "column", column, "count", lost)
	}
	return nil
}
