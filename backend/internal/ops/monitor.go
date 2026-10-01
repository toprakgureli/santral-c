package ops

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/prometheus/client_golang/prometheus"
	goredis "github.com/redis/go-redis/v9"

	"github.com/toprakgureli/santral-c/backend/internal/middlewares"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
	"github.com/toprakgureli/santral-c/backend/pkg/safe"
)

// The system warnings: once a minute the server looks at what tends to go
// wrong unnoticed (the disk filling up, Redis gone, backups late, WhatsApp
// queues backing up) and keeps a short list of warnings in plain words,
// with what to do about each. People holding system.health see them in
// the panel. When PostgreSQL itself is down the panel cannot ask (every
// signed-in request needs the database); Grafana's alerts cover that.

// errNoDiskCheck means the disk cannot be measured on this system; the
// server runs on Linux, so only development machines get it.
var errNoDiskCheck = errors.New("disk usage is read only on Linux")

// Warning levels.
const (
	LevelWarning  = "warning"
	LevelCritical = "critical"
)

// Thresholds of the checks.
const (
	diskWarnRatio     = 0.85
	diskCriticalRatio = 0.95
	backupLateAfter   = 7 * time.Hour
	backupLostAfter   = 24 * time.Hour
	firstBackupGrace  = time.Hour
	outboxLateAfter   = 10 * time.Minute
	outboxLostAfter   = 30 * time.Minute
	inboundBacklog    = 200
	monitorEvery      = time.Minute
	monitorStale      = 2 * time.Minute
)

// Warning is one problem, worded for the people using the panel.
type Warning struct {
	// Key names the check ("disk", "redis", ...).
	Key   string `json:"key"`
	Level string `json:"level"`
	Title string `json:"title"`
	// Text says what is wrong, Action what to do about it.
	Text   string `json:"text"`
	Action string `json:"action"`
	// Link is a panel page where it can be fixed, when there is one.
	Link string `json:"link,omitempty"`
	// Fingerprint changes when the warning changes in a way worth
	// showing again (it gets worse, a new failure happens), so a warning
	// someone closed comes back only then.
	Fingerprint string `json:"fingerprint"`
}

// Report is the current list of warnings.
type Report struct {
	Warnings  []Warning `json:"warnings"`
	CheckedAt time.Time `json:"checkedAt"`
}

// Disk is how full a file system is, in bytes.
type Disk struct {
	Total uint64
	Free  uint64 // free for the database (what df calls available)
	Used  uint64
}

// Ratio is the used part, as df counts it.
func (d Disk) Ratio() float64 {
	if d.Used+d.Free == 0 {
		return 0
	}
	return float64(d.Used) / float64(d.Used+d.Free)
}

// Monitor runs the checks and keeps the last report.
type Monitor struct {
	db       *sql.DB
	redis    *goredis.Client
	dataPath string
	// disk reads a file system's usage; tests replace it.
	disk func(path string) (Disk, error)
	now  func() time.Time

	diskErrLogged atomic.Bool

	mu   sync.Mutex
	last Report
}

// NewMonitor builds the monitor; dataPath is a folder on the database's
// disk.
func NewMonitor(db *sql.DB, redis *goredis.Client, dataPath string) *Monitor {
	return &Monitor{db: db, redis: redis, dataPath: dataPath, disk: diskUsage, now: time.Now}
}

