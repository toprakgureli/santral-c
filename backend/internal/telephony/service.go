package telephony

import (
	"context"
	"fmt"

	"github.com/toprakgureli/santral-c/backend/internal/domain/dtos/requests"
	"github.com/toprakgureli/santral-c/backend/internal/domain/dtos/responses"
	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
)

// Service serves the call log and places outbound calls.
type Service struct {
	repo       *Repository
	users      IActorResolver
	originator IOriginator
	context    string
}

// NewService builds a telephony service.
func NewService(repo *Repository, users IActorResolver, originator IOriginator, dialContext string) *Service {
	return &Service{repo: repo, users: users, originator: originator, context: dialContext}
}

// Originate places an outbound call from the actor's extension to a number.
func (s *Service) Originate(ctx context.Context, actorID uint, req requests.CallOriginate) error {
	actor, err := s.users.GetByID(ctx, actorID)
	if err != nil {
		return err
	}
	if !actor.Can(enums.CallOriginate) {
		return errs.Forbidden("Çağrı başlatma yetkiniz yok.")
	}
	if actor.SIPExtension == nil || *actor.SIPExtension == "" {
		return errs.Invalid("Hesabınızda tanımlı bir dahili numara yok.", nil)
	}
	ext := *actor.SIPExtension

	if _, err := s.originator.Originate(ctx, map[string]string{
		"Channel":  "PJSIP/" + ext,
		"Context":  s.context,
		"Exten":    req.To,
		"Priority": "1",
		"CallerID": fmt.Sprintf("%s <%s>", actor.Name, ext),
		"Async":    "true",
	}); err != nil {
		return errs.Internal(err)
	}
	return nil
}

// List returns a page of calls, scoped to the actor's visibility.
func (s *Service) List(ctx context.Context, actorID uint, filter requests.CallFilter) (*responses.CallList, error) {
	actor, scopeOwn, err := s.authorizeView(ctx, actorID)
	if err != nil {
		return nil, err
	}
	if scopeOwn {
		filter.OwnerID = &actor.ID
	}
	filter.Page, filter.PerPage = normalizePaging(filter.Page, filter.PerPage)

	calls, total, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, errs.Internal(err)
	}
	items := make([]responses.Call, 0, len(calls))
	for i := range calls {
		items = append(items, responses.NewCall(&calls[i]))
	}
	return &responses.CallList{Items: items, Total: total, Page: filter.Page, PerPage: filter.PerPage}, nil
}

// Get returns a call with its timeline and quality samples.
func (s *Service) Get(ctx context.Context, actorID, id uint) (*responses.CallDetail, error) {
	actor, scopeOwn, err := s.authorizeView(ctx, actorID)
	if err != nil {
		return nil, err
	}
	call, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, errs.Internal(err)
	}
	if call == nil || (scopeOwn && !involves(call, actor.ID)) {
		return nil, errs.NotFound("Çağrı bulunamadı.")
	}

	events, err := s.repo.Events(ctx, id)
	if err != nil {
		return nil, errs.Internal(err)
	}
	quality, err := s.repo.Quality(ctx, id)
	if err != nil {
		return nil, errs.Internal(err)
	}
	detail := responses.NewCallDetail(call, events, quality)
	return &detail, nil
}

// authorizeView reports whether the actor may see all calls or only their own.
func (s *Service) authorizeView(ctx context.Context, actorID uint) (*models.User, bool, error) {
	actor, err := s.users.GetByID(ctx, actorID)
	if err != nil {
		return nil, false, err
	}
	if actor.Can(enums.CDRViewAll) || actor.Can(enums.CallViewAll) {
		return actor, false, nil
	}
	if actor.Can(enums.CDRViewOwn) || actor.Can(enums.CallViewOwn) {
		return actor, true, nil
	}
	return nil, false, errs.Forbidden("Çağrı kayıtlarını görme yetkiniz yok.")
}

func involves(c *models.Call, userID uint) bool {
	return (c.FromUserID != nil && *c.FromUserID == userID) ||
		(c.ToUserID != nil && *c.ToUserID == userID)
}

func normalizePaging(page, perPage int) (int, int) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}
	return page, perPage
}
