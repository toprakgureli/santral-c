package whatsapp

import (
	"context"
	"log/slog"
	"time"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/pkg/safe"
)

const (
	// inboundJobGrace is how long a message's follow-up work may take before
	// the sweep takes it to have been cut off.
	inboundJobGrace = 2 * time.Minute
	// inboundJobTries is how often the sweep retries one message's work.
	inboundJobTries = 5
	// mediaKeepGrace and mediaKeepWindow bound the customer files the media
	// sweep fetches again: not the ones still being fetched, not old ones.
	mediaKeepGrace  = 10 * time.Minute
	mediaKeepWindow = 72 * time.Hour
)

// inboundJob is a stored customer message whose follow-up work is not done.
type inboundJob struct {
	MessageID uint
	Created   bool
	Reopened  bool
	First     bool
	OptedOut  bool
	Attempts  int
}

// finishInboundJob removes a message's job once its follow-up work is done.
func (s *Service) finishInboundJob(ctx context.Context, messageID uint) {
	warnDB(ctx, s.db.WithContext(ctx).Exec("DELETE FROM wa_inbound_jobs WHERE message_id = ?", messageID).Error)
}

// sweepInboundJobs runs the follow-up work of messages whose work was cut
// off, for example by a restart. A job is taken by moving its clock, with
// jobs another worker holds skipped, and gives up after inboundJobTries.
func (s *Service) sweepInboundJobs(ctx context.Context) {
	var jobs []inboundJob
	err := s.db.WithContext(ctx).Raw(`WITH due AS (
			SELECT message_id FROM wa_inbound_jobs WHERE claimed_at < ?
			ORDER BY message_id LIMIT 20 FOR UPDATE SKIP LOCKED)
		UPDATE wa_inbound_jobs j SET claimed_at = now(), attempts = j.attempts + 1
		FROM due WHERE j.message_id = due.message_id
		RETURNING j.message_id, j.created, j.reopened, j.first, j.opted_out, j.attempts`,
		time.Now().Add(-inboundJobGrace)).Scan(&jobs).Error
	if err != nil {
		slog.ErrorContext(ctx, "whatsapp follow-up work could not be taken", "error", err)
		return
	}
	for i := range jobs {
		job := jobs[i]
		if job.Attempts > inboundJobTries {
			slog.ErrorContext(ctx, "whatsapp follow-up work given up", "message", job.MessageID, "attempts", job.Attempts-1)
			s.finishInboundJob(ctx, job.MessageID)
			continue
		}
		safe.Run(ctx, "whatsapp follow-up work", func() { s.resumeInbound(ctx, job) })
	}
}

// resumeInbound rebuilds what a stored message's follow-up work needs and
// runs it.
func (s *Service) resumeInbound(ctx context.Context, job inboundJob) {
	var msg models.WAMessage
	if err := s.db.WithContext(ctx).First(&msg, job.MessageID).Error; err != nil {
		slog.WarnContext(ctx, "whatsapp follow-up message could not be loaded", "message", job.MessageID, "error", err)
		return
	}
	conv, _, err := s.loadConv(ctx, msg.ConversationID)
	if err != nil {
		slog.WarnContext(ctx, "whatsapp follow-up conversation could not be loaded", "message", job.MessageID, "error", err)
		return
	}
	if msg.TicketID == nil {
		s.finishInboundJob(ctx, msg.ID)
		return
	}
	ticket := s.ticketFresh(ctx, *msg.TicketID)
	contact, err := s.contact(ctx, conv.ContactID)
	if ticket == nil || err != nil {
		slog.WarnContext(ctx, "whatsapp follow-up ticket or customer could not be loaded", "message", job.MessageID)
		return
	}
	ch, err := s.channel(ctx, conv.ChannelID)
	if err != nil {
		slog.WarnContext(ctx, "whatsapp follow-up device could not be loaded", "message", job.MessageID, "error", err)
		return
	}
	res := &inboundResult{msg: &msg, conv: conv, ticket: ticket, contact: contact,
		created: job.Created, reopened: job.Reopened, first: job.First, optedOut: job.OptedOut}
	slog.InfoContext(ctx, "whatsapp follow-up work resumed", "message", msg.ID, "attempt", job.Attempts)
	s.afterInbound(ctx, ch, res)
	s.finishInboundJob(ctx, msg.ID)
}

// sweepMedia fetches customer files that were never kept, for example
// because the server stopped during the download.
func (s *Service) sweepMedia(ctx context.Context) {
	var rows []struct {
		ID        uint
		ChannelID uint
	}
	err := s.db.WithContext(ctx).Raw(`SELECT id, channel_id FROM wa_messages
		WHERE direction = 'in' AND media IS NOT NULL
		  AND COALESCE(media->>'metaId', '') <> '' AND COALESCE(media->>'storeId', '') = ''
		  AND COALESCE(media->>'failed', '') = '' AND COALESCE((media->>'size')::bigint, 0) = 0
		  AND created_at BETWEEN ? AND ?
		ORDER BY id LIMIT 10`, time.Now().Add(-mediaKeepWindow), time.Now().Add(-mediaKeepGrace)).Scan(&rows).Error
	if err != nil {
		slog.ErrorContext(ctx, "whatsapp files to keep could not be listed", "error", err)
		return
	}
	for _, r := range rows {
		ch, err := s.channel(ctx, r.ChannelID)
		if err != nil {
			continue
		}
		slog.InfoContext(ctx, "whatsapp file fetched again", "message", r.ID)
		safe.Run(ctx, "whatsapp keep media", func() { s.keepMedia(ctx, ch, r.ID) })
	}
}
