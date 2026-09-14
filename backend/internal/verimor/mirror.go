package verimor

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
)

// The call-record mirror. Verimor's CDR API cannot search: its number filter
// returns unrelated rows and its exact-match parameters return nothing, so
// the panel keeps its own copy in pbx_cdrs. The regular poll copies the
// newest page every 30 seconds; a background backfill walks older days once,
// slowly, so the hosted API's rate limit (about ten requests a minute) is
// shared fairly with the panel.
const (
	headPageSize       = 100              // newest records fetched per poll
	mirrorPageSize     = 100              // records per backfill request
	mirrorRequestGap   = 15 * time.Second // pause between backfill requests
	mirrorRetryGap     = time.Minute      // pause after a failed backfill request
	mirrorRetries      = 3                // attempts per backfill page
	mirrorRepairAfter  = 30 * time.Minute // a gap longer than this is re-fetched on start
	defaultHistoryDays = 90
	mirrorCursorKey    = "cdr_mirror_cursor"
	stampLayout        = "2006-01-02 15:04:05 -0700"
)

// party is one side of a call as the hosted API prints it: an external number
// ("05304230113"), a DID with the extension behind it ("902127060510 (1014)"),
// an extension with its outbound DID ("1014 (902129510292)"), or a bare
// extension ("1014").
type party struct {
	Ext string // internal extension, if any
	Num string // digits of the phone number (external or DID), if any
}

// parseParty splits a caller or destination field into its extension and
// number.
func parseParty(field string) party {
	field = strings.TrimSpace(field)
	outer, inner := field, ""
	if i := strings.IndexByte(field, '('); i >= 0 {
		outer = strings.TrimSpace(field[:i])
		inner = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(field[i+1:]), ")"))
	}
	var p party
	for _, part := range []string{outer, inner} {
		switch {
		case part == "":
		case isExtension(part):
			if p.Ext == "" {
				p.Ext = part
			}
		default:
			if d := digitsOf(part); d != "" && p.Num == "" {
				p.Num = d
			}
		}
	}
	return p
}

// digitsOf keeps only the digits of s.
func digitsOf(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// phoneQuery normalizes a typed number for matching against the stored
// digits: the country code and the trunk zero are dropped so "0530...",
// "90530..." and "530..." all find the same calls, and a partial number still
// matches as a substring.
func phoneQuery(s string) string {
	d := digitsOf(s)
	switch {
	case len(d) == 12 && strings.HasPrefix(d, "90"):
		return d[2:]
	case len(d) == 11 && strings.HasPrefix(d, "0"):
		return d[1:]
	}
	return d
}

// extIsParty reports whether the extension is a party on the call, as caller
// or callee, in either direction.
func extIsParty(c CDR, ext string) bool {
	return parseParty(c.CallerIDNumber).Ext == ext || parseParty(c.DestinationNumber).Ext == ext
}

// cdrRow maps a hosted record to its mirror row. ok is false when the start
// stamp cannot be read, since such a row could not be ordered or filtered.
func cdrRow(c CDR, fetchedAt time.Time) (models.PBXCDR, bool) {
	start, err := time.Parse(stampLayout, strings.TrimSpace(c.StartStamp))
	if err != nil || c.CallUUID == "" {
		return models.PBXCDR{}, false
	}
	caller, dest := parseParty(c.CallerIDNumber), parseParty(c.DestinationNumber)
	return models.PBXCDR{
		CallUUID:          c.CallUUID,
		StartAt:           start,
		StartStamp:        c.StartStamp,
		Direction:         normalizeDirection(c.Direction),
		RawDirection:      c.Direction,
		CallerIDNumber:    c.CallerIDNumber,
		DestinationNumber: c.DestinationNumber,
		CallerNum:         caller.Num,
		DestNum:           dest.Num,
		CallerExt:         caller.Ext,
		DestExt:           dest.Ext,
		Duration:          c.Duration,
		TalkDuration:      c.TalkDuration,
		AnswerStamp:       c.AnswerStamp,
		Result:            c.Result,
		Missed:            bool(c.Missed),
		RecordingPresent:  bool(c.RecordingPresent),
		FetchedAt:         fetchedAt,
	}, true
}

// rowCDR maps a mirror row back to the hosted API's shape, so the existing
// mapping to the panel's Call view applies unchanged.
func rowCDR(r models.PBXCDR) CDR {
	return CDR{
		CallUUID:          r.CallUUID,
		StartStamp:        r.StartStamp,
		Direction:         r.RawDirection,
		CallerIDNumber:    r.CallerIDNumber,
		DestinationNumber: r.DestinationNumber,
		Duration:          r.Duration,
		TalkDuration:      r.TalkDuration,
		AnswerStamp:       r.AnswerStamp,
		Result:            r.Result,
		Missed:            flexBool(r.Missed),
		RecordingPresent:  flexBool(r.RecordingPresent),
	}
}

// UpsertCDRs stores records, replacing existing rows so late changes (a
// recording attached after the call) are picked up. It returns how many rows
// were written.
func (r *Repository) UpsertCDRs(ctx context.Context, cdrs []CDR) (int, error) {
	now := time.Now()
	rows := make([]models.PBXCDR, 0, len(cdrs))
	for i := range cdrs {
		if row, ok := cdrRow(cdrs[i], now); ok {
			rows = append(rows, row)
		}
	}
	if len(rows) == 0 {
		return 0, nil
	}
	err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "call_uuid"}},
		UpdateAll: true,
	}).CreateInBatches(rows, 200).Error
	if err != nil {
		return 0, fmt.Errorf("call records could not be stored: %w", err)
	}
	return len(rows), nil
}

