package whatsapp

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/pkg/safe"
)

const (
	// inboundJobGrace is how long a message's follow-up work may go without
	// a sign of life before another worker takes it to have been cut off.
	// A running job renews its claim every inboundJobBeat.
	inboundJobGrace = 2 * time.Minute
	inboundJobBeat  = 30 * time.Second
	// inboundJobTries is how often one message's work is tried.
	inboundJobTries = 5
	// inboundWorkers run follow-up work side by side; one conversation's
	// messages still go one after another.
	inboundWorkers = 4
	// mediaKeepGrace and mediaKeepWindow bound the customer files the media
	// sweep fetches again: not the ones still being fetched, not old ones.
	mediaKeepGrace  = 10 * time.Minute
	mediaKeepWindow = 72 * time.Hour
)

// inboundJob is a stored customer message whose follow-up work is not done:
// starting or continuing a chatbot, handing the chat out, automatic rules.
type inboundJob struct {
	MessageID uint
	Created   bool
	Reopened  bool
	First     bool
	OptedOut  bool
	Attempts  int
	// ResolvedAt is when the ticket had been resolved before this message
	// reopened it; OwnerID who had it when the message came.
	ResolvedAt *time.Time
	OwnerID    *uint
	// Steps lists the steps already done, comma separated.
	Steps string
}

// finishInboundJob removes a message's job once its follow-up work is done.
func (s *Service) finishInboundJob(ctx context.Context, messageID uint) {
	warnDB(ctx, s.db.WithContext(ctx).Exec("DELETE FROM wa_inbound_jobs WHERE message_id = ?", messageID).Error)
}

// inboundWorker runs follow-up work until ctx ends. Work for a message is
// taken by moving its clock, with jobs another worker holds skipped, and
// only the oldest waiting message of a conversation can be taken, so a
// conversation's messages are handled in the order they came. A slow
// outside system in one conversation no longer holds up the others, nor
// the receiving of new messages.
func (s *Service) inboundWorker(ctx context.Context) {
	for {
		job, ok := s.takeInboundJob(ctx)
		if ok {
			s.runInboundJob(ctx, job)
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-s.wakeInbound:
		case <-time.After(5 * time.Second):
		}
	}
}

func (s *Service) takeInboundJob(ctx context.Context) (inboundJob, bool) {
	var jobs []inboundJob
	err := s.db.WithContext(ctx).Raw(`WITH due AS (
			SELECT j.message_id FROM wa_inbound_jobs j
			WHERE j.claimed_at < ?
			  AND NOT EXISTS (SELECT 1 FROM wa_inbound_jobs o WHERE o.conversation_id = j.conversation_id AND o.message_id < j.message_id)
			ORDER BY j.message_id LIMIT 1
			FOR UPDATE OF j SKIP LOCKED)
		UPDATE wa_inbound_jobs j SET claimed_at = now(), attempts = j.attempts + 1
		FROM due WHERE j.message_id = due.message_id
		RETURNING j.message_id, j.created, j.reopened, j.first, j.opted_out, j.attempts, j.resolved_at, j.owner_id, j.steps`,
		time.Now().Add(-inboundJobGrace)).Scan(&jobs).Error
	if err != nil {
		if ctx.Err() == nil {
			slog.ErrorContext(ctx, "whatsapp follow-up work could not be taken", "error", err)
		}
		return inboundJob{}, false
	}
	if len(jobs) == 0 {
		return inboundJob{}, false
	}
	return jobs[0], true
}

func (s *Service) runInboundJob(ctx context.Context, job inboundJob) {
	if job.Attempts > inboundJobTries {
		slog.ErrorContext(ctx, "whatsapp follow-up work given up", "message", job.MessageID, "attempts", job.Attempts-1)
		s.finishInboundJob(ctx, job.MessageID)
		return
	}
	// A long chatbot flow keeps its claim alive, so no one else takes it.
	beat, stop := context.WithCancel(ctx)
	defer stop()
	safe.Go(beat, "whatsapp follow-up claim", func() {
		t := time.NewTicker(inboundJobBeat)
		defer t.Stop()
		for {
			select {
			case <-beat.Done():
				return
			case <-t.C:
				warnDB(beat, s.db.WithContext(beat).Exec("UPDATE wa_inbound_jobs SET claimed_at = now() WHERE message_id = ?", job.MessageID).Error)
			}
		}
	})
	// Work that started finishes even while the server stops; what was
	// done is marked step by step either way.
	work := context.WithoutCancel(ctx)
	err := safe.Call(func() error {
		s.resumeInbound(work, job)
		return nil
	})
	if err != nil {
		slog.ErrorContext(ctx, "whatsapp follow-up work panicked", "message", job.MessageID, "error", err)
	}
}

