package setup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"gorm.io/gorm"

	"github.com/toprakgureli/santral-c/backend/pkg/crypt"
)

// LegacyKeys are the keys stored secrets were encrypted with before the
// keyring: WhatsApp used "wa:" plus the session secret, the Drive link the
// session secret itself.
type LegacyKeys struct {
	WhatsApp string
	Drive    string
}

// SecretPurposes are the keyring labels each module seals its secrets with.
type SecretPurposes struct {
	WhatsApp string
	Drive    string
}

// secretColumn is one place a secret is stored, sealed for purpose.
type secretColumn struct {
	purpose string
	legacy  string
	table   string
	column  string
	// key is the row key column; rows are updated one at a time
	key string
	// where limits the rows, for a table that holds more than secrets
	where string
	// jsonPath, when set, is where the secret sits inside a JSON column
	jsonPath []string
}

// RewrapSecrets seals every stored secret with the keyring's current key.
// It opens values made before the keyring with their legacy key and values
// made with a previous data key with that key. It is safe to run on every
// start: current values are left alone. A value that cannot be opened is
// logged and kept as it is, so nothing is lost; the integration it belongs
// to has to be entered again.
func RewrapSecrets(ctx context.Context, db *gorm.DB, ring *crypt.Keyring, purposes SecretPurposes, legacy LegacyKeys) error {
	wa, drive := purposes.WhatsApp, purposes.Drive
	columns := []secretColumn{
		{purpose: wa, legacy: legacy.WhatsApp, table: "wa_channels", column: "access_token_enc", key: "id"},
		{purpose: wa, legacy: legacy.WhatsApp, table: "wa_channels", column: "app_secret_enc", key: "id"},
		{purpose: wa, legacy: legacy.WhatsApp, table: "wa_channels", column: "settings", key: "id", jsonPath: []string{"survey", "secretEnc"}},
		{purpose: wa, legacy: legacy.WhatsApp, table: "wa_integrations", column: "headers_enc", key: "id"},
		{purpose: wa, legacy: legacy.WhatsApp, table: "wa_global_settings", column: "value", key: "key", where: "key = 'ai'", jsonPath: []string{"keyEnc"}},
		{purpose: drive, legacy: legacy.Drive, table: "system_settings", column: "value", key: "key", where: "key = 'drive_token'"},
	}
	for _, c := range columns {
		if err := rewrapColumn(ctx, db, ring, c); err != nil {
			return err
		}
	}
	return nil
}

func rewrapColumn(ctx context.Context, db *gorm.DB, ring *crypt.Keyring, c secretColumn) error {
	var rows []struct {
		Key   string
		Value string
	}
	q := fmt.Sprintf("SELECT %s::text AS key, COALESCE(%s::text, '') AS value FROM %s", c.key, c.column, c.table)
	if c.where != "" {
		q += " WHERE " + c.where
	}
	if err := db.WithContext(ctx).Raw(q).Scan(&rows).Error; err != nil {
		return fmt.Errorf("secrets in %s.%s could not be read: %w", c.table, c.column, err)
	}
	moved := 0
	for _, r := range rows {
		next, changed, err := rewrapValue(ring, c, r.Value)
		if err != nil {
			slog.ErrorContext(ctx, "stored secret could not be decrypted; it has to be entered again",
				"table", c.table, "column", c.column, "row", r.Key, "error", err)
			continue
		}
		if !changed {
			continue
		}
		u := fmt.Sprintf("UPDATE %s SET %s = ? WHERE %s::text = ?", c.table, c.column, c.key)
		if c.jsonPath != nil {
			u = fmt.Sprintf("UPDATE %s SET %s = ?::jsonb WHERE %s::text = ?", c.table, c.column, c.key)
		}
		if err := db.WithContext(ctx).Exec(u, next, r.Key).Error; err != nil {
			return fmt.Errorf("secret in %s.%s could not be saved: %w", c.table, c.column, err)
		}
		moved++
	}
	if moved > 0 {
		slog.InfoContext(ctx, "stored secrets sealed with the current data key", "table", c.table, "column", c.column, "count", moved)
	}
	return nil
}

// rewrapValue returns the value with its secret sealed by the current key,
// and whether anything changed.
func rewrapValue(ring *crypt.Keyring, c secretColumn, value string) (string, bool, error) {
	if c.jsonPath == nil {
		return reseal(ring, c, value)
	}
	if value == "" {
		return value, false, nil
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(value), &doc); err != nil {
		return "", false, fmt.Errorf("json could not be read: %w", err)
	}
	parent := doc
	for _, k := range c.jsonPath[:len(c.jsonPath)-1] {
		next, ok := parent[k].(map[string]any)
		if !ok {
			return value, false, nil
		}
		parent = next
	}
	leaf := c.jsonPath[len(c.jsonPath)-1]
	secret, _ := parent[leaf].(string)
	sealed, changed, err := reseal(ring, c, secret)
	if err != nil || !changed {
		return value, false, err
	}
	parent[leaf] = sealed
	out, err := json.Marshal(doc)
	if err != nil {
		return "", false, fmt.Errorf("json could not be written: %w", err)
	}
	return string(out), true, nil
}

func reseal(ring *crypt.Keyring, c secretColumn, value string) (string, bool, error) {
	if ring.Current(value) {
		return value, false, nil
	}
	plain, err := ring.Open(c.purpose, value)
	if errors.Is(err, crypt.ErrNotSealed) {
		plain, err = crypt.Decrypt(c.legacy, value)
	}
	if err != nil {
		return "", false, err
	}
	sealed, err := ring.Seal(c.purpose, plain)
	if err != nil {
		return "", false, err
	}
	return sealed, true, nil
}
