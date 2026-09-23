package teams

// Attachments: the service side of file sharing. Limits, the upload
// handshake with Drive, binding files to lines, serving them back to
// seated people and sweeping uploads that never became a message.

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"log"
	"net/http"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
)

// Size limits per kind.
const (
	MaxImageBytes int64 = 200 << 20
	MaxVideoBytes int64 = 1 << 30
	MaxFileBytes  int64 = 3 << 30

	maxAttachmentsPerLine = 10
	thumbPrefix           = "data:image/webp;base64,"
	thumbMaxBytes         = 160 << 10
	thumbMaxSide          = 640
	orphanTTL             = 24 * time.Hour
)

// AttachmentView is an attachment as the browser sees it.
type AttachmentView struct {
	ID         uint   `json:"id"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Mime       string `json:"mime"`
	Size       int64  `json:"size"`
	Width      int    `json:"width,omitempty"`
	Height     int    `json:"height,omitempty"`
	DurationMs int    `json:"durationMs,omitempty"`
	HasThumb   bool   `json:"hasThumb"`
	Ready      bool   `json:"ready"`
}

func attachmentView(a *models.ChatAttachment) AttachmentView {
	return AttachmentView{ID: a.ID, Kind: a.Kind, Name: a.Name, Mime: a.Mime, Size: a.Size, Width: a.Width, Height: a.Height, DurationMs: a.DurationMs, HasThumb: a.Thumb != "", Ready: a.Status == "ready"}
}

// kindOf sorts a mime type into image, video or file.
func kindOf(mime string) string {
	switch {
	case strings.HasPrefix(mime, "image/"):
		return "image"
	case strings.HasPrefix(mime, "video/"):
		return "video"
	default:
		return "file"
	}
}

// limitFor is the byte ceiling of a kind.
func limitFor(kind string) int64 {
	switch kind {
	case "image":
		return MaxImageBytes
	case "video":
		return MaxVideoBytes
	default:
		return MaxFileBytes
	}
}

func human(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/float64(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.0f MB", float64(n)/float64(1<<20))
	default:
		return fmt.Sprintf("%d KB", n>>10)
	}
}

// UploadInput describes the file the browser is about to send.
type UploadInput struct {
	Name string
	Mime string
	Size int64
}

// UploadSession is what the browser needs to push bytes to Drive.
type UploadSession struct {
	AttachmentID uint   `json:"attachmentId"`
	UploadURL    string `json:"uploadUrl"`
	ChunkBytes   int    `json:"chunkBytes"`
}

// BeginUpload checks the limits, records the pending attachment and opens
// the Drive session for it.
func (s *Service) BeginUpload(ctx context.Context, actorID, groupID uint, in UploadInput, origin string) (*UploadSession, error) {
	actor, err := s.actor(ctx, actorID)
	if err != nil {
		return nil, err
	}
	g, m, err := s.seat(ctx, actor, groupID)
	if err != nil {
		return nil, err
	}
	if !canPost(g, m) {
		return nil, errs.Forbidden("Bu grupta yazma yetkiniz yok.")
	}
	if !s.drive.Configured() {
		return nil, errs.Invalid("Dosya paylaşımı kapalı: Google Drive yapılandırılmamış.", nil)
	}
	if !s.drive.Connected(ctx) {
		return nil, errs.Invalid("Dosya paylaşımı kapalı: Google Drive hesabı bağlanmamış. Yönetim ekranından bağlayın.", nil)
	}
	name := strings.TrimSpace(path.Base(strings.ReplaceAll(in.Name, "\\", "/")))
	if name == "" || name == "." || name == "/" {
		name = "dosya"
	}
	if len([]rune(name)) > 200 {
		name = string([]rune(name)[:200])
	}
	mime := strings.ToLower(strings.TrimSpace(in.Mime))
	if mime == "" {
		mime = "application/octet-stream"
	}
	kind := kindOf(mime)
	if in.Size <= 0 {
		return nil, errs.Invalid("Dosya boş görünüyor.", nil)
	}
	if limit := limitFor(kind); in.Size > limit {
		label := map[string]string{"image": "Görsel", "video": "Video", "file": "Dosya"}[kind]
		return nil, errs.Invalid(fmt.Sprintf("%s en fazla %s olabilir (%s).", label, human(limit), human(in.Size)), nil)
	}
	row := &models.ChatAttachment{GroupID: groupID, UploaderID: actorID, Kind: kind, Name: name, Mime: mime, Size: in.Size, Status: "pending", CreatedAt: time.Now()}
	if err := s.repo.CreateAttachment(ctx, row); err != nil {
		return nil, errs.Internal(err)
	}
	// The Drive name carries our id so the finish step can prove the file
	// came from this very session.
	driveName := fmt.Sprintf("att-%d-%s", row.ID, name)
	folder, err := s.roomFolder(ctx, g)
	if err != nil {
		_ = s.repo.DeleteAttachmentRow(ctx, row.ID)
		return nil, errs.Invalid("Drive klasörü hazırlanamadı: "+err.Error(), nil)
	}
	loc, err := s.drive.StartUpload(ctx, folder, driveName, mime, in.Size, origin)
	if err != nil && g.DriveFolder != "" {
		// The room folder may have been removed by hand in Drive: forget it,
		// make it again and try once more.
		_ = s.repo.UpdateGroup(ctx, g.ID, map[string]any{"drive_folder": ""})
		g.DriveFolder = ""
		if folder, err = s.roomFolder(ctx, g); err == nil {
			loc, err = s.drive.StartUpload(ctx, folder, driveName, mime, in.Size, origin)
		}
	}
	if err != nil {
		_ = s.repo.DeleteAttachmentRow(ctx, row.ID)
		return nil, errs.Invalid("Yükleme başlatılamadı: "+err.Error(), nil)
	}
	return &UploadSession{AttachmentID: row.ID, UploadURL: loc, ChunkBytes: 8 << 20}, nil
}

// roomFolderName is the Drive folder name of a room: the group's name
// with its id (two groups may share a name), or the two people of a
// direct message.
func (s *Service) roomFolderName(ctx context.Context, g *models.ChatGroup) string {
	clean := func(v string) string {
		v = strings.NewReplacer("/", "-", "\\", "-").Replace(strings.TrimSpace(v))
		if v == "" {
			v = "oda"
		}
		return v
	}
	if g.Kind != "dm" {
		return fmt.Sprintf("%s (#%d)", clean(g.Name), g.ID)
	}
	ids, _ := s.repo.MemberIDs(ctx, g.ID)
	people, _ := s.repo.PeopleByID(ctx, ids)
	names := make([]string, 0, len(people))
	for _, p := range people {
		names = append(names, clean(p.Name))
	}
	sort.Strings(names)
	if len(names) == 0 {
		return fmt.Sprintf("Sohbet (#%d)", g.ID)
	}
	return strings.Join(names, " - ")
}

// roomFolder returns the room's Drive folder, building the tree on first
// use: SantralC / Gruplar / <ad (#id)> or SantralC / Özel Mesajlar / <a - b>.
func (s *Service) roomFolder(ctx context.Context, g *models.ChatGroup) (string, error) {
	if g.DriveFolder != "" {
		return g.DriveFolder, nil
	}
	root, err := s.drive.Folder(ctx)
	if err != nil {
		return "", err
	}
	branch := "Gruplar"
	if g.Kind == "dm" {
		branch = "Özel Mesajlar"
	}
	parent, err := s.drive.EnsureFolder(ctx, branch, root)
	if err != nil {
		return "", err
	}
	id, err := s.drive.EnsureFolder(ctx, s.roomFolderName(ctx, g), parent)
	if err != nil {
		return "", err
	}
	if err := s.repo.UpdateGroup(ctx, g.ID, map[string]any{"drive_folder": id}); err != nil {
		return "", err
	}
	g.DriveFolder = id
	return id, nil
}

// FinishInput is what the browser reports once Drive has the bytes.
type FinishInput struct {
	DriveID    string
	Width      int
	Height     int
	DurationMs int
	Thumb      string
}

// FinishUpload verifies the Drive file against the pending row and marks
// it ready. The thumbnail is checked like a profile photo.
func (s *Service) FinishUpload(ctx context.Context, actorID, attachmentID uint, in FinishInput) (*AttachmentView, error) {
	a, err := s.repo.Attachment(ctx, attachmentID)
	if err != nil {
		return nil, errs.Internal(err)
	}
	if a == nil || a.UploaderID != actorID || a.DeletedAt != nil {
		return nil, errs.NotFound("Yükleme bulunamadı.")
	}
	if a.Status == "ready" {
		v := attachmentView(a)
		return &v, nil
	}
	if strings.TrimSpace(in.DriveID) == "" {
		return nil, errs.Invalid("Drive dosya kimliği eksik.", nil)
	}
	f, err := s.drive.File(ctx, in.DriveID)
	if err != nil {
		return nil, errs.Invalid("Dosya Drive'da doğrulanamadı: "+err.Error(), nil)
	}
	if !strings.HasPrefix(f.Name, fmt.Sprintf("att-%d-", a.ID)) || f.Size != a.Size {
		return nil, errs.Invalid("Drive'daki dosya bu yüklemeyle eşleşmiyor.", nil)
	}
	thumb := strings.TrimSpace(in.Thumb)
	if thumb != "" {
		if err := checkThumb(thumb); err != nil {
			return nil, err
		}
	}
	fields := map[string]any{"drive_id": f.ID, "status": "ready", "thumb": thumb}
	if in.Width > 0 && in.Height > 0 && in.Width <= 20000 && in.Height <= 20000 {
		fields["width"] = in.Width
		fields["height"] = in.Height
	}
	if in.DurationMs > 0 && in.DurationMs < 24*3600*1000 {
		fields["duration_ms"] = in.DurationMs
	}
	if err := s.repo.UpdateAttachment(ctx, a.ID, fields); err != nil {
		return nil, errs.Internal(err)
	}
	fresh, err := s.repo.Attachment(ctx, a.ID)
	if err != nil || fresh == nil {
		return nil, errs.Internal(err)
	}
	v := attachmentView(fresh)
	return &v, nil
}

func checkThumb(thumb string) error {
	if !strings.HasPrefix(thumb, thumbPrefix) {
		return errs.Invalid("Önizleme webp biçiminde olmalı.", nil)
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(thumb, thumbPrefix))
	if err != nil || len(raw) > thumbMaxBytes {
		return errs.Invalid("Önizleme okunamadı ya da çok büyük.", nil)
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil || format != "webp" || cfg.Width > thumbMaxSide || cfg.Height > thumbMaxSide {
		return errs.Invalid("Önizleme geçerli bir webp değil ya da çok büyük.", nil)
	}
	return nil
}

// CancelUpload drops a pending upload the actor gave up on.
func (s *Service) CancelUpload(ctx context.Context, actorID, attachmentID uint) error {
	a, err := s.repo.Attachment(ctx, attachmentID)
	if err != nil {
		return errs.Internal(err)
	}
	if a == nil || a.UploaderID != actorID {
		return errs.NotFound("Yükleme bulunamadı.")
	}
	if a.MessageID != nil {
		return errs.Invalid("Gönderilmiş bir ek bu yoldan silinemez.", nil)
	}
	if a.DriveID != "" {
		go func(id string) {
			_ = s.drive.Delete(context.Background(), id)
		}(a.DriveID)
	}
	if err := s.repo.DeleteAttachmentRow(ctx, a.ID); err != nil {
		return errs.Internal(err)
	}
	return nil
}

// bindAttachments attaches ready uploads of the actor to a freshly sent
// line. Ids that are not the actor's, not ready or already used are
// ignored rather than failing the whole message.
func (s *Service) bindAttachments(ctx context.Context, actorID, groupID, messageID uint, ids []uint) ([]models.ChatAttachment, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	if len(ids) > maxAttachmentsPerLine {
		ids = ids[:maxAttachmentsPerLine]
	}
	if err := s.repo.BindAttachments(ctx, ids, actorID, groupID, messageID); err != nil {
		return nil, errs.Internal(err)
	}
	byMsg, err := s.repo.AttachmentsByMessage(ctx, []uint{messageID})
	if err != nil {
		return nil, errs.Internal(err)
	}
	return byMsg[messageID], nil
}

// dropAttachments soft-deletes a line's files and removes them from Drive.
func (s *Service) dropAttachments(ctx context.Context, messageID uint) {
	rows, err := s.repo.AttachmentsByMessage(ctx, []uint{messageID})
	if err != nil {
		return
	}
	_ = s.repo.SoftDeleteAttachments(ctx, messageID)
	for _, a := range rows[messageID] {
		if a.DriveID != "" {
			go func(id string) {
				_ = s.drive.Delete(context.Background(), id)
			}(a.DriveID)
		}
	}
}

// OpenAttachment checks the reader's seat and returns the row to stream.
func (s *Service) OpenAttachment(ctx context.Context, actorID, attachmentID uint) (*models.ChatAttachment, error) {
	actor, err := s.actor(ctx, actorID)
	if err != nil {
		return nil, err
	}
	a, err := s.repo.Attachment(ctx, attachmentID)
	if err != nil {
		return nil, errs.Internal(err)
	}
	if a == nil || a.DeletedAt != nil {
		return nil, errs.NotFound("Dosya bulunamadı.")
	}
	if _, _, err := s.seat(ctx, actor, a.GroupID); err != nil {
		return nil, err
	}
	if a.Status != "ready" && a.UploaderID != actorID {
		return nil, errs.NotFound("Dosya henüz hazır değil.")
	}
	return a, nil
}

// StreamAttachment opens the bytes at Drive for a checked row.
func (s *Service) StreamAttachment(ctx context.Context, a *models.ChatAttachment, rangeHeader string) (*http.Response, error) {
	if a.DriveID == "" {
		return nil, errs.NotFound("Dosya henüz hazır değil.")
	}
	resp, err := s.drive.Open(ctx, a.DriveID, rangeHeader)
	if err != nil {
		return nil, errs.Internal(err)
	}
	return resp, nil
}

// StartSweeper removes uploads that never became a message.
func (s *Service) StartSweeper(ctx context.Context) {
	go func() {
		t := time.NewTicker(time.Hour)
		defer t.Stop()
		for {
			s.sweepOrphans(ctx)
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	}()
}

func (s *Service) sweepOrphans(ctx context.Context) {
	rows, err := s.repo.OrphanAttachments(ctx, time.Now().Add(-orphanTTL))
	if err != nil {
		return
	}
	for _, a := range rows {
		if a.DriveID != "" {
			if err := s.drive.Delete(ctx, a.DriveID); err != nil {
				log.Printf("teams: orphan attachment %d could not be removed from Drive: %v", a.ID, err)
				continue
			}
		}
		_ = s.repo.DeleteAttachmentRow(ctx, a.ID)
	}
}

// ---------------------------------------------------------------- drive admin

// DriveStatus is what the settings card shows.
type DriveStatus struct {
	Configured bool   `json:"configured"`
	Connected  bool   `json:"connected"`
	Account    string `json:"account"`
	Folder     string `json:"folder"`
	Limit      int64  `json:"limit"`
	Usage      int64  `json:"usage"`
	Error      string `json:"error,omitempty"`
}

func (s *Service) driveAdmin(ctx context.Context, actorID uint) (*models.User, error) {
	actor, err := s.actor(ctx, actorID)
	if err != nil {
		return nil, err
	}
	if !actor.Can(enums.TeamsAdmin) {
		return nil, errs.Forbidden("Bu işlem için Teams yönetim yetkisi gerekir.")
	}
	return actor, nil
}

// DriveState reports the storage connection.
func (s *Service) DriveState(ctx context.Context, actorID uint) (*DriveStatus, error) {
	if _, err := s.driveAdmin(ctx, actorID); err != nil {
		return nil, err
	}
	st := &DriveStatus{Configured: s.drive.Configured(), Connected: s.drive.Connected(ctx), Account: s.drive.Account(ctx), Folder: s.drive.cfg.FolderName}
	if st.Connected {
		if q, err := s.drive.About(ctx); err == nil {
			st.Limit = q.Limit
			st.Usage = q.Usage
		} else {
			st.Error = err.Error()
		}
	}
	return st, nil
}

// DriveConnectURL starts the OAuth dance for an administrator.
func (s *Service) DriveConnectURL(ctx context.Context, actorID uint) (string, error) {
	if _, err := s.driveAdmin(ctx, actorID); err != nil {
		return "", err
	}
	if !s.drive.Configured() {
		return "", errs.Invalid("config.yml içinde drive.clientId, drive.clientSecret ve drive.redirectUrl tanımlı olmalı.", nil)
	}
	return s.drive.AuthURL(s.drive.State(actorID)), nil
}

// DriveCallback finishes the OAuth dance.
func (s *Service) DriveCallback(ctx context.Context, actorID uint, state, code string) (string, error) {
	if _, err := s.driveAdmin(ctx, actorID); err != nil {
		return "", err
	}
	uid, err := s.drive.VerifyState(state)
	if err != nil || uid != actorID {
		return "", errs.Invalid("Bağlantı isteği doğrulanamadı, yeniden deneyin.", nil)
	}
	email, err := s.drive.Exchange(ctx, code)
	if err != nil {
		return "", errs.Invalid("Google bağlantısı tamamlanamadı: "+err.Error(), nil)
	}
	// A new account means new folders: the old ids point into the old Drive.
	_ = s.repo.ClearDriveFolders(ctx)
	// Warm the root so the first upload does not pay for it.
	if _, err := s.drive.Folder(ctx); err != nil {
		return "", errs.Invalid("Drive klasörü oluşturulamadı: "+err.Error(), nil)
	}
	return email, nil
}

// DriveDisconnect forgets the account.
func (s *Service) DriveDisconnect(ctx context.Context, actorID uint) error {
	if _, err := s.driveAdmin(ctx, actorID); err != nil {
		return err
	}
	if err := s.drive.Disconnect(ctx); err != nil {
		return errs.Internal(err)
	}
	return nil
}
