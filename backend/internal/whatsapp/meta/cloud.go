// Package meta talks to Meta's WhatsApp Cloud API for one device: sending
// messages, files and templates, reading the number's state, and putting
// Meta's error codes into words.
package meta

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/toprakgureli/santral-c/backend/internal/telemetry"
)

const graphBase = "https://graph.facebook.com"

// DefaultGraphVersion is used only when a device has no version entered.
const DefaultGraphVersion = "v23.0"

// Client is a client for one device's Graph API calls. It is built per call
// from the device's decrypted credentials.
type Client struct {
	PhoneNumberID string
	WABAID        string
	AppID         string
	Token         string
	Version       string
	HTTP          *http.Client
}

// APIError is an error Meta returned, with its code for the panel.
type APIError struct {
	Status  int
	Code    int
	Subcode int
	Message string
	Details string
}

func (e *APIError) Error() string {
	msg := e.Message
	if e.Details != "" {
		msg += ": " + e.Details
	}
	return fmt.Sprintf("Meta yanıtı %d (kod %d): %s", e.Status, e.Code, msg)
}

func (c *Client) base() string {
	v := c.Version
	if v == "" {
		v = DefaultGraphVersion
	}
	return graphBase + "/" + v
}

func (c *Client) client() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 30 * time.Second, Transport: telemetry.Transport(nil)}
}

func (c *Client) do(ctx context.Context, method, endpoint string, body io.Reader, contentType string, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := c.client().Do(req)
	if err != nil {
		return fmt.Errorf("Meta'ya ulaşılamadı: %w", err) //nolint:staticcheck,revive // starts with a proper noun
	}
	defer func() { _ = resp.Body.Close() }()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode/100 != 2 {
		var wrap struct {
			Error struct {
				Message      string `json:"message"`
				Code         int    `json:"code"`
				Subcode      int    `json:"error_subcode"`
				ErrorUserMsg string `json:"error_user_msg"`
				ErrorData    struct {
					Details string `json:"details"`
				} `json:"error_data"`
			} `json:"error"`
		}
		_ = json.Unmarshal(data, &wrap)
		e := &APIError{Status: resp.StatusCode, Code: wrap.Error.Code, Subcode: wrap.Error.Subcode, Message: wrap.Error.Message, Details: wrap.Error.ErrorData.Details}
		if e.Details == "" {
			e.Details = wrap.Error.ErrorUserMsg
		}
		if e.Message == "" {
			e.Message = strings.TrimSpace(string(data))
		}
		return e
	}
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("Meta yanıtı okunamadı: %w", err) //nolint:staticcheck,revive // starts with a proper noun
		}
	}
	return nil
}

func (c *Client) postJSON(ctx context.Context, endpoint string, payload, out any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return c.do(ctx, http.MethodPost, endpoint, bytes.NewReader(raw), "application/json", out)
}

// ---------------------------------------------------------------- number

// NumberInfo is what Meta says about the connected number.
type NumberInfo struct {
	ID                 string `json:"id"`
	DisplayPhoneNumber string `json:"display_phone_number"`
	VerifiedName       string `json:"verified_name"`
	QualityRating      string `json:"quality_rating"`
	MessagingLimitTier string `json:"messaging_limit_tier"`
	NameStatus         string `json:"name_status"`
}

