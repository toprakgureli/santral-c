package whatsapp

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
)

// A number can keep the webhook that is already registered in Meta, for
// example the one an earlier system set up, when nobody can change it in
// Meta's developer screens. The panel stores that address; requests that
// arrive on its path are handled here as if they came to the panel's own
// address. The web server in front must send that path to this backend.

// existingPath checks an address typed in the panel and returns its path.
func existingPath(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return "", errs.Invalid("Kayıtlı webhook adresini tam yazın. Örnek: https://alanadi.com/webhook/whatsapp", nil)
	}
	p := "/" + strings.Trim(u.EscapedPath(), "/")
	if p == "/" || strings.HasPrefix(p, "/api/") || p == "/healthz" {
		return "", errs.Invalid("Bu adres kullanılamaz; Meta'da kayıtlı olan webhook adresini yazın.", nil)
	}
	return p, nil
}

// hookPaths caches the paths in use, so the check on every request of the
// server costs nothing.
type hookPaths struct {
	mu    sync.Mutex
	paths map[string]bool
	at    time.Time
}

var existingHooks hookPaths

func (s *Service) forgetHookPaths() {
	existingHooks.mu.Lock()
	existingHooks.at = time.Time{}
	existingHooks.mu.Unlock()
}

// IsExistingHook tells whether a request path belongs to a registered
// webhook of some number.
func (s *Service) IsExistingHook(ctx context.Context, path string) bool {
	path = "/" + strings.Trim(path, "/")
	existingHooks.mu.Lock()
	defer existingHooks.mu.Unlock()
	if time.Since(existingHooks.at) > 30*time.Second {
		var list []string
		_ = s.db.WithContext(ctx).Raw("SELECT DISTINCT existing_hook_path FROM wa_channels WHERE existing_hook_path <> ''").Scan(&list).Error
		existingHooks.paths = map[string]bool{}
		for _, p := range list {
			existingHooks.paths[p] = true
		}
		existingHooks.at = time.Now()
	}
	return existingHooks.paths[path]
}

func (s *Service) channelsOnPath(ctx context.Context, path string) ([]models.WAChannel, error) {
	var list []models.WAChannel
	if err := s.db.WithContext(ctx).Where("existing_hook_path = ?", "/"+strings.Trim(path, "/")).Order("id").Find(&list).Error; err != nil {
		return nil, errs.Internal(err)
	}
	if len(list) == 0 {
		return nil, errs.NotFound("Bilinmeyen adres.")
	}
	return list, nil
}

// VerifyExisting answers Meta's check on a registered address.
func (s *Service) VerifyExisting(ctx context.Context, path, mode, token, challenge string) (string, error) {
	list, err := s.channelsOnPath(ctx, path)
	if err != nil {
		return "", err
	}
	if mode == "subscribe" {
		for _, ch := range list {
			if ch.ExistingVerifyToken != "" && hmac.Equal([]byte(token), []byte(ch.ExistingVerifyToken)) {
				return challenge, nil
			}
		}
	}
	return "", errs.Forbidden("Doğrulama anahtarı eşleşmedi.")
}

// ReceiveExisting stores a notice that came to a registered address. Numbers
// of one Meta app share the address; the notice goes to the number it names.
func (s *Service) ReceiveExisting(ctx context.Context, path, signature string, body []byte) error {
	list, err := s.channelsOnPath(ctx, path)
	if err != nil {
		return err
	}
	var secrets []string
	unsigned := false
	for _, ch := range list {
		if sec := s.open(ch.AppSecretEnc); sec != "" {
			secrets = append(secrets, sec)
		}
		if ch.AcceptUnsigned {
			unsigned = true
		}
	}
	switch {
	case len(secrets) > 0:
		ok := false
		for _, sec := range secrets {
			mac := hmac.New(sha256.New, []byte(sec))
			mac.Write(body)
			if hmac.Equal([]byte(strings.TrimSpace(signature)), []byte("sha256="+hex.EncodeToString(mac.Sum(nil)))) {
				ok = true
				break
			}
		}
		if !ok {
			return errs.Unauthorized("İmza doğrulanamadı.")
		}
	case !unsigned:
		return errs.Forbidden("Cihazın uygulama gizli anahtarı girilmemiş.")
	}
	if !json.Valid(body) {
		return errs.Invalid("Geçersiz içerik.", nil)
	}
	target := &list[0]
	var probe struct {
		Entry []struct {
			Changes []struct {
				Value struct {
					Metadata struct {
						PhoneNumberID string `json:"phone_number_id"`
					} `json:"metadata"`
				} `json:"value"`
			} `json:"changes"`
		} `json:"entry"`
	}
	if json.Unmarshal(body, &probe) == nil {
		for _, e := range probe.Entry {
			for _, c := range e.Changes {
				for i := range list {
					if pid := c.Value.Metadata.PhoneNumberID; pid != "" && list[i].PhoneNumberID == pid {
						target = &list[i]
					}
				}
			}
		}
	}
	return s.storeEvent(ctx, target, body)
}
