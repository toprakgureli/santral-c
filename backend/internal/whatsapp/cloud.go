// Package whatsapp connects WhatsApp Business numbers to the panel: Meta
// tells us about every message through the webhook, agents answer from the
// inbox, tickets are distributed, templates, quick replies, automatic
// messages and chatbots are configured per device from the panel.
package whatsapp

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
)

const graphBase = "https://graph.facebook.com"

// defaultGraphVersion is used only when a device has no version entered.
const defaultGraphVersion = "v23.0"

// Cloud is a client for one device's Graph API calls. It is built per call
// from the device's decrypted credentials.
type Cloud struct {
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

func (c *Cloud) base() string {
	v := c.Version
	if v == "" {
		v = defaultGraphVersion
	}
	return graphBase + "/" + v
}

func (c *Cloud) client() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 30 * time.Second}
}

func (c *Cloud) do(ctx context.Context, method, endpoint string, body io.Reader, contentType string, out any) error {
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
		return fmt.Errorf("Meta'ya ulaşılamadı: %w", err)
	}
	defer resp.Body.Close()
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
			return fmt.Errorf("Meta yanıtı okunamadı: %w", err)
		}
	}
	return nil
}

func (c *Cloud) postJSON(ctx context.Context, endpoint string, payload, out any) error {
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
func (c *Cloud) Number(ctx context.Context) (*NumberInfo, error) {
	var out NumberInfo
	q := url.Values{"fields": {"display_phone_number,verified_name,quality_rating,messaging_limit_tier,name_status"}}
	if err := c.do(ctx, http.MethodGet, c.base()+"/"+url.PathEscape(c.PhoneNumberID)+"?"+q.Encode(), nil, "", &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Subscribed reports whether our app receives this account's webhooks.
func (c *Cloud) Subscribed(ctx context.Context) (bool, error) {
	var out struct {
		Data []json.RawMessage `json:"data"`
	}
	if err := c.do(ctx, http.MethodGet, c.base()+"/"+url.PathEscape(c.WABAID)+"/subscribed_apps", nil, "", &out); err != nil {
		return false, err
	}
	return len(out.Data) > 0, nil
}

// Subscribe asks Meta to send this account's events to the app.
func (c *Cloud) Subscribe(ctx context.Context) error {
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
func (c *Cloud) Send(ctx context.Context, to string, message map[string]any) (string, error) {
	payload := map[string]any{"messaging_product": "whatsapp", "recipient_type": "individual", "to": to}
	for k, v := range message {
		payload[k] = v
	}
	var out SendResult
	if err := c.postJSON(ctx, c.base()+"/"+url.PathEscape(c.PhoneNumberID)+"/messages", payload, &out); err != nil {
		return "", err
	}
	if len(out.Messages) == 0 || out.Messages[0].ID == "" {
		return "", errors.New("Meta mesaj kimliği döndürmedi")
	}
	return out.Messages[0].ID, nil
}

// MarkRead tells the customer their message was read (blue ticks). With
// typing set the customer also sees that we are writing.
func (c *Cloud) MarkRead(ctx context.Context, wamid string, typing bool) error {
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
func (c *Cloud) Media(ctx context.Context, id string) (*MediaInfo, error) {
	var out MediaInfo
	if err := c.do(ctx, http.MethodGet, c.base()+"/"+url.PathEscape(id), nil, "", &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Download fetches a media object's bytes.
func (c *Cloud) Download(ctx context.Context, id string, limit int64) ([]byte, *MediaInfo, error) {
	info, err := c.Media(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, info.URL, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	cl := &http.Client{Timeout: 5 * time.Minute}
	resp, err := cl.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("medya indirilemedi: %w", err)
	}
	defer resp.Body.Close()
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
func (c *Cloud) Upload(ctx context.Context, name, mime string, data []byte) (string, error) {
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
		return "", errors.New("Meta medya kimliği döndürmedi")
	}
	return out.ID, nil
}

// UploadHandle puts a file through the app's resumable upload so it can be
// used as a template header example. It needs the app id.
func (c *Cloud) UploadHandle(ctx context.Context, mime string, data []byte) (string, error) {
	if c.AppID == "" {
		return "", errors.New("Görselli şablon için cihazda uygulama kimliği (App ID) girilmeli.")
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
	defer resp.Body.Close()
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

// MetaTemplate is a template as Meta lists it.
type MetaTemplate struct {
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
func (c *Cloud) Templates(ctx context.Context) ([]MetaTemplate, error) {
	var out []MetaTemplate
	next := c.base() + "/" + url.PathEscape(c.WABAID) + "/message_templates?" + url.Values{
		"fields": {"id,name,language,category,status,rejected_reason,components,quality_score"},
		"limit":  {"100"},
	}.Encode()
	for page := 0; next != "" && page < 50; page++ {
		var resp struct {
			Data   []MetaTemplate `json:"data"`
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
func (c *Cloud) CreateTemplate(ctx context.Context, name, language, category string, components []map[string]any) (string, string, error) {
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
func (c *Cloud) DeleteTemplate(ctx context.Context, name string) error {
	return c.do(ctx, http.MethodDelete, c.base()+"/"+url.PathEscape(c.WABAID)+"/message_templates?"+url.Values{"name": {name}}.Encode(), nil, "", nil)
}