// Start checks once a minute until ctx ends.
func (m *Monitor) Start(ctx context.Context, g *safe.Group) {
	g.Loop(ctx, "system-health", func(ctx context.Context) {
		t := time.NewTicker(monitorEvery)
		defer t.Stop()
		for {
			m.refresh(ctx)
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	})
}

// Current returns the last report, checking again first when it is older
// than two minutes (or there was none yet).
func (m *Monitor) Current(ctx context.Context) Report {
	m.mu.Lock()
	last := m.last
	m.mu.Unlock()
	if last.CheckedAt.IsZero() || m.now().Sub(last.CheckedAt) > monitorStale {
		return m.refresh(ctx)
	}
	return last
}

func (m *Monitor) refresh(ctx context.Context) Report {
	r := Report{Warnings: m.Check(ctx), CheckedAt: m.now()}
	m.mu.Lock()
	m.last = r
	m.mu.Unlock()
	return r
}

// Check runs every check now and returns the warnings, the most urgent
// first.
func (m *Monitor) Check(ctx context.Context) []Warning {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out := make([]Warning, 0)
	add := func(w *Warning) {
		if w != nil {
			out = append(out, *w)
		}
	}
	add(m.checkDisk())
	add(m.checkRedis(ctx))
	if w := m.checkDatabase(ctx); w != nil {
		// Nothing else can be read without the database.
		return append([]Warning{*w}, out...)
	}
	add(m.checkBackup(ctx))
	add(m.checkWebhooks(ctx))
	add(m.checkOutbox(ctx))
	add(m.checkInbound(ctx))
	slices.SortStableFunc(out, func(a, b Warning) int { return urgency(a) - urgency(b) })
	return out
}

func urgency(w Warning) int {
	if w.Level == LevelCritical {
		return 0
	}
	return 1
}

func (m *Monitor) checkDisk() *Warning {
	d, err := m.disk(m.dataPath)
	if err != nil {
		// Said once: a missing folder stays missing every minute.
		if !errors.Is(err, errNoDiskCheck) && m.diskErrLogged.CompareAndSwap(false, true) {
			slog.Warn("disk usage could not be read; set database.dataPath to a folder on the database's disk", "path", m.dataPath, "error", err)
		}
		return nil
	}
	return diskWarning(d)
}

// diskWarning warns from 85 % full and calls it critical from 95 %. It
// shows again after it was closed when the disk fills by another 5 %.
func diskWarning(d Disk) *Warning {
	ratio := d.Ratio()
	if ratio < diskWarnRatio {
		return nil
	}
	pct := int(ratio * 100)
	w := &Warning{Key: "disk", Level: LevelWarning, Title: "Sunucunun diski doluyor",
		Text: fmt.Sprintf("Veritabanının durduğu disk dolmak üzere: dolu kısım yüzde %d, boş yer %s.", pct, humanBytes(d.Free)),
		Action: "Sunucuyu yöneten kişiye haber ver. Eski deploy kopyaları (/var/backups/santral) silinebilir ya da disk büyütülebilir. " +
			"Disk tamamen dolarsa panel yeni kayıt yazamaz.",
	}
	if ratio >= diskCriticalRatio {
		w.Level, w.Title = LevelCritical, "Sunucunun diski neredeyse dolu"
	}
	w.Fingerprint = fingerprint(w.Key, w.Level, strconv.Itoa(pct/5*5))
	return w
}

func (m *Monitor) checkRedis(ctx context.Context) *Warning {
	if m.redis == nil {
		return nil
	}
	pctx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()
	if err := m.redis.Ping(pctx).Err(); err == nil {
		return nil
	}
	return &Warning{Key: "redis", Level: LevelCritical, Title: "Redis cevap vermiyor",
		Text:        "Sunucunun oturum ve deneme sınırı bilgilerini tuttuğu Redis'e ulaşılamıyor. Giriş yapmak ve canlı ekranlar aksayabilir.",
		Action:      "Sunucuyu yöneten kişiye hemen haber ver; Redis servisinin yeniden başlatılması gerekebilir.",
		Fingerprint: fingerprint("redis", LevelCritical),
	}
}

func (m *Monitor) checkDatabase(ctx context.Context) *Warning {
	pctx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()
	if err := m.db.PingContext(pctx); err == nil {
		return nil
	}
	return &Warning{Key: "database", Level: LevelCritical, Title: "Veritabanına ulaşılamıyor",
		Text:        "Sunucu veritabanından cevap alamıyor; yeni kayıtlar yazılamıyor.",
		Action:      "Sunucuyu yöneten kişiye hemen haber ver.",
		Fingerprint: fingerprint("database", LevelCritical),
	}
}

func (m *Monitor) checkBackup(ctx context.Context) *Warning {
	var (
		enabled bool
		since   time.Time
	)
	err := m.db.QueryRowContext(ctx, "SELECT enabled, updated_at FROM backup_settings WHERE id = 1").Scan(&enabled, &since)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		slog.WarnContext(ctx, "backup settings could not be read for the system warnings", "error", err)
		return nil
	}
	if !enabled {
		return nil
	}
	var last sql.NullTime
	if err := m.db.QueryRowContext(ctx, "SELECT max(finished_at) FROM backup_runs WHERE ok").Scan(&last); err != nil {
		slog.WarnContext(ctx, "last backup could not be read for the system warnings", "error", err)
		return nil
	}
	var lastOK *time.Time
	if last.Valid {
		lastOK = &last.Time
	}
	return backupWarning(m.now(), since, lastOK)
}