// resumeInbound rebuilds what a stored message's follow-up work needs and
// runs the steps not done yet.
func (s *Service) resumeInbound(ctx context.Context, job inboundJob) {
	var msg models.WAMessage
	if err := s.db.WithContext(ctx).First(&msg, job.MessageID).Error; err != nil {
		slog.WarnContext(ctx, "whatsapp follow-up message could not be loaded", "message", job.MessageID, "error", err)
		return
	}
	conv, _, err := s.repo.Conversation(ctx, msg.ConversationID)
	if err != nil {
		slog.WarnContext(ctx, "whatsapp follow-up conversation could not be loaded", "message", job.MessageID, "error", err)
		return
	}
	if msg.TicketID == nil {
		s.finishInboundJob(ctx, msg.ID)
		return
	}
	ticket := s.repo.Ticket(ctx, *msg.TicketID)
	contact, err := s.repo.Contact(ctx, conv.ContactID)
	if ticket == nil || err != nil {
		slog.WarnContext(ctx, "whatsapp follow-up ticket or customer could not be loaded", "message", job.MessageID)
		return
	}
	ch, err := s.repo.Channel(ctx, conv.ChannelID)
	if err != nil {
		slog.WarnContext(ctx, "whatsapp follow-up device could not be loaded", "message", job.MessageID, "error", err)
		return
	}
	res := &inboundResult{msg: &msg, conv: conv, ticket: ticket, contact: contact,
		created: job.Created, reopened: job.Reopened, first: job.First, optedOut: job.OptedOut,
		resolvedAt: job.ResolvedAt, ownerAtMessage: job.OwnerID}
	if job.Attempts > 1 {
		slog.InfoContext(ctx, "whatsapp follow-up work resumed", "message", msg.ID, "attempt", job.Attempts, "done", job.Steps)
	}
	s.afterInbound(ctx, ch, res, &jobSteps{s: s, messageID: msg.ID, done: job.Steps})
	s.finishInboundJob(ctx, msg.ID)
}

// jobSteps remembers which steps of a message's follow-up work are done, so
// work picked up again skips them instead of greeting or notifying twice.
type jobSteps struct {
	s         *Service
	messageID uint
	done      string
}

// run does a step once: skipped when it was done before, marked done after.
func (j *jobSteps) run(ctx context.Context, name string, fn func()) {
	if strings.Contains(","+j.done, ","+name+",") {
		return
	}
	fn()
	j.done += name + ","
	warnDB(ctx, j.s.db.WithContext(ctx).Exec("UPDATE wa_inbound_jobs SET steps = steps || ? WHERE message_id = ?", name+",", j.messageID).Error)
}

// mediaLoop fetches customer files that were never kept, apart from the
// clock, so a slow download never holds up the timed work.
func (s *Service) mediaLoop(ctx context.Context) {
	t := time.NewTicker(5 * time.Minute)
	defer t.Stop()
	for {
		s.sweepMedia(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
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
		if ctx.Err() == nil {
			slog.ErrorContext(ctx, "whatsapp files to keep could not be listed", "error", err)
		}
		return
	}
	for _, r := range rows {
		if ctx.Err() != nil {
			return
		}
		ch, err := s.repo.Channel(ctx, r.ChannelID)
		if err != nil {
			continue
		}
		slog.InfoContext(ctx, "whatsapp file fetched again", "message", r.ID)
		safe.Run(ctx, "whatsapp keep media", func() { s.keepMedia(ctx, ch, r.ID) })
	}
}
