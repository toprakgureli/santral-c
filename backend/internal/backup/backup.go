// Package backup copies the database to a Google Shared Drive every few
// hours. The service account it uses is only a Contributor there, so it can
// add files and never delete them: a server taken over cannot wipe its own
// backups. Before every upload the backup asks Drive what the account may
// do in the folder and refuses to run if it could delete. After the upload
// the file is locked read-only with a lock only a Shared Drive organizer can
// lift, so it cannot be overwritten or lose its earlier revisions either; a
// copy that cannot be locked counts as failed. Restoring is a manual
// pg_restore of one of the files.
package backup

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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
	return &Service{db: db, dbCfg: dbCfg, ring: ring, users: users, audit: auditor, http: &http.Client{}}
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
// folder. When the folder looks right it also adds a small file there and
// locks it, to see that backups can be locked; that file stays, as every
// backup does.
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
	if check.Problem() == "" {
		ok := true
		if err := g.tryLock(ctx, token, st.FolderID); err != nil {
			ok = false
			check.LockError = err.Error()
		}
		check.CanLock = &ok
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

// Start runs a backup whenever the last good one is older than Every,
// checking every ten minutes, until ctx ends.
func (s *Service) Start(ctx context.Context, g *safe.Group) {
	g.Loop(ctx, "backup", func(ctx context.Context) {
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

func (s *Service) dueRun(ctx context.Context) {
	st, err := s.settings(ctx)
	if err != nil || !st.Enabled {
		return
	}
	var last *time.Time
	if err := s.db.WithContext(ctx).Raw("SELECT max(started_at) FROM backup_runs WHERE started_by IS NULL").Scan(&last).Error; err != nil {
		slog.WarnContext(ctx, "last backup could not be read", "error", err)
		return
	}
	if last != nil && time.Since(*last) < Every {
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
		fields["error"] = err.Error()
		slog.ErrorContext(ctx, "backup failed", "error", err)
	} else {
		slog.InfoContext(ctx, "backup stored", "file", file, "bytes", size)
	}
	if err := s.db.WithContext(ctx).Model(&models.BackupRun{}).Where("id = ?", row.ID).Updates(fields).Error; err != nil {
		slog.ErrorContext(ctx, "backup outcome could not be recorded", "error", err)
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
	dir, path, err := dumpDatabase(ctx, s.dbCfg)
	if err != nil {
		return "", 0, err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	name := "santral-" + time.Now().In(tz.Istanbul).Format("2006-01-02-1504") + ".dump"
	// A long upload may outlive the first token.
	if token, err = g.token(ctx); err != nil {
		return "", 0, err
	}
	file, err := os.Open(path)
	if err != nil {
		return "", 0, fmt.Errorf("dump could not be opened: %w", err)
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return "", 0, fmt.Errorf("dump could not be read: %w", err)
	}
	id, err := g.upload(ctx, token, st.FolderID, name, file, info.Size())
	if err != nil {
		return name, 0, err
	}
	if err := g.lock(ctx, token, id); err != nil {
		return name, info.Size(), fmt.Errorf("yedek yüklendi ama kilitlenemedi, bu yüzden sonradan değiştirilebilir: %w. Ayarlardaki \"Bağlantıyı denetle\" ile kilitlemeyi dene", err)
	}
	return name, info.Size(), nil
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
