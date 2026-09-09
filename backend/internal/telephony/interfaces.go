package telephony

import (
	"context"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/pkg/ami"
)

// IIngestRepo persists the records built from AMI activity.
type IIngestRepo interface {
	CreateCall(ctx context.Context, c *models.Call) error
	UpdateCall(ctx context.Context, id uint, fields map[string]any) error
	AddEvent(ctx context.Context, e *models.CallEvent) error
	AddQuality(ctx context.Context, q *models.CallQuality) error
	UserIDByExtension(ctx context.Context, ext string) (*uint, error)
	ContactIDByNumber(ctx context.Context, e164 string) (*uint, error)
}

// IActorResolver loads the acting user for authorization.
type IActorResolver interface {
	GetByID(ctx context.Context, id uint) (*models.User, error)
}

// IOriginator places outbound calls on the switch.
type IOriginator interface {
	Originate(ctx context.Context, fields map[string]string) (ami.Event, error)
}
