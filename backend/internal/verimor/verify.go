package verimor

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/toprakgureli/santral-c/backend/pkg/phone"
)

// callSeenWindow is how far apart the panel's and the phone system's start
// times of one call may be.
const callSeenWindow = 10 * time.Minute

// CallSeen looks in the mirrored call records for a call between extension
// and peer that started around at. It reports whether one was found, how
// long the two sides talked and whether it was answered. The call log uses
// it so that only calls the phone system really carried are followed by an
// escalation entry or a survey.
func (s *Service) CallSeen(ctx context.Context, extension, peer string, at time.Time) (bool, int, bool, error) {
	return s.repo.CallSeen(ctx, extension, peer, at)
}

// CallSeen is the mirror query behind Service.CallSeen.
func (r *Repository) CallSeen(ctx context.Context, extension, peer string, at time.Time) (bool, int, bool, error) {
	key := phone.Key(peer)
	if key == "" || strings.TrimSpace(extension) == "" {
		return false, 0, false, nil
	}
	var rows []struct {
		TalkDuration string
		AnswerStamp  string
	}
	party, like := "caller_num LIKE ? OR dest_num LIKE ?", "%"+key
	if len(key) <= 5 {
		// The other side is an extension.
		party, like = "caller_ext = ? OR dest_ext = ?", key
	}
	if err := r.db.WithContext(ctx).Raw(`SELECT talk_duration, answer_stamp FROM pbx_cdrs
		WHERE (caller_ext = ? OR dest_ext = ?) AND (`+party+`) AND start_at BETWEEN ? AND ?
		ORDER BY abs(extract(epoch FROM start_at - ?)) LIMIT 1`,
		extension, extension, like, like, at.Add(-callSeenWindow), at.Add(callSeenWindow), at).Scan(&rows).Error; err != nil {
		return false, 0, false, fmt.Errorf("call record could not be looked up: %w", err)
	}
	if len(rows) == 0 {
		return false, 0, false, nil
	}
	return true, parseDuration(rows[0].TalkDuration), strings.TrimSpace(rows[0].AnswerStamp) != "", nil
}