// backupWarning warns when backups are on and the last good one is older
// than seven hours (critical after a day), or when none ever worked an hour
// after they were switched on.
func backupWarning(now, enabledSince time.Time, lastOK *time.Time) *Warning {
	w := &Warning{Key: "backup", Level: LevelWarning, Title: "Veritabanı yedeği gecikti",
		Action: "Sistem Ayarları > Veritabanı Yedeği bölümünde son denemenin hatasına bak.",
		Link:   "/settings",
	}
	switch {
	case lastOK == nil:
		if now.Sub(enabledSince) < firstBackupGrace {
			return nil
		}
		w.Text = "Yedekleme açık ama henüz başarılı bir yedek alınamadı."
		w.Fingerprint = fingerprint("backup", "never")
		return w
	case now.Sub(*lastOK) < backupLateAfter:
		return nil
	case now.Sub(*lastOK) >= backupLostAfter:
		w.Level = LevelCritical
	}
	w.Text = fmt.Sprintf("Son başarılı yedek %s önce alındı. Yedekler normalde 6 saatte bir alınır.", humanAge(now.Sub(*lastOK)))
	w.Fingerprint = fingerprint("backup", w.Level, strconv.FormatInt(lastOK.Unix(), 10))
	return w
}

func (m *Monitor) checkWebhooks(ctx context.Context) *Warning {
	var (
		n      int64
		latest float64
	)
	err := m.db.QueryRowContext(ctx, `SELECT count(*), COALESCE(EXTRACT(EPOCH FROM max(failed_at)), 0)
		FROM wa_webhook_events WHERE status = 'failed' AND failed_at > now() - interval '1 hour'`).Scan(&n, &latest)
	if err != nil {
		slog.WarnContext(ctx, "failed webhook events could not be counted for the system warnings", "error", err)
		return nil
	}
	return webhookWarning(n, int64(latest))
}

// webhookWarning warns about Meta notices given up on in the last hour; a
// new failure shows a closed warning again.
func webhookWarning(n, latest int64) *Warning {
	if n == 0 {
		return nil
	}
	return &Warning{Key: "webhook", Level: LevelWarning, Title: "WhatsApp bildirimleri işlenemedi",
		Text:        fmt.Sprintf("Son bir saatte Meta'dan gelen %d bildirim işlenemedi. Bazı müşteri mesajları panele düşmemiş olabilir.", n),
		Action:      "WhatsApp > Ayarlar > İşlenemeyenler bölümünden tekrar dene. Yine olmazsa sunucuyu yöneten kişiye haber ver.",
		Link:        "/whatsapp/settings?tab=events",
		Fingerprint: fingerprint("webhook", strconv.FormatInt(latest, 10)),
	}
}

func (m *Monitor) checkOutbox(ctx context.Context) *Warning {
	var (
		n      int64
		oldest float64
	)
	err := m.db.QueryRowContext(ctx, `SELECT count(*), COALESCE(EXTRACT(EPOCH FROM now() - min(created_at)), 0)
		FROM wa_messages WHERE status IN ('queued','sending')`).Scan(&n, &oldest)
	if err != nil {
		slog.WarnContext(ctx, "the WhatsApp send queue could not be read for the system warnings", "error", err)
		return nil
	}
	return outboxWarning(n, time.Duration(oldest)*time.Second)
}

