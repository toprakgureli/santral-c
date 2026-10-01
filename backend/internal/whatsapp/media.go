package whatsapp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/toprakgureli/santral-c/backend/internal/whatsapp/meta"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
	"github.com/toprakgureli/santral-c/backend/pkg/safe"
)

// Meta keeps a customer's file only for a while and its link only for
// minutes, so every file is copied to our storage as soon as it arrives.

const mediaLimit = 100 << 20

func extFor(mime string) string {
	switch {
	case strings.HasPrefix(mime, "image/jpeg"):
		return ".jpg"
	case strings.HasPrefix(mime, "image/png"):
		return ".png"
	case strings.HasPrefix(mime, "image/webp"):
		return ".webp"
	case strings.HasPrefix(mime, "video/mp4"):
		return ".mp4"
	case strings.HasPrefix(mime, "video/3gpp"):
		return ".3gp"
	case strings.HasPrefix(mime, "audio/ogg"):
		return ".ogg"
	case strings.HasPrefix(mime, "audio/mpeg"):
		return ".mp3"
	case strings.HasPrefix(mime, "audio/mp4"), strings.HasPrefix(mime, "audio/aac"):
		return ".m4a"
	case strings.HasPrefix(mime, "application/pdf"):
		return ".pdf"
	}
	return ""
}

// folderFor is the device's folder in storage: SantralC/WhatsApp/<ad>.
func (s *Service) folderFor(ctx context.Context, ch *models.WAChannel) (string, error) {
	s.mu.Lock()
	if id, ok := s.folders[ch.ID]; ok {
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
	name := fmt.Sprintf("%s (#%d)", strings.TrimSpace(ch.Name), ch.ID)
	id, err := s.storage.EnsureFolder(ctx, name, wa)
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	s.folders[ch.ID] = id
	s.mu.Unlock()
	return id, nil
}

func (s *Service) mediaRef(msg *models.WAMessage) MediaRef {
	var ref MediaRef
	if msg.Media != nil {
		_ = json.Unmarshal([]byte(*msg.Media), &ref)
	}
	return ref
}

func (s *Service) saveRef(ctx context.Context, msgID uint, ref MediaRef) {
	warnDB(ctx, s.repo.SetMessageMedia(ctx, msgID, jsonString(ref)))
}

// keepMedia copies a customer's file to storage.
func (s *Service) keepMedia(ctx context.Context, ch *models.WAChannel, msgID uint) {
	msg, err := s.repo.LoadMessage(ctx, msgID)
	if err != nil {
		return
	}
	ref := s.mediaRef(msg)
	if ref.MetaID == "" || ref.StoreID != "" {
		return
	}
	cl, err := s.cloudFor(ch)
	if err != nil {
		// Marked, so the sweep moves on to newer files instead of trying
		// this one again and again.
		ref.Failed = "Cihazın erişim bilgisi okunamadığı için dosya alınamadı."
		s.saveRef(ctx, msgID, ref)
		return
	}
	var data []byte
	var info *meta.MediaInfo
	for attempt := 0; attempt < 3; attempt++ {
		c, cancel := context.WithTimeout(ctx, 5*time.Minute)
		data, info, err = cl.Download(c, ref.MetaID, mediaLimit)
		cancel()
		if err == nil {
			break
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Duration(attempt+1) * 5 * time.Second):
		}
	}
	if err != nil {
		ref.Failed = "Dosya Meta'dan alınamadı."
		s.saveRef(ctx, msgID, ref)
		slog.WarnContext(ctx, "whatsapp media download failed", "message", msgID, "error", err)
		return
	}
	ref.Size = int64(len(data))
	if ref.Mime == "" {
		ref.Mime = info.MimeType
	}
	if ref.Name == "" {
		ref.Name = fmt.Sprintf("%s-%d%s", msg.Kind, msg.ID, extFor(ref.Mime))
	}
	if s.storage != nil && s.storage.Connected(ctx) {
		folder, err := s.folderFor(ctx, ch)
		if err == nil {
			c, cancel := context.WithTimeout(ctx, 5*time.Minute)
			id, err := s.storage.Put(c, folder, ref.Name, ref.Mime, data)
			cancel()
			if err == nil {
				ref.StoreID = id
			} else {
				slog.WarnContext(ctx, "whatsapp media could not be stored", "message", msgID, "error", err)
			}
		}
	}
	s.saveRef(ctx, msgID, ref)
	warnDB(ctx, s.repo.ReloadMessage(ctx, msg))
	s.publish(ctx, msg.ConversationID, msg, nil)
}

