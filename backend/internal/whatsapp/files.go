package whatsapp

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
)

// Files uploaded from the panel: an image a chatbot sends, a document for a
// template's header. The bytes are kept in the connected storage. When one
// goes to a customer it is uploaded to Meta once and the id Meta gives is
// reused for a while, since Meta keeps uploaded media for about a month.

const metaFileTTL = 20 * 24 * time.Hour

type metaFile struct {
	id string
	at time.Time
}

// FileView is an uploaded file as the panel sees it.
type FileView struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
	Mime string `json:"mime"`
	Size int64  `json:"size"`
	Kind string `json:"kind"` // image | video | audio | document
	URL  string `json:"url"`
}

func fileView(f *models.WAFile) FileView {
	kind, _ := mediaKind(f.Mime, 0)
	return FileView{ID: f.ID, Name: f.Name, Mime: f.Mime, Size: f.Size, Kind: kind, URL: fmt.Sprintf("/api/v1/wa/files/%d", f.ID)}
}

func (s *Service) uploadsFolder(ctx context.Context) (string, error) {
	s.mu.Lock()
	if id, ok := s.folders[0]; ok {
		s.mu.Unlock()
		return id, nil
	}
	s.mu.Unlock()
	root, err := s.storage.Folder(ctx)
	if err != nil {
		return "", err
	}
	wa, err := s.storage.EnsureFolder(ctx, "WhatsApp", root)
	if err != nil {
		return "", err
	}
	id, err := s.storage.EnsureFolder(ctx, "Yüklenen dosyalar", wa)
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	s.folders[0] = id
	s.mu.Unlock()
	return id, nil
}

// UploadFile keeps a file for chatbots and templates.
func (s *Service) UploadFile(ctx context.Context, actorID uint, name, mime string, data []byte) (*FileView, error) {
	u, err := s.users.GetByID(ctx, actorID)
	if err != nil {
		return nil, err
	}
	if !u.Can(enums.WABotManage) && !u.Can(enums.WATemplateSend) && !u.Can(enums.WATemplateManage) {
		return nil, errs.Forbidden("Dosya yükleme yetkiniz yok.")
	}
	if s.storage == nil || !s.storage.Connected(ctx) {
		return nil, errs.Invalid("Dosyaları saklamak için önce Sistem Ayarları'ndan Google Drive bağlanmalı.", nil)
	}
	if len(data) == 0 {
		return nil, errs.Invalid("Dosya boş.", nil)
	}
	if mime == "" || mime == "application/octet-stream" {
		mime = http.DetectContentType(data)
	}
	mime = strings.SplitN(mime, ";", 2)[0]
	kind, err := mediaKind(mime, int64(len(data)))
	if err != nil {
		return nil, err
	}
	name = filepath.Base(strings.TrimSpace(name))
	if name == "" || name == "." {
		name = kind + extFor(mime)
	}
	folder, err := s.uploadsFolder(ctx)
	if err != nil {
		return nil, errs.Invalid("Dosya klasörü açılamadı: "+err.Error(), err)
	}
	c, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	store, err := s.storage.Put(c, folder, name, mime, data)
	if err != nil {
		return nil, errs.Invalid("Dosya saklanamadı: "+err.Error(), err)
	}
	f := &models.WAFile{StorageID: store, Name: name, Mime: mime, Size: int64(len(data)), CreatedBy: uintPtr(actorID), CreatedAt: time.Now()}
	if err := s.db.WithContext(ctx).Create(f).Error; err != nil {
		return nil, errs.Internal(err)
	}
	v := fileView(f)
	return &v, nil
}

func (s *Service) file(ctx context.Context, id uint) (*models.WAFile, error) {
	var f models.WAFile
	if err := s.db.WithContext(ctx).First(&f, id).Error; err != nil {
		return nil, errs.NotFound("Dosya bulunamadı.")
	}
	return &f, nil
}

// OpenFile streams an uploaded file for the panel's previews.
func (s *Service) OpenFile(ctx context.Context, actorID, id uint, rangeHeader string) (*MediaStream, error) {
	if _, err := s.require(ctx, actorID, enums.WAView, "WhatsApp'ı görme yetkiniz yok."); err != nil {
		return nil, err
	}
	f, err := s.file(ctx, id)
	if err != nil {
		return nil, err
	}
	if s.storage == nil {
		return nil, errs.NotFound("Dosya okunamadı.")
	}
	resp, err := s.storage.Open(ctx, f.StorageID, rangeHeader)
	if err != nil {
		return nil, errs.NotFound("Dosya okunamadı.")
	}
	return &MediaStream{Body: resp.Body, Mime: f.Mime, Name: f.Name, Size: resp.ContentLength, Status: resp.StatusCode, ContentRange: resp.Header.Get("Content-Range")}, nil
}

// metaMediaFor returns Meta's id for an uploaded file on a device,
// uploading it when Meta does not have it (or may have dropped it).
func (s *Service) metaMediaFor(ctx context.Context, ch *models.WAChannel, fileID uint) (string, *models.WAFile, error) {
	f, err := s.file(ctx, fileID)
	if err != nil {
		return "", nil, err
	}
	key := fmt.Sprintf("%d:%d", ch.ID, fileID)
	s.mu.Lock()
	cached, ok := s.metaFiles[key]
	s.mu.Unlock()
	if ok && time.Since(cached.at) < metaFileTTL {
		return cached.id, f, nil
	}
	if s.storage == nil {
		return "", f, fmt.Errorf("dosya deposu bağlı değil")
	}
	resp, err := s.storage.Open(ctx, f.StorageID, "")
	if err != nil {
		return "", f, fmt.Errorf("dosya okunamadı: %w", err)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, mediaLimit))
	resp.Body.Close()
	if err != nil {
		return "", f, fmt.Errorf("dosya okunamadı: %w", err)
	}
	cl, err := s.cloudFor(ch)
	if err != nil {
		return "", f, err
	}
	c, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	id, err := cl.Upload(c, f.Name, f.Mime, data)
	if err != nil {
		return "", f, err
	}
	s.mu.Lock()
	s.metaFiles[key] = metaFile{id: id, at: time.Now()}
	s.mu.Unlock()
	return id, f, nil
}

// mediaObject is the message body for sending an uploaded file.
func mediaObject(kind, metaID, caption, name string) map[string]any {
	obj := map[string]any{"id": metaID}
	if caption = strings.TrimSpace(caption); caption != "" && kind != "audio" {
		obj["caption"] = caption
	}
	if kind == "document" {
		obj["filename"] = name
	}
	return map[string]any{"type": kind, kind: obj}
}
