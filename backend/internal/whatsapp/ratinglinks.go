package whatsapp

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
)

// Rating links open the ratings to someone outside the panel, without
// signing in, for a limited time. Whoever holds whatsapp.rating_link makes
// them on the ratings page and can cancel any of them there. The link is a
// signature over the row's id, a random nonce and the end, made with a key
// of its own derived from the data key, so it can be neither guessed nor
// stretched; the row decides whether it still works, and so does its maker,
// who must still be able to see the ratings. Such a link shows the ratings
// page without what identifies a customer: names are cut to the first name
// and an initial, numbers to their last four digits, the search reads the
// comments only, and nothing leads into a conversation.

const ratingLinkPurpose = "rating-link"

const (
	ratingLinkMaxHours = 30 * 24
	ratingLinkLabelMax = 120
	// ratingLinkKeepEnded is how long a cancelled or expired link stays in
	// the list.
	ratingLinkKeepEnded = 30 * 24 * time.Hour
)

// RatingLinkInput is a new link: who or what it is for, and for how long.
type RatingLinkInput struct {
	Label string `json:"label"`
	Hours int    `json:"hours"`
}

// RatingLinkView is one link as the ratings page lists it. Token is set
// only while the link works.
type RatingLinkView struct {
	ID           uint        `json:"id"`
	Label        string      `json:"label"`
	Token        string      `json:"token,omitempty"`
	Active       bool        `json:"active"`
	CreatedBy    PersonView  `json:"createdBy"`
	CreatedAt    time.Time   `json:"createdAt"`
	ExpiresAt    time.Time   `json:"expiresAt"`
	RevokedAt    *time.Time  `json:"revokedAt,omitempty"`
	RevokedBy    *PersonView `json:"revokedBy,omitempty"`
	OpenCount    int         `json:"openCount"`
	LastOpenedAt *time.Time  `json:"lastOpenedAt,omitempty"`
}

// SharedChannel is a device as the shared page's filter names it.
type SharedChannel struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
}

// SharedRatingsView is the ratings page opened through a link.
type SharedRatingsView struct {
	*RatingsView
	Channels  []SharedChannel `json:"channels"`
	Label     string          `json:"label"`
	ExpiresAt time.Time       `json:"expiresAt"`
}

func ratingLinkSubject(l *models.WARatingLink) string {
	return fmt.Sprintf("%d:%s:%d", l.ID, l.Nonce, l.ExpiresAt.Unix())
}

func ratingLinkSig(key []byte, subject string) string {
	mac := hmac.New(sha256.New, key)
	_, _ = io.WriteString(mac, subject) // writing to a hash cannot fail
	return hex.EncodeToString(mac.Sum(nil))
}

// ratingLinkToken is what the link carries: the row's id and its signature.
func (s *Service) ratingLinkToken(l *models.WARatingLink) string {
	return fmt.Sprintf("%d.%s", l.ID, ratingLinkSig(s.ring.MACKeys(ratingLinkPurpose)[0], ratingLinkSubject(l)))
}

func ratingLinkWorks(l *models.WARatingLink, now time.Time) bool {
	return l.RevokedAt == nil && now.Before(l.ExpiresAt)
}

// requireRatingLinks checks that the actor may see the ratings and manage
// their links.
func (s *Service) requireRatingLinks(ctx context.Context, actorID uint) error {
	if _, err := s.require(ctx, actorID, enums.WARatings, "Puanlamaları görme yetkin yok."); err != nil {
		return err
	}
	_, err := s.require(ctx, actorID, enums.WARatingLink, "Puanlama linki oluşturma yetkin yok.")
	return err
}

// CreateRatingLink makes a link that opens the ratings for the given hours.
func (s *Service) CreateRatingLink(ctx context.Context, actorID uint, in RatingLinkInput, ip string) (*RatingLinkView, error) {
	if err := s.requireRatingLinks(ctx, actorID); err != nil {
		return nil, err
	}
	label := strings.TrimSpace(in.Label)
	if utf8.RuneCountInString(label) < 2 {
		return nil, errs.Invalid("Linkin kime ya da ne için olduğunu yaz.", nil)
	}
	if utf8.RuneCountInString(label) > ratingLinkLabelMax {
		return nil, errs.Invalid(fmt.Sprintf("Açıklama en fazla %d karakter olabilir.", ratingLinkLabelMax), nil)
	}
	if in.Hours < 1 || in.Hours > ratingLinkMaxHours {
		return nil, errs.Invalid("Süre 1 saat ile 30 gün arasında olmalı.", nil)
	}
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return nil, errs.Internal(err)
	}
	l := &models.WARatingLink{
		Nonce:     hex.EncodeToString(nonce),
		Label:     label,
		CreatedBy: actorID,
		// Whole seconds, so the end signed is the end stored.
		ExpiresAt: time.Now().Add(time.Duration(in.Hours) * time.Hour).Truncate(time.Second),
	}
	if err := s.repo.CreateRatingLink(ctx, l); err != nil {
		return nil, errs.Internal(err)
	}
	s.record(ctx, actorID, enums.AuditWARatingLinkCreated, "wa_rating_link", l.ID, ip,
		map[string]any{"label": label, "hours": in.Hours, "expiresAt": l.ExpiresAt})
	views := s.ratingLinkViews(ctx, []models.WARatingLink{*l})
	return &views[0], nil
}

// RatingLinks lists the links that work and those that stopped in the last
// thirty days.
func (s *Service) RatingLinks(ctx context.Context, actorID uint) ([]RatingLinkView, error) {
	if err := s.requireRatingLinks(ctx, actorID); err != nil {
		return nil, err
	}
	rows, err := s.repo.RatingLinks(ctx, time.Now().Add(-ratingLinkKeepEnded))
	if err != nil {
		return nil, errs.Internal(err)
	}
	return s.ratingLinkViews(ctx, rows), nil
}