// outboxWarning warns when the oldest message waiting to go out has waited
// ten minutes, critical after half an hour.
func outboxWarning(n int64, oldest time.Duration) *Warning {
	if n == 0 || oldest < outboxLateAfter {
		return nil
	}
	w := &Warning{Key: "outbox", Level: LevelWarning, Title: "WhatsApp mesajları gönderilemiyor",
		Text:   fmt.Sprintf("Gönderilmeyi bekleyen %d mesaj var; en eskisi %s bekliyor.", n, humanAge(oldest)),
		Action: "WhatsApp > Ayarlar > Cihazlar'da numaranın bağlantısına bak. Sorun sürerse sunucuyu yöneten kişiye haber ver.",
		Link:   "/whatsapp/settings?tab=devices",
	}
	if oldest >= outboxLostAfter {
		w.Level = LevelCritical
	}
	w.Fingerprint = fingerprint("outbox", w.Level)
	return w
}

func (m *Monitor) checkInbound(ctx context.Context) *Warning {
	var n int64
	if err := m.db.QueryRowContext(ctx, "SELECT count(*) FROM wa_inbound_jobs").Scan(&n); err != nil {
		slog.WarnContext(ctx, "the inbound job backlog could not be read for the system warnings", "error", err)
		return nil
	}
	return inboundWarning(n)
}

// inboundWarning warns when more than 200 customer messages wait for the
// work that follows them (chatbot, distribution, rules).
func inboundWarning(n int64) *Warning {
	if n <= inboundBacklog {
		return nil
	}
	return &Warning{Key: "inbound", Level: LevelWarning, Title: "Gelen mesajların işleri birikti",
		Text:        fmt.Sprintf("Chatbot, otomatik dağıtım ve kurallar %d mesajın gerisinde kaldı. Sohbetler temsilcilere geç düşebilir.", n),
		Action:      "Birkaç dakikada azalmazsa sunucuyu yöneten kişiye haber ver.",
		Fingerprint: fingerprint("inbound", strconv.FormatInt(n/inboundBacklog, 10)),
	}
}

func fingerprint(parts ...string) string {
	return strings.Join(parts, ":")
}

// humanBytes writes a size the way people read it: "12,4 GB".
func humanBytes(n uint64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := uint64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	s := fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
	return strings.Replace(s, ".", ",", 1)
}

// humanAge writes a duration in the largest whole unit: "14 dakika",
// "9 saat", "2 gün".
func humanAge(d time.Duration) string {
	switch {
	case d >= 48*time.Hour:
		return fmt.Sprintf("%d gün", int(d/(24*time.Hour)))
	case d >= time.Hour:
		return fmt.Sprintf("%d saat", int(d/time.Hour))
	default:
		return fmt.Sprintf("%d dakika", max(1, int(d/time.Minute)))
	}
}

// diskDesc is how full the database's disk is, for Grafana.
var diskDesc = prometheus.NewDesc("santral_database_disk_used_ratio",
	"Used part (0-1) of the file system holding the database files (database.dataPath).", nil, nil)

// Describe sends the disk gauge's description.
func (m *Monitor) Describe(ch chan<- *prometheus.Desc) {
	ch <- diskDesc
}

// Collect reads the disk at scrape time; nothing is sent when it cannot
// be read.
func (m *Monitor) Collect(ch chan<- prometheus.Metric) {
	d, err := m.disk(m.dataPath)
	if err != nil {
		return
	}
	ch <- prometheus.MustNewConstMetric(diskDesc, prometheus.GaugeValue, d.Ratio())
}

// Routes mounts GET /system/health for people holding system.health.
func (m *Monitor) Routes(api fiber.Router, guard fiber.Handler, need middlewares.Requirer) {
	api.Get("/system/health", guard, need(enums.SystemHealth), m.handle)
}

func (m *Monitor) handle(c *fiber.Ctx) error {
	return c.JSON(m.Current(c.UserContext()))
}