// CDRQuery narrows a mirror query. Zero values mean "no restriction".
type CDRQuery struct {
	Ext       string    // extension that must be a party on the call
	Phone     string    // digit substring that must appear in a party's number
	Direction string    // inbound, outbound or internal
	From      time.Time // inclusive lower bound on start_at
	To        time.Time // exclusive upper bound on start_at
	Page      int
	Limit     int
}

// QueryCDRs returns one page of matching records, newest first, with the
// total match count.
func (r *Repository) QueryCDRs(ctx context.Context, q CDRQuery) ([]models.PBXCDR, int64, error) {
	tx := r.db.WithContext(ctx).Model(&models.PBXCDR{})
	if q.Ext != "" {
		tx = tx.Where("caller_ext = ? OR dest_ext = ?", q.Ext, q.Ext)
	}
	if q.Phone != "" {
		like := "%" + q.Phone + "%"
		tx = tx.Where("caller_num LIKE ? OR dest_num LIKE ?", like, like)
	}
	if q.Direction != "" {
		tx = tx.Where("direction = ?", q.Direction)
	}
	if !q.From.IsZero() {
		tx = tx.Where("start_at >= ?", q.From)
	}
	if !q.To.IsZero() {
		tx = tx.Where("start_at < ?", q.To)
	}
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("call records could not be counted: %w", err)
	}
	var rows []models.PBXCDR
	err := tx.Order("start_at DESC").Offset((q.Page - 1) * q.Limit).Limit(q.Limit).Find(&rows).Error
	if err != nil {
		return nil, 0, fmt.Errorf("call records could not be listed: %w", err)
	}
	return rows, total, nil
}

// LatestCDRAt returns the newest mirrored start time; ok is false while the
// mirror is empty.
func (r *Repository) LatestCDRAt(ctx context.Context) (time.Time, bool, error) {
	var row models.PBXCDR
	err := r.db.WithContext(ctx).Order("start_at DESC").First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, fmt.Errorf("latest call record could not be read: %w", err)
	}
	return row.StartAt, true, nil
}

// MirrorCursor returns the oldest day (YYYY-MM-DD) known to be fully
// mirrored, or "" before the first backfill.
func (r *Repository) MirrorCursor(ctx context.Context) (string, error) {
	var values []string
	err := r.db.WithContext(ctx).Model(&models.SystemSetting{}).
		Where("key = ?", mirrorCursorKey).Limit(1).Pluck("value", &values).Error
	if err != nil {
		return "", fmt.Errorf("mirror cursor could not be read: %w", err)
	}
	if len(values) == 0 {
		return "", nil
	}
	return values[0], nil
}

// SetMirrorCursor records that every day on or after day is mirrored.
func (r *Repository) SetMirrorCursor(ctx context.Context, day string) error {
	row := models.SystemSetting{Key: mirrorCursorKey, Value: day, UpdatedAt: time.Now()}
	err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at"}),
	}).Create(&row).Error
	if err != nil {
		return fmt.Errorf("mirror cursor could not be saved: %w", err)
	}
	return nil
}

// dayBounds returns the Istanbul-local [start, next day start) of a
// YYYY-MM-DD day.
func dayBounds(day string) (time.Time, time.Time, error) {
	t, err := time.ParseInLocation("2006-01-02", day, istanbul)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	return t, t.AddDate(0, 0, 1), nil
}

