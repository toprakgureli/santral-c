// Package backup copies the database to a Google Shared Drive every few
// hours. The service account it uses is only a Contributor there, so it can
// add files and never delete them: a server taken over cannot wipe its own
// backups. Before every upload the backup asks Drive what the account may
// do in the folder and refuses to run if it could delete. Restoring is a
// manual pg_restore of one of the files.
package backup

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/toprakgureli/santral-c/backend/configs"
	"github.com/toprakgureli/santral-c/backend/internal/audit"
	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/pkg/crypt"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
	"github.com/toprakgureli/santral-c/backend/pkg/safe"
	"github.com/toprakgureli/santral-c/backend/pkg/tz"
)

// SealPurpose is the keyring label of the stored service account key.
const SealPurpose = "backup"

// Every is how often the database is copied.
const Every = 6 * time.Hour

// Retrying and deadlines. A failed backup is tried again after RetryAfter,
// then after twice that, and so on up to Every. The dump and the upload
// each get a deadline, so a hung pg_dump or a stuck connection to Drive
// cannot hold the backups up for ever; tests shorten them.
var (
	RetryAfter    = 30 * time.Minute
	dumpTimeout   = time.Hour
	uploadTimeout = time.Hour
	// callTimeout bounds the short calls to Google (sign-in, folder check).
	callTimeout = 30 * time.Second
)

// IActorResolver loads the acting user for authorization.
type IActorResolver interface {
	GetByID(ctx context.Context, id uint) (*models.User, error)
}

// IAudit records administrative changes.
type IAudit interface {
	Record(ctx context.Context, e audit.Entry)
}

// Service keeps the backup settings and runs the backups.
type Service struct {
	db    *gorm.DB
	dbCfg configs.Database
	ring  *crypt.Keyring
	users IActorResolver
	audit IAudit
	http  *http.Client

	mu      sync.Mutex
	running bool
}

// NewService builds the backup service.
func NewService(db *gorm.DB, dbCfg configs.Database, ring *crypt.Keyring, users IActorResolver, auditor IAudit) *Service {
	return &Service{db: db, dbCfg: dbCfg, ring: ring, users: users, audit: auditor, http: driveClient()}
}

// driveClient talks to Google. It has no overall time limit, since the
// upload of a big dump takes long; connecting and waiting for an answer
// are bounded here instead, and every call has a deadline of its own.
func driveClient() *http.Client {
	dialer := &net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}
	return &http.Client{Transport: &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           dialer.DialContext,
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: 2 * time.Minute,
		IdleConnTimeout:       90 * time.Second,
		ForceAttemptHTTP2:     true,
	}}
}