// MediaStream is a file ready to be written to the browser.
type MediaStream struct {
	Body         io.ReadCloser
	Mime         string
	Name         string
	Size         int64
	Status       int
	ContentRange string
}

// OpenMedia streams a message's file to someone who sees the chat.
func (s *Service) OpenMedia(ctx context.Context, actorID, messageID uint, rangeHeader string) (*MediaStream, error) {
	msg, err := s.repo.LoadMessage(ctx, messageID)
	if err != nil {
		return nil, errs.NotFound("Dosya bulunamadı.")
	}
	if _, _, _, err := s.reachable(ctx, actorID, msg.ConversationID); err != nil {
		return nil, err
	}
	ref := s.mediaRef(msg)
	if ref.StoreID != "" && s.storage != nil {
		resp, err := s.storage.Open(ctx, ref.StoreID, rangeHeader)
		if err == nil {
			return &MediaStream{Body: resp.Body, Mime: ref.Mime, Name: ref.Name, Size: resp.ContentLength, Status: resp.StatusCode, ContentRange: resp.Header.Get("Content-Range")}, nil
		}
		slog.WarnContext(ctx, "whatsapp media could not be read from storage", "message", messageID, "error", err)
	}
	if ref.MetaID == "" {
		return nil, errs.NotFound("Dosya artık yok.")
	}
	ch, err := s.repo.Channel(ctx, msg.ChannelID)
	if err != nil {
		return nil, err
	}
	cl, err := s.cloudFor(ch)
	if err != nil {
		return nil, errs.Invalid(err.Error(), nil)
	}
	data, info, err := cl.Download(ctx, ref.MetaID, mediaLimit)
	if err != nil {
		return nil, errs.NotFound("Dosya Meta'dan alınamadı. Meta dosyaları bir süre sonra siler.")
	}
	mime := ref.Mime
	if mime == "" {
		mime = info.MimeType
	}
	return &MediaStream{Body: io.NopCloser(bytes.NewReader(data)), Mime: mime, Name: ref.Name, Size: int64(len(data)), Status: http.StatusOK}, nil
}

// mediaKind decides how a file is sent and checks WhatsApp's limits.
func mediaKind(mime string, size int64) (string, error) {
	mime = strings.ToLower(strings.TrimSpace(strings.SplitN(mime, ";", 2)[0]))
	switch mime {
	case "image/jpeg", "image/png":
		if size > 5<<20 {
			return "", errs.Invalid("Görsel en fazla 5 MB olabilir. Belge olarak göndermeyi dene.", nil)
		}
		return "image", nil
	case "video/mp4", "video/3gpp":
		if size > 16<<20 {
			return "", errs.Invalid("Video en fazla 16 MB olabilir.", nil)
		}
		return "video", nil
	case "audio/ogg", "audio/mpeg", "audio/mp4", "audio/aac", "audio/amr":
		if size > 16<<20 {
			return "", errs.Invalid("Ses dosyası en fazla 16 MB olabilir.", nil)
		}
		return "audio", nil
	}
	if !documentTypes[mime] {
		return "", errs.Invalid("Bu dosya türü gönderilemez. PDF, Word, Excel, PowerPoint ya da düz metin gönderebilirsin.", nil)
	}
	if size > 100<<20 {
		return "", errs.Invalid("Dosya en fazla 100 MB olabilir.", nil)
	}
	return "document", nil
}