// Number reads the number's card; it doubles as the credentials check.
func (c *Client) Number(ctx context.Context) (*NumberInfo, error) {
	var out NumberInfo
	q := url.Values{"fields": {"display_phone_number,verified_name,quality_rating,messaging_limit_tier,name_status"}}
	if err := c.do(ctx, http.MethodGet, c.base()+"/"+url.PathEscape(c.PhoneNumberID)+"?"+q.Encode(), nil, "", &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Subscribed reports whether our app receives this account's webhooks.
func (c *Client) Subscribed(ctx context.Context) (bool, error) {
	var out struct {
		Data []json.RawMessage `json:"data"`
	}
	if err := c.do(ctx, http.MethodGet, c.base()+"/"+url.PathEscape(c.WABAID)+"/subscribed_apps", nil, "", &out); err != nil {
		return false, err
	}
	return len(out.Data) > 0, nil
}

// Subscribe asks Meta to send this account's events to the app.
func (c *Client) Subscribe(ctx context.Context) error {
	return c.do(ctx, http.MethodPost, c.base()+"/"+url.PathEscape(c.WABAID)+"/subscribed_apps", nil, "", nil)
}

// ---------------------------------------------------------------- sending

// SendResult is Meta's answer to a send.
type SendResult struct {
	Messages []struct {
		ID string `json:"id"`
	} `json:"messages"`
}

// Send posts one message object (the part after messaging_product and to)
// and returns the message id Meta gave it.
func (c *Client) Send(ctx context.Context, to string, message map[string]any) (string, error) {
	payload := map[string]any{"messaging_product": "whatsapp", "recipient_type": "individual", "to": to}
	for k, v := range message {
		payload[k] = v
	}
	var out SendResult
	if err := c.postJSON(ctx, c.base()+"/"+url.PathEscape(c.PhoneNumberID)+"/messages", payload, &out); err != nil {
		return "", err
	}
	if len(out.Messages) == 0 || out.Messages[0].ID == "" {
		return "", errors.New("Meta mesaj kimliği döndürmedi") //nolint:staticcheck,revive // starts with a proper noun
	}
	return out.Messages[0].ID, nil
}

// MarkRead tells the customer their message was read (blue ticks). With
// typing set the customer also sees that we are writing.
func (c *Client) MarkRead(ctx context.Context, wamid string, typing bool) error {
	payload := map[string]any{"messaging_product": "whatsapp", "status": "read", "message_id": wamid}
	if typing {
		payload["typing_indicator"] = map[string]any{"type": "text"}
	}
	return c.postJSON(ctx, c.base()+"/"+url.PathEscape(c.PhoneNumberID)+"/messages", payload, nil)
}

// ---------------------------------------------------------------- media

// MediaInfo is a media object's card.
type MediaInfo struct {
	URL      string `json:"url"`
	MimeType string `json:"mime_type"`
	SHA256   string `json:"sha256"`
	FileSize int64  `json:"file_size"`
}

// Media reads a media object's card; its URL lives a few minutes.
func (c *Client) Media(ctx context.Context, id string) (*MediaInfo, error) {
	var out MediaInfo
	if err := c.do(ctx, http.MethodGet, c.base()+"/"+url.PathEscape(id), nil, "", &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Download fetches a media object's bytes.
func (c *Client) Download(ctx context.Context, id string, limit int64) ([]byte, *MediaInfo, error) {
	info, err := c.Media(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, info.URL, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	cl := &http.Client{Timeout: 5 * time.Minute, Transport: telemetry.Transport(nil)}
	resp, err := cl.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("medya indirilemedi: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode/100 != 2 {
		return nil, nil, fmt.Errorf("medya indirme yanıtı %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, nil, fmt.Errorf("medya okunamadı: %w", err)
	}
	if int64(len(data)) > limit {
		return nil, nil, errors.New("medya sınırdan büyük")
	}
	return data, info, nil
}

// Upload puts a file into Meta's media store and returns its id.
func (c *Client) Upload(ctx context.Context, name, mime string, data []byte) (string, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("messaging_product", "whatsapp")
	_ = w.WriteField("type", mime)
	h := make(map[string][]string)
	h["Content-Disposition"] = []string{fmt.Sprintf(`form-data; name="file"; filename=%q`, name)}
	h["Content-Type"] = []string{mime}
	part, err := w.CreatePart(h)
	if err != nil {
		return "", err
	}
	if _, err := part.Write(data); err != nil {
		return "", err
	}
	if err := w.Close(); err != nil {
		return "", err
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := c.do(ctx, http.MethodPost, c.base()+"/"+url.PathEscape(c.PhoneNumberID)+"/media", &buf, w.FormDataContentType(), &out); err != nil {
		return "", err
	}
	if out.ID == "" {
		return "", errors.New("Meta medya kimliği döndürmedi") //nolint:staticcheck,revive // starts with a proper noun
	}
	return out.ID, nil
}

// UploadHandle puts a file through the app's resumable upload so it can be
// used as a template header example. It needs the app id.
func (c *Client) UploadHandle(ctx context.Context, mime string, data []byte) (string, error) {
	if c.AppID == "" {
		return "", errors.New("Görselli şablon için cihazda uygulama kimliği (App ID) girilmeli.") //nolint:staticcheck,revive // a sentence shown to people as it is
	}
	q := url.Values{"file_length": {strconv.Itoa(len(data))}, "file_type": {mime}}
	var session struct {
		ID string `json:"id"`
	}
	if err := c.do(ctx, http.MethodPost, c.base()+"/"+url.PathEscape(c.AppID)+"/uploads?"+q.Encode(), nil, "", &session); err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base()+"/"+session.ID, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "OAuth "+c.Token)
	req.Header.Set("file_offset", "0")
	resp, err := c.client().Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	var out struct {
		H string `json:"h"`
	}
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		return "", fmt.Errorf("şablon görseli yüklenemedi (%d): %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	if err := json.Unmarshal(raw, &out); err != nil || out.H == "" {
		return "", errors.New("şablon görseli yükleme yanıtı okunamadı")
	}
	return out.H, nil
}

// ---------------------------------------------------------------- templates

// Template is a template as Meta lists it.
type Template struct {
	ID             string          `json:"id"`
	Name           string          `json:"name"`
	Language       string          `json:"language"`
	Category       string          `json:"category"`
	Status         string          `json:"status"`
	RejectedReason string          `json:"rejected_reason"`
	Components     json.RawMessage `json:"components"`
	QualityScore   struct {
		Score string `json:"score"`
	} `json:"quality_score"`
}

// Templates lists every template of the business account.
func (c *Client) Templates(ctx context.Context) ([]Template, error) {
	var out []Template
	next := c.base() + "/" + url.PathEscape(c.WABAID) + "/message_templates?" + url.Values{
		"fields": {"id,name,language,category,status,rejected_reason,components,quality_score"},
		"limit":  {"100"},
	}.Encode()
	for page := 0; next != "" && page < 50; page++ {
		var resp struct {
			Data   []Template `json:"data"`
			Paging struct {
				Next string `json:"next"`
			} `json:"paging"`
		}
		if err := c.do(ctx, http.MethodGet, next, nil, "", &resp); err != nil {
			return nil, err
		}
		out = append(out, resp.Data...)
		next = resp.Paging.Next
	}
	return out, nil
}

// CreateTemplate submits a template for approval and returns its id and
// first status.
func (c *Client) CreateTemplate(ctx context.Context, name, language, category string, components []map[string]any) (string, string, error) {
	var out struct {
		ID       string `json:"id"`
		Status   string `json:"status"`
		Category string `json:"category"`
	}
	payload := map[string]any{"name": name, "language": language, "category": category, "components": components}
	if err := c.postJSON(ctx, c.base()+"/"+url.PathEscape(c.WABAID)+"/message_templates", payload, &out); err != nil {
		return "", "", err
	}
	return out.ID, out.Status, nil
}

// DeleteTemplate removes a template (all its languages) by name.
func (c *Client) DeleteTemplate(ctx context.Context, name string) error {
	return c.do(ctx, http.MethodDelete, c.base()+"/"+url.PathEscape(c.WABAID)+"/message_templates?"+url.Values{"name": {name}}.Encode(), nil, "", nil)
}

// Describe puts Meta's error codes into words an agent understands.
func Describe(code int, fallback string) string {
	switch code {
	case 190:
		return "Cihazın erişim anahtarı geçersiz ya da süresi dolmuş. Yöneticinin cihaz ayarlarından yenilemesi gerekiyor."
	case 10, 200, 3:
		return "Erişim anahtarının bu işlem için izni yok."
	case 131047:
		return "Müşterinin son mesajının üzerinden 24 saat geçti. Artık yalnızca şablonla yazılabilir."
	case 131026:
		return "Mesaj müşteriye ulaşmadı. Numara WhatsApp kullanmıyor olabilir ya da uygulaması çok eski olabilir."
	case 131049:
		return "Meta bu pazarlama mesajını müşteriye iletmedi. Kısa süre içinde çok sayıda pazarlama mesajı almış olabilir."
	case 131050:
		return "Müşteri pazarlama mesajlarını kapatmış."
	case 131051:
		return "Bu mesaj türü desteklenmiyor."
	case 131052:
		return "Müşterinin gönderdiği dosya indirilemedi."
	case 131053:
		return "Dosya Meta'ya yüklenemedi. Biçimi ya da boyutu uygun olmayabilir."
	case 132000:
		return "Şablondaki değişken sayısı ile girilen değerler tutmuyor."
	case 132001:
		return "Şablon bulunamadı ya da bu dilde onaylı değil."
	case 132005:
		return "Şablon değişkenleri çok uzun."
	case 132007:
		return "Şablon metni Meta kurallarına uymuyor."
	case 132012:
		return "Şablon değişkenlerinin biçimi yanlış."
	case 132015:
		return "Şablon düşük kalite nedeniyle Meta tarafından duraklatıldı."
	case 132016:
		return "Şablon Meta tarafından kapatıldı."
	case 130429:
		return "Gönderim hızı sınırına takıldı. Biraz sonra tekrar denenecek."
	case 131056:
		return "Bu müşteriye çok kısa sürede çok fazla mesaj gönderildi. Biraz bekleyip tekrar dene."
	case 131048:
		return "Meta, numaranın gönderimlerini geçici olarak kısıtladı (spam şüphesi)."
	case 131042:
		return "Meta işletme hesabında ödeme sorunu var."
	case 131031, 368:
		return "Meta bu hesabı kurallar nedeniyle kısıtlamış."
	case 133010:
		return "Numara WhatsApp Business'a kayıtlı değil."
	case 131021:
		return "Gönderen ve alıcı aynı numara olamaz."
	case 100:
		if f := strings.TrimSpace(fallback); f != "" {
			return "Meta isteği anlamadı: " + f
		}
		return "Meta isteği anlamadı."
	}
	if f := strings.TrimSpace(fallback); f != "" {
		return f
	}
	return "Meta bir hata bildirdi."
}

// Friendly turns any error from Meta into a sentence.
func Friendly(err error) string {
	var api *APIError
	if errors.As(err, &api) {
		return Describe(api.Code, api.Message+" "+api.Details)
	}
	return "Meta'ya ulaşılamadı, internet bağlantısı ya da Meta tarafında geçici bir sorun olabilir."
}

// Retryable reports whether a failed call may succeed when tried again.
func Retryable(err error) bool {
	var api *APIError
	if !errors.As(err, &api) {
		return true
	}
	if api.Status >= 500 {
		return true
	}
	switch api.Code {
	case 1, 2, 4, 80007, 130429, 131000, 131016, 133004, 131056:
		return true
	}
	return false
}