// RevokeRatingLink cancels a link at once; whoever opens it afterwards is
// told it no longer works.
func (s *Service) RevokeRatingLink(ctx context.Context, actorID, id uint, ip string) error {
	if err := s.requireRatingLinks(ctx, actorID); err != nil {
		return err
	}
	ok, err := s.repo.RevokeRatingLink(ctx, id, actorID)
	if err != nil {
		return errs.Internal(err)
	}
	if !ok {
		return errs.NotFound("Link bulunamadı ya da zaten geçersiz.")
	}
	s.record(ctx, actorID, enums.AuditWARatingLinkRevoked, "wa_rating_link", id, ip, nil)
	return nil
}

func (s *Service) ratingLinkViews(ctx context.Context, rows []models.WARatingLink) []RatingLinkView {
	var ids []uint
	for _, l := range rows {
		ids = append(ids, l.CreatedBy)
		if l.RevokedBy != nil {
			ids = append(ids, *l.RevokedBy)
		}
	}
	people := s.people(ctx, ids)
	now := time.Now()
	out := make([]RatingLinkView, 0, len(rows))
	for i := range rows {
		l := &rows[i]
		v := RatingLinkView{ID: l.ID, Label: l.Label, Active: ratingLinkWorks(l, now), CreatedBy: people[l.CreatedBy],
			CreatedAt: l.CreatedAt, ExpiresAt: l.ExpiresAt, RevokedAt: l.RevokedAt, OpenCount: l.OpenCount, LastOpenedAt: l.LastOpenedAt}
		if v.Active {
			v.Token = s.ratingLinkToken(l)
		}
		if l.RevokedBy != nil {
			p := people[*l.RevokedBy]
			v.RevokedBy = &p
		}
		out = append(out, v)
	}
	return out
}

// errRatingLinkGone is the one answer to a link that does not open, so a
// stranger learns nothing about why.
var errRatingLinkGone = errs.NotFound("Bu link geçersiz ya da süresi dolmuş. Linki gönderen kişiden yenisini isteyebilirsin.")

// ratingLinkFor finds the working link a token names.
func (s *Service) ratingLinkFor(ctx context.Context, token string) (*models.WARatingLink, error) {
	idPart, sig, ok := strings.Cut(strings.TrimSpace(token), ".")
	if !ok || sig == "" {
		return nil, errRatingLinkGone
	}
	id, err := strconv.ParseUint(idPart, 10, 64)
	if err != nil || id == 0 {
		return nil, errRatingLinkGone
	}
	l, found, err := s.repo.RatingLink(ctx, uint(id))
	if err != nil {
		return nil, errs.Internal(err)
	}
	if !found {
		return nil, errRatingLinkGone
	}
	signed := false
	for _, k := range s.ring.MACKeys(ratingLinkPurpose) {
		if hmac.Equal([]byte(sig), []byte(ratingLinkSig(k, ratingLinkSubject(l)))) {
			signed = true
		}
	}
	if !signed || !ratingLinkWorks(l, time.Now()) {
		return nil, errRatingLinkGone
	}
	// The link shows what its maker may see, and only while they may.
	maker, err := s.users.GetByID(ctx, l.CreatedBy)
	if err != nil || maker == nil || !maker.Active || !maker.Can(enums.WAView) || !maker.Can(enums.WARatings) {
		return nil, errRatingLinkGone
	}
	return l, nil
}

// SharedRatings is the ratings page opened through a link. first marks the
// first load of a visit, which is counted.
func (s *Service) SharedRatings(ctx context.Context, token string, f RatingFilter, first bool) (*SharedRatingsView, error) {
	l, err := s.ratingLinkFor(ctx, token)
	if err != nil {
		return nil, err
	}
	args, err := f.args()
	if err != nil {
		return nil, err
	}
	args["commentsOnly"] = true
	view, err := s.ratingsView(ctx, args, f.Page)
	if err != nil {
		return nil, err
	}
	for i := range view.Items {
		it := &view.Items[i]
		it.Customer = maskName(it.Customer)
		it.Phone = maskPhone(it.Phone)
		it.ConversationID, it.TicketNumber = nil, nil
		if it.Agent != nil {
			a := *it.Agent
			a.HasAvatar, a.AvatarVersion = false, 0
			it.Agent = &a
		}
	}
	for i := range view.Agents {
		view.Agents[i].Agent.HasAvatar, view.Agents[i].Agent.AvatarVersion = false, 0
	}
	out := &SharedRatingsView{RatingsView: view, Channels: []SharedChannel{}, Label: l.Label, ExpiresAt: l.ExpiresAt}
	channels, err := s.repo.Channels(ctx)
	warnDB(ctx, err)
	for _, ch := range channels {
		out.Channels = append(out.Channels, SharedChannel{ID: ch.ID, Name: ch.Name})
	}
	if first {
		warnDB(ctx, s.repo.RatingLinkOpened(ctx, l.ID))
	}
	return out, nil
}

// maskName keeps the first name and the initial of the last one:
// "Zeynep Arslan" becomes "Zeynep A.".
func maskName(name string) string {
	parts := strings.Fields(name)
	switch len(parts) {
	case 0:
		return ""
	case 1:
		return parts[0]
	}
	last := []rune(parts[len(parts)-1])
	return parts[0] + " " + string(last[0]) + "."
}

// maskPhone keeps the last four digits of a number.
func maskPhone(waID string) string {
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, waID)
	if len(digits) <= 4 {
		return "••••"
	}
	return "•••• " + digits[len(digits)-4:]
}