// storedKind is the kind of a file already stored; one uploaded before the
// type list existed still counts as a document.
func storedKind(mime string) string {
	if k, err := mediaKind(mime, 0); err == nil {
		return k
	}
	return "document"
}

// documentTypes are the document types WhatsApp delivers. Anything else
// (a web page, a picture format it does not show) is refused at upload.
var documentTypes = map[string]bool{
	"application/pdf":    true,
	"text/plain":         true,
	"application/msword": true,
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document": true,
	"application/vnd.ms-excel": true,
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":         true,
	"application/vnd.ms-powerpoint":                                             true,
	"application/vnd.openxmlformats-officedocument.presentationml.presentation": true,
}

// SendMedia uploads a file to Meta and queues it for the customer.
func (s *Service) SendMedia(ctx context.Context, actorID, conversationID uint, name, mime, caption, clientID string, replyTo uint, data []byte) (*MessageView, error) {
	v, conv, ticket, err := s.reachable(ctx, actorID, conversationID)
	if err != nil {
		return nil, err
	}
	if !v.can(enums.WAReply) {
		return nil, errs.Forbidden("Müşteriye yazma yetkin yok.")
	}
	if !windowOpen(conv) {
		return nil, errs.Invalid("Müşterinin son mesajının üzerinden 24 saat geçti. Dosya gönderilemez, önce şablonla yaz.", nil)
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
	ch, err := s.repo.Channel(ctx, conv.ChannelID)
	if err != nil {
		return nil, err
	}
	cl, err := s.cloudFor(ch)
	if err != nil {
		return nil, errs.Invalid(err.Error(), nil)
	}
	name = filepath.Base(strings.TrimSpace(name))
	if name == "" || name == "." {
		name = kind + extFor(mime)
	}
	c, cancel := context.WithTimeout(ctx, 3*time.Minute)
	metaID, err := cl.Upload(c, name, mime, data)
	cancel()
	if err != nil {
		return nil, errs.Invalid("Dosya Meta'ya yüklenemedi. "+meta.Friendly(err), err)
	}
	obj := map[string]any{"id": metaID}
	caption = strings.TrimSpace(caption)
	if caption != "" && kind != "audio" {
		obj["caption"] = caption
	}
	if kind == "document" {
		obj["filename"] = name
	}
	ref := MediaRef{MetaID: metaID, Mime: mime, Name: name, Size: int64(len(data))}
	msg := &models.WAMessage{ChannelID: ch.ID, ConversationID: conv.ID, TicketID: uintPtr(ticket.ID), Direction: "out", Kind: kind,
		SenderKind: "agent", SenderUserID: uintPtr(actorID), Body: caption, Media: strPtr(jsonString(ref)), Status: "queued", CreatedAt: time.Now()}
	if cid := strings.TrimSpace(clientID); cid != "" {
		msg.ClientID = strPtr(cid)
	}
	if replyTo > 0 {
		if target, err := s.repo.MessageInConversation(ctx, replyTo, conv.ID); err == nil && target.WAMID != nil {
			msg.ReplyToWAMID = target.WAMID
		}
	}
	view, err := s.enqueue(ctx, ch, conv, ticket, msg, map[string]any{"type": kind, kind: obj}, actorID)
	if err != nil {
		return nil, err
	}
	// Keep our own copy too; Meta's goes away.
	if s.storage != nil && s.storage.Connected(ctx) {
		id := msg.ID
		safe.Go(ctx, "whatsapp media copy", func() {
			bg, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Minute)
			defer cancel()
			folder, err := s.folderFor(bg, ch)
			if err != nil {
				return
			}
			store, err := s.storage.Put(bg, folder, name, mime, data)
			if err != nil {
				return
			}
			ref.StoreID = store
			s.saveRef(bg, id, ref)
		})
	}
	return view, nil
}