// dayParams builds the hosted API's UTC range parameters for one local day.
func dayParams(day string) (url.Values, error) {
	from, to, err := dayBounds(day)
	if err != nil {
		return nil, err
	}
	p := url.Values{}
	p.Set("start_stamp_from", from.UTC().Format("2006-01-02 15:04:05")+" UTC")
	p.Set("start_stamp_to", to.UTC().Format("2006-01-02 15:04:05")+" UTC")
	return p, nil
}

// runMirror is the background backfill. It first repairs any gap left by
// downtime (re-fetching the days since the newest mirrored record), then
// walks older days down to the configured history, one slow request at a
// time. Progress is stored, so a restart resumes instead of starting over.
func (s *Service) runMirror(ctx context.Context) {
	// Let the regular poll's startup burst pass first.
	if !sleepCtx(ctx, 20*time.Second) {
		return
	}
	today := time.Now().In(istanbul)
	todayDay := today.Format("2006-01-02")

	latest, ok, err := s.repo.LatestCDRAt(ctx)
	if err != nil {
		slog.WarnContext(ctx, "cdr mirror could not read its state", "error", err)
	}
	if ok && time.Since(latest) > mirrorRepairAfter {
		for d := today; !d.Before(latest.In(istanbul).Truncate(24 * time.Hour)); d = d.AddDate(0, 0, -1) {
			day := d.Format("2006-01-02")
			if day < latest.In(istanbul).Format("2006-01-02") {
				break
			}
			if !s.mirrorDay(ctx, day) {
				return
			}
		}
	}

	days := s.cfg.HistoryDays
	if days <= 0 {
		days = defaultHistoryDays
	}
	floor := today.AddDate(0, 0, -days).Format("2006-01-02")
	cursor, err := s.repo.MirrorCursor(ctx)
	if err != nil {
		slog.WarnContext(ctx, "cdr mirror could not read its cursor", "error", err)
		return
	}
	next := todayDay
	if cursor != "" {
		t, err := time.ParseInLocation("2006-01-02", cursor, istanbul)
		if err != nil {
			slog.WarnContext(ctx, "cdr mirror cursor is malformed, restarting backfill", "cursor", cursor)
		} else {
			next = t.AddDate(0, 0, -1).Format("2006-01-02")
		}
	}
	for day := next; day >= floor; {
		if !s.mirrorDay(ctx, day) {
			return
		}
		if err := s.repo.SetMirrorCursor(ctx, day); err != nil {
			slog.WarnContext(ctx, "cdr mirror cursor could not be saved", "day", day, "error", err)
			return
		}
		t, _ := time.ParseInLocation("2006-01-02", day, istanbul)
		day = t.AddDate(0, 0, -1).Format("2006-01-02")
	}
	slog.InfoContext(ctx, "cdr mirror backfill complete", "oldestDay", floor)
}

// mirrorDay fetches every page of one local day into the mirror. It returns
// false when the context ended or the day could not be completed, so the
// caller stops without advancing the cursor.
func (s *Service) mirrorDay(ctx context.Context, day string) bool {
	base, err := dayParams(day)
	if err != nil {
		slog.WarnContext(ctx, "cdr mirror skipped a malformed day", "day", day)
		return true
	}
	written := 0
	for page := 1; ; page++ {
		p := url.Values{}
		for k, v := range base {
			p[k] = append([]string(nil), v...)
		}
		p.Set("page", strconv.Itoa(page))
		p.Set("limit", strconv.Itoa(mirrorPageSize))
		var cdrs []CDR
		var pg Pagination
		for attempt := 1; ; attempt++ {
			cdrs, pg, err = s.client.CDRsSlow(ctx, p)
			if err == nil {
				break
			}
			if attempt >= mirrorRetries || ctx.Err() != nil {
				slog.WarnContext(ctx, "cdr mirror gave up on a day for now", "day", day, "page", page, "error", err)
				return false
			}
			if !sleepCtx(ctx, mirrorRetryGap) {
				return false
			}
		}
		n, err := s.repo.UpsertCDRs(ctx, cdrs)
		if err != nil {
			slog.WarnContext(ctx, "cdr mirror could not store a page", "day", day, "page", page, "error", err)
			return false
		}
		written += n
		if len(cdrs) < mirrorPageSize || (pg.TotalPages > 0 && pg.Page >= pg.TotalPages) {
			break
		}
		if !sleepCtx(ctx, mirrorRequestGap) {
			return false
		}
	}
	slog.InfoContext(ctx, "cdr mirror stored a day", "day", day, "records", written)
	return sleepCtx(ctx, mirrorRequestGap)
}