// Run is one backup, as the settings page lists it.
type Run struct {
	ID         uint       `json:"id"`
	StartedAt  time.Time  `json:"startedAt"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
	OK         bool       `json:"ok"`
	File       string     `json:"file"`
	Size       int64      `json:"size"`
	Error      string     `json:"error,omitempty"`
	Manual     bool       `json:"manual"`
}

// View is the backup settings without the key itself.
type View struct {
	Enabled      bool   `json:"enabled"`
	FolderID     string `json:"folderId"`
	Account      string `json:"account"`
	HasKey       bool   `json:"hasKey"`
	EveryHours   int    `json:"everyHours"`
	Running      bool   `json:"running"`
	Runs         []Run  `json:"runs"`
	PgDumpExists bool   `json:"pgDumpExists"`
}

// Input changes the settings. An empty Credentials keeps the stored key.
type Input struct {
	Enabled     bool   `json:"enabled"`
	FolderID    string `json:"folderId"`
	Credentials string `json:"credentials"`
}

func (s *Service) authorize(ctx context.Context, actorID uint) (*models.User, error) {
	u, err := s.users.GetByID(ctx, actorID)
	if err != nil {
		return nil, err
	}
	if !u.Can(enums.SystemBackup) {
		return nil, errs.Forbidden("Yedekleme ayarlarını yönetme yetkin yok.")
	}
	return u, nil
}

func (s *Service) settings(ctx context.Context) (*models.BackupSettings, error) {
	var st models.BackupSettings
	err := s.db.WithContext(ctx).Where("id = 1").Limit(1).Find(&st).Error
	if err != nil {
		return nil, fmt.Errorf("backup settings could not be read: %w", err)
	}
	return &st, nil
}

// Settings returns the settings and the last runs.
func (s *Service) Settings(ctx context.Context, actorID uint) (*View, error) {
	if _, err := s.authorize(ctx, actorID); err != nil {
		return nil, err
	}
	return s.view(ctx)
}

func (s *Service) view(ctx context.Context) (*View, error) {
	st, err := s.settings(ctx)
	if err != nil {
		return nil, errs.Internal(err)
	}
	var rows []models.BackupRun
	if err := s.db.WithContext(ctx).Order("started_at DESC").Limit(12).Find(&rows).Error; err != nil {
		return nil, errs.Internal(fmt.Errorf("backup runs could not be listed: %w", err))
	}
	runs := make([]Run, 0, len(rows))
	for _, r := range rows {
		runs = append(runs, Run{ID: r.ID, StartedAt: r.StartedAt, FinishedAt: r.FinishedAt, OK: r.OK, File: r.File, Size: r.Size, Error: r.Error, Manual: r.StartedBy != nil})
	}
	s.mu.Lock()
	running := s.running
	s.mu.Unlock()
	return &View{Enabled: st.Enabled, FolderID: st.FolderID, Account: st.Account, HasKey: st.CredentialsEnc != "",
		EveryHours: int(Every / time.Hour), Running: running, Runs: runs, PgDumpExists: pgDumpFound()}, nil
}

// Save stores the settings.
func (s *Service) Save(ctx context.Context, actorID uint, in Input, ip string) (*View, error) {
	if _, err := s.authorize(ctx, actorID); err != nil {
		return nil, err
	}
	folder := strings.TrimSpace(in.FolderID)
	if strings.Contains(folder, "/folders/") {
		// A pasted folder link: keep the id at its end.
		folder = folder[strings.LastIndex(folder, "/folders/")+len("/folders/"):]
		folder, _, _ = strings.Cut(folder, "?")
	}
	if len(folder) > 200 || strings.ContainsAny(folder, " /") {
		return nil, errs.Invalid("Klasör kimliği okunamadı; klasörün adresini ya da kimliğini yapıştır.", nil)
	}
	fields := map[string]any{"enabled": in.Enabled, "folder_id": folder, "updated_at": time.Now()}
	if raw := strings.TrimSpace(in.Credentials); raw != "" {
		sa, err := parseServiceAccount(raw)
		if err != nil {
			return nil, errs.Invalid(err.Error(), nil)
		}
		enc, err := s.ring.Seal(SealPurpose, raw)
		if err != nil {
			return nil, errs.Internal(err)
		}
		fields["credentials_enc"], fields["account"] = enc, sa.ClientEmail
	}
	st, err := s.settings(ctx)
	if err != nil {
		return nil, errs.Internal(err)
	}
	if in.Enabled && (folder == "" || (st.CredentialsEnc == "" && fields["credentials_enc"] == nil)) {
		return nil, errs.Invalid("Yedeklemeyi açmak için klasör ve servis hesabı anahtarı girilmeli.", nil)
	}
	if err := s.db.WithContext(ctx).Exec(`INSERT INTO backup_settings (id) VALUES (1) ON CONFLICT (id) DO NOTHING`).Error; err != nil {
		return nil, errs.Internal(fmt.Errorf("backup settings could not be created: %w", err))
	}
	if err := s.db.WithContext(ctx).Model(&models.BackupSettings{}).Where("id = 1").Updates(fields).Error; err != nil {
		return nil, errs.Internal(fmt.Errorf("backup settings could not be saved: %w", err))
	}
	s.audit.Record(ctx, audit.Entry{ActorID: &actorID, Action: enums.AuditBackupSettings, TargetType: "backup", IP: ip,
		Detail: map[string]any{"enabled": in.Enabled, "folderId": folder, "keyChanged": fields["credentials_enc"] != nil}})
	return s.view(ctx)
}

// Check signs in as the service account and reports what it may do in the
// folder. It writes nothing.
func (s *Service) Check(ctx context.Context, actorID uint) (*FolderCheck, error) {
	if _, err := s.authorize(ctx, actorID); err != nil {
		return nil, err
	}
	st, err := s.settings(ctx)
	if err != nil {
		return nil, errs.Internal(err)
	}
	g, err := s.client(st)
	if err != nil {
		return nil, errs.Invalid(err.Error(), nil)
	}
	token, err := g.token(ctx)
	if err != nil {
		return nil, errs.Invalid(err.Error(), nil)
	}
	check, err := g.checkFolder(ctx, token, st.FolderID)
	if err != nil {
		return nil, errs.Invalid(err.Error(), nil)
	}
	return check, nil
}

// RunNow starts a backup at once, unless one is running.
func (s *Service) RunNow(ctx context.Context, actorID uint, ip string) (*View, error) {
	if _, err := s.authorize(ctx, actorID); err != nil {
		return nil, err
	}
	bg := context.WithoutCancel(ctx)
	if s.begin() {
		s.audit.Record(ctx, audit.Entry{ActorID: &actorID, Action: enums.AuditBackupStarted, TargetType: "backup", IP: ip})
		safe.Go(bg, "backup", func() {
			defer s.end()
			s.run(bg, &actorID)
		})
	}
	return s.view(ctx)
}

// Start runs a backup whenever one is due (see due), checking every ten
// minutes, until ctx ends. A run the previous server left unfinished is
// closed as failed first, so it does not look like it still runs.
func (s *Service) Start(ctx context.Context, g *safe.Group) {
	g.Loop(ctx, "backup", func(ctx context.Context) {
		s.closeUnfinished(ctx)
		t := time.NewTicker(10 * time.Minute)
		defer t.Stop()
		for {
			s.dueRun(ctx)
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	})
}

// closeUnfinished marks runs a stopped server left open as failed. Only
// one server runs at a time, so none of them is still going.
func (s *Service) closeUnfinished(ctx context.Context) {
	if s.isRunning() {
		return
	}
	err := s.db.WithContext(ctx).Exec(`UPDATE backup_runs SET finished_at = now(), ok = false,
		error = 'Yedek yarıda kaldı: sunucu bu sırada yeniden başladı.' WHERE finished_at IS NULL`).Error
	if err != nil {
		slog.WarnContext(ctx, "unfinished backups could not be closed", "error", err)
	}
}

// history is what the schedule looks at: the start of the last good run,
// and the tries that failed (or never finished) after it.
type history struct {
	lastOK   *time.Time
	failures int
	lastTry  *time.Time
}

// due tells whether a backup should start now. The next one is Every
// after the start of the last good run. A failure is tried again after
// RetryAfter, doubling with every further failure up to Every, but never
// before the regular time: a manual run that failed an hour after a good
// one does not bring the next backup forward.
func due(now time.Time, h history) bool {
	if h.lastOK == nil && h.lastTry == nil {
		return true
	}
	var next time.Time
	if h.lastOK != nil {
		next = h.lastOK.Add(Every)
	}
	if h.failures > 0 && h.lastTry != nil {
		wait := RetryAfter
		for i := 1; i < h.failures && wait < Every; i++ {
			wait *= 2
		}
		if retry := h.lastTry.Add(min(wait, Every)); retry.After(next) {
			next = retry
		}
	}
	return !now.Before(next)
}

// history reads the schedule's inputs from the runs.
func (s *Service) history(ctx context.Context) (history, error) {
	var h history
	var ok struct {
		LastOK *time.Time
	}
	if err := s.db.WithContext(ctx).Raw("SELECT max(started_at) AS last_ok FROM backup_runs WHERE ok").Scan(&ok).Error; err != nil {
		return h, err
	}
	h.lastOK = ok.LastOK
	var tries struct {
		N       int
		LastTry *time.Time
	}
	q := s.db.WithContext(ctx).Raw("SELECT count(*) AS n, max(started_at) AS last_try FROM backup_runs WHERE NOT ok")
	if h.lastOK != nil {
		q = s.db.WithContext(ctx).Raw("SELECT count(*) AS n, max(started_at) AS last_try FROM backup_runs WHERE NOT ok AND started_at > ?", *h.lastOK)
	}
	if err := q.Scan(&tries).Error; err != nil {
		return h, err
	}
	h.failures, h.lastTry = tries.N, tries.LastTry
	return h, nil
}

func (s *Service) dueRun(ctx context.Context) {
	st, err := s.settings(ctx)
	if err != nil || !st.Enabled {
		return
	}
	h, err := s.history(ctx)
	if err != nil {
		slog.WarnContext(ctx, "last backup could not be read", "error", err)
		return
	}
	if !due(time.Now(), h) {
		return
	}
	if !s.begin() {
		return
	}
	defer s.end()
	s.run(ctx, nil)
}

func (s *Service) begin() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return false
	}
	s.running = true
	return true
}

func (s *Service) isRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

func (s *Service) end() {
	s.mu.Lock()
	s.running = false
	s.mu.Unlock()
}

// run makes one backup and records how it went.
func (s *Service) run(ctx context.Context, by *uint) {
	row := models.BackupRun{StartedAt: time.Now(), StartedBy: by}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		slog.ErrorContext(ctx, "backup run could not be recorded", "error", err)
		return
	}
	file, size, err := s.copy(ctx)
	now := time.Now()
	fields := map[string]any{"finished_at": now, "ok": err == nil, "file": file, "size": size}
	if err != nil {
		if ctx.Err() != nil {
			err = fmt.Errorf("yedek yarıda kesildi, sunucu duruyordu: %w", err)
		}
		fields["error"] = err.Error()
		slog.ErrorContext(ctx, "backup failed", "error", err)
	} else {
		slog.InfoContext(ctx, "backup stored", "file", file, "bytes", size)
	}
	// The outcome is written even when the server is stopping and ctx is
	// over, so the run does not stay open on the settings page.
	wctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if err := s.db.WithContext(wctx).Model(&models.BackupRun{}).Where("id = ?", row.ID).Updates(fields).Error; err != nil {
		slog.ErrorContext(wctx, "backup outcome could not be recorded", "error", err)
	}
}

func (s *Service) copy(ctx context.Context) (string, int64, error) {
	st, err := s.settings(ctx)
	if err != nil {
		return "", 0, err
	}
	g, err := s.client(st)
	if err != nil {
		return "", 0, err
	}
	token, err := g.token(ctx)
	if err != nil {
		return "", 0, err
	}
	check, err := g.checkFolder(ctx, token, st.FolderID)
	if err != nil {
		return "", 0, err
	}
	if p := check.Problem(); p != "" {
		return "", 0, errors.New(p)
	}
	dctx, cancelDump := context.WithTimeout(ctx, dumpTimeout)
	dir, path, err := dumpDatabase(dctx, s.dbCfg)
	timedOut := errors.Is(dctx.Err(), context.DeadlineExceeded)
	cancelDump()
	if err != nil {
		if timedOut {
			return "", 0, fmt.Errorf("pg_dump %s içinde bitmedi: %w", dumpTimeout, err)
		}
		return "", 0, err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	name := "santral-" + time.Now().In(tz.Istanbul).Format("2006-01-02-1504") + ".dump"
	// A long dump may outlive the first token.
	if token, err = g.token(ctx); err != nil {
		return "", 0, err
	}
	uctx, cancelUpload := context.WithTimeout(ctx, uploadTimeout)
	defer cancelUpload()
	_, size, err := g.upload(uctx, token, st.FolderID, name, path)
	if err != nil {
		if errors.Is(uctx.Err(), context.DeadlineExceeded) {
			return name, 0, fmt.Errorf("yükleme %s içinde bitmedi: %w", uploadTimeout, err)
		}
		return name, 0, err
	}
	return name, size, nil
}

func (s *Service) client(st *models.BackupSettings) (*google, error) {
	if st.FolderID == "" || st.CredentialsEnc == "" {
		return nil, errors.New("önce klasörü ve servis hesabı anahtarını kaydet")
	}
	raw, err := s.ring.Open(SealPurpose, st.CredentialsEnc)
	if err != nil {
		return nil, errors.New("kayıtlı anahtar açılamadı; anahtarı yeniden gir")
	}
	sa, err := parseServiceAccount(raw)
	if err != nil {
		return nil, err
	}
	return &google{sa: sa, http: s.http}, nil
}

// pgDumpFound reports whether the dump program can be found, for the
// settings page.
func pgDumpFound() bool {
	_, err := lookPath(pgDump())
	return err == nil
}
