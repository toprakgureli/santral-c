package user

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"strings"

	// Registers the webp decoder so image.DecodeConfig can read the avatar.
	_ "golang.org/x/image/webp"

	"github.com/toprakgureli/santral-c/backend/internal/domain/dtos/responses"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
)

// The panel encodes the cropped photo as a 256x256 webp. The server checks
// the bytes anyway: a client could send anything, and an oversized image
// would sit in every backup and every page that shows the face.
const (
	avatarPrefix   = "data:image/webp;base64,"
	avatarMaxSide  = 512
	avatarMaxBytes = 90_000
)

// checkAvatar validates a webp data URI: real webp, small, at most 512px.
func checkAvatar(value string) error {
	if !strings.HasPrefix(value, avatarPrefix) {
		return errs.Invalid("Profil fotoğrafı webp biçiminde olmalı.", nil)
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(value, avatarPrefix))
	if err != nil {
		return errs.Invalid("Profil fotoğrafı okunamadı.", nil)
	}
	if len(raw) > avatarMaxBytes {
		return errs.Invalid("Profil fotoğrafı çok büyük.", nil)
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil || format != "webp" {
		return errs.Invalid("Profil fotoğrafı geçerli bir webp görsel değil.", nil)
	}
	if cfg.Width > avatarMaxSide || cfg.Height > avatarMaxSide {
		return errs.Invalid("Profil fotoğrafı en fazla 512x512 olabilir.", nil)
	}
	return nil
}

// SetAvatar stores the actor's own photo; an empty value removes it.
func (s *Service) SetAvatar(ctx context.Context, actorID uint, avatar string) (*responses.User, error) {
	actor, err := s.repo.GetByID(ctx, actorID)
	if err != nil {
		return nil, errs.Internal(err)
	}
	if actor == nil {
		return nil, errs.Unauthorized("Oturum bulunamadı. Lütfen giriş yapın.")
	}
	avatar = strings.TrimSpace(avatar)
	if avatar != "" {
		if err := checkAvatar(avatar); err != nil {
			return nil, err
		}
	}
	if err := s.repo.UpdateCore(ctx, actorID, map[string]any{"avatar": avatar}); err != nil {
		return nil, errs.Internal(err)
	}
	updated, err := s.repo.GetByID(ctx, actorID)
	if err != nil {
		return nil, errs.Internal(err)
	}
	dto := responses.NewUser(updated)
	return &dto, nil
}

// Avatar returns a user's photo bytes, or nil when they have none.
func (s *Service) Avatar(ctx context.Context, id uint) ([]byte, error) {
	value, err := s.repo.Avatar(ctx, id)
	if err != nil {
		return nil, errs.Internal(err)
	}
	if !strings.HasPrefix(value, avatarPrefix) {
		return nil, nil
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(value, avatarPrefix))
	if err != nil {
		return nil, nil
	}
	return raw, nil
}
