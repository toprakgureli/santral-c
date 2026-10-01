package teams

// Google Drive storage for chat attachments.
//
// Bytes never pass through this server on the way up: the server opens a
// resumable upload session with the account's token and hands the session
// URL to the browser, which PUTs the file straight to Google in chunks.
// On the way down the server streams from Drive with its own token, so a
// file is only readable by people seated in the room.
//
// The account is connected once by an administrator through OAuth; the
// refresh token is kept encrypted in system_settings.

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/toprakgureli/santral-c/backend/configs"
	"github.com/toprakgureli/santral-c/backend/internal/telemetry"
	"github.com/toprakgureli/santral-c/backend/pkg/crypt"
	"github.com/toprakgureli/santral-c/backend/pkg/lockout"
)

const (
	driveScope     = "https://www.googleapis.com/auth/drive.file https://www.googleapis.com/auth/userinfo.email"
	driveAuthURL   = "https://accounts.google.com/o/oauth2/v2/auth"
	driveTokenURL  = "https://oauth2.googleapis.com/token"
	driveRevokeURL = "https://oauth2.googleapis.com/revoke"
	driveAPI       = "https://www.googleapis.com/drive/v3"
	driveUpload    = "https://www.googleapis.com/upload/drive/v3/files"
	driveUserInfo  = "https://www.googleapis.com/oauth2/v3/userinfo"
	folderMime     = "application/vnd.google-apps.folder"

	keyDriveToken   = "drive_token"
	keyDriveAccount = "drive_account"
	keyDriveFolder  = "drive_folder"
)

// Drive talks to Google Drive on behalf of the connected account.
type Drive struct {
	cfg  configs.Drive
	ring *crypt.Keyring // encrypts the refresh token, signs the link state
	db   *gorm.DB
	http *http.Client
	// bulk carries file bodies up and down. It has no overall limit, since
	// a large file on a slow line takes as long as it takes; the request's
	// context ends it, and a server that does not answer at all still
	// times out.
	bulk *http.Client

	mu      sync.Mutex
	access  string
	expires time.Time
}

// NewDrive builds the Drive client. It is usable even when nothing is
// configured: every call then fails with a clear message.
func NewDrive(cfg configs.Drive, ring *crypt.Keyring, db *gorm.DB) *Drive {
	if cfg.FolderName == "" {
		cfg.FolderName = "SantralC"
	}
	bulk := http.DefaultTransport.(*http.Transport).Clone()
	bulk.ResponseHeaderTimeout = 2 * time.Minute
	return &Drive{cfg: cfg, ring: ring, db: db, http: &http.Client{Timeout: 60 * time.Second, Transport: telemetry.Transport(nil)}, bulk: &http.Client{Transport: telemetry.Transport(bulk)}}
}

// DriveSealPurpose is the keyring label of the stored Drive refresh token.
const DriveSealPurpose = "drive"

// DriveTokenSetting is the system setting holding the sealed refresh token.
const DriveTokenSetting = keyDriveToken

// Configured reports whether the OAuth client is set in config.yml.
func (d *Drive) Configured() bool {
	return d.cfg.ClientID != "" && d.cfg.ClientSecret != "" && d.cfg.RedirectURL != ""
}

// ---------------------------------------------------------------- settings

func (d *Drive) setting(ctx context.Context, key string) string {
	var value string
	if err := d.db.WithContext(ctx).Raw("SELECT value FROM system_settings WHERE key = ?", key).Scan(&value).Error; err != nil {
		slog.WarnContext(ctx, "drive setting could not be read", "key", key, "error", err)
	}
	return value
}

func (d *Drive) store(ctx context.Context, key, value string) error {
	return d.db.WithContext(ctx).Exec(
		"INSERT INTO system_settings (key, value, updated_at) VALUES (?, ?, now()) "+
			"ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()", key, value).Error
}

// Connected reports whether an account is linked.
func (d *Drive) Connected(ctx context.Context) bool {
	return d.setting(ctx, keyDriveToken) != ""
}

// Account is the linked Google account's e-mail, or "".
func (d *Drive) Account(ctx context.Context) string {
	return d.setting(ctx, keyDriveAccount)
}

// ---------------------------------------------------------------- oauth

// driveStatePurpose names the key that signs the account-link state.
const driveStatePurpose = "drive-state"

// State signs the connecting user's id so the callback can trust it. Each
// state carries a random nonce and works once.
func (d *Drive) State(userID uint) string {
	nonce := make([]byte, 9)
	_, _ = rand.Read(nonce)
	payload := fmt.Sprintf("%d:%d:%s", userID, time.Now().Add(15*time.Minute).Unix(), base64.RawURLEncoding.EncodeToString(nonce))
	mac := hmac.New(sha256.New, d.ring.MACKeys(driveStatePurpose)[0])
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// VerifyState returns the user id a state was issued for, once.
func (d *Drive) VerifyState(ctx context.Context, state string) (uint, error) {
	parts := strings.SplitN(state, ".", 2)
	if len(parts) != 2 {
		return 0, errors.New("state okunamadı")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return 0, errors.New("state okunamadı")
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return 0, errors.New("state okunamadı")
	}
	mac := hmac.New(sha256.New, d.ring.MACKeys(driveStatePurpose)[0])
	mac.Write(payload)
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return 0, errors.New("state imzası geçersiz")
	}
	fields := strings.SplitN(string(payload), ":", 3)
	if len(fields) != 3 {
		return 0, errors.New("state okunamadı")
	}
	uid, err1 := strconv.ParseUint(fields[0], 10, 64)
	exp, err2 := strconv.ParseInt(fields[1], 10, 64)
	if err1 != nil || err2 != nil || time.Now().Unix() > exp {
		return 0, errors.New("state süresi dolmuş")
	}
	first, err := lockout.Once(ctx, "drive-state:"+fields[2], 20*time.Minute)
	if err != nil {
		return 0, fmt.Errorf("state denetlenemedi: %w", err)
	}
	if !first {
		return 0, errors.New("bu bağlantı adımı zaten kullanıldı")
	}
	return uint(uid), nil
}

// AuthURL is where the administrator is sent to grant access.
func (d *Drive) AuthURL(state string) string {
	q := url.Values{}
	q.Set("client_id", d.cfg.ClientID)
	q.Set("redirect_uri", d.cfg.RedirectURL)
	q.Set("response_type", "code")
	q.Set("scope", driveScope)
	q.Set("access_type", "offline")
	q.Set("prompt", "consent")
	q.Set("include_granted_scopes", "true")
	q.Set("state", state)
	return driveAuthURL + "?" + q.Encode()
}

// Exchange turns the callback code into a refresh token, stores it and
// returns the account's e-mail.
func (d *Drive) Exchange(ctx context.Context, code string) (string, error) {
	form := url.Values{}
	form.Set("code", code)
	form.Set("client_id", d.cfg.ClientID)
	form.Set("client_secret", d.cfg.ClientSecret)
	form.Set("redirect_uri", d.cfg.RedirectURL)
	form.Set("grant_type", "authorization_code")
	var tok struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := d.postForm(ctx, driveTokenURL, form, &tok); err != nil {
		return "", err
	}
	if tok.RefreshToken == "" {
		return "", errors.New("Google yenileme anahtarı vermedi; hesabın uygulama izinlerinden SantralC erişimini kaldırıp yeniden bağlan") //nolint:staticcheck,revive // starts with a proper noun
	}
	email := ""
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, driveUserInfo, nil)
	req.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	if resp, err := d.http.Do(req); err == nil {
		var info struct {
			Email string `json:"email"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&info)
		_ = resp.Body.Close()
		email = info.Email
	}
	enc, err := d.ring.Seal(DriveSealPurpose, tok.RefreshToken)
	if err != nil {
		return "", fmt.Errorf("anahtar şifrelenemedi: %w", err)
	}
	if err := d.store(ctx, keyDriveToken, enc); err != nil {
		return "", err
	}
	if err := d.store(ctx, keyDriveAccount, email); err != nil {
		return "", err
	}
	_ = d.store(ctx, keyDriveFolder, "")
	d.mu.Lock()
	d.access = tok.AccessToken
	d.expires = time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second)
	d.mu.Unlock()
	return email, nil
}

// Disconnect forgets the account and revokes the token at Google.
func (d *Drive) Disconnect(ctx context.Context) error {
	if enc := d.setting(ctx, keyDriveToken); enc != "" {
		if refresh, err := d.ring.Open(DriveSealPurpose, enc); err == nil {
			form := url.Values{}
			form.Set("token", refresh)
			_ = d.postForm(ctx, driveRevokeURL, form, nil)
		}
	}
	d.mu.Lock()
	d.access = ""
	d.expires = time.Time{}
	d.mu.Unlock()
	for _, k := range []string{keyDriveToken, keyDriveAccount, keyDriveFolder} {
		if err := d.store(ctx, k, ""); err != nil {
			return err
		}
	}
	return nil
}

func (d *Drive) postForm(ctx context.Context, endpoint string, form url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := d.http.Do(req)
	if err != nil {
		return fmt.Errorf("Google'a ulaşılamadı: %w", err) //nolint:staticcheck,revive // starts with a proper noun
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("Google yanıtı %d: %s", resp.StatusCode, strings.TrimSpace(string(body))) //nolint:staticcheck,revive // starts with a proper noun
	}
	if out != nil {
		return json.Unmarshal(body, out)
	}
	return nil
}

// token returns a live access token, refreshing when needed.
func (d *Drive) token(ctx context.Context) (string, error) {
	if !d.Configured() {
		return "", errors.New("Google Drive yapılandırılmamış (config.yml drive bölümü)") //nolint:staticcheck,revive // starts with a proper noun
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.access != "" && time.Now().Before(d.expires.Add(-60*time.Second)) {
		return d.access, nil
	}
	enc := d.setting(ctx, keyDriveToken)
	if enc == "" {
		return "", errors.New("Google Drive hesabı bağlı değil") //nolint:staticcheck,revive // starts with a proper noun
	}
	refresh, err := d.ring.Open(DriveSealPurpose, enc)
	if err != nil {
		slog.ErrorContext(ctx, "stored drive token could not be decrypted", "error", err)
		return "", fmt.Errorf("anahtar çözülemedi: %w", err)
	}
	form := url.Values{}
	form.Set("client_id", d.cfg.ClientID)
	form.Set("client_secret", d.cfg.ClientSecret)
	form.Set("refresh_token", refresh)
	form.Set("grant_type", "refresh_token")
	var tok struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := d.postForm(ctx, driveTokenURL, form, &tok); err != nil {
		return "", fmt.Errorf("Drive erişimi yenilenemedi: %w", err)
	}
	d.access = tok.AccessToken
	d.expires = time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second)
	return d.access, nil
}

func (d *Drive) do(ctx context.Context, method, endpoint string, body io.Reader, headers map[string]string) (*http.Response, error) {
	return d.doWith(ctx, d.http, method, endpoint, body, headers)
}

func (d *Drive) doWith(ctx context.Context, client *http.Client, method, endpoint string, body io.Reader, headers map[string]string) (*http.Response, error) {
	tok, err := d.token(ctx)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Drive'a ulaşılamadı: %w", err) //nolint:staticcheck,revive // starts with a proper noun
	}
	return resp, nil
}

func (d *Drive) doJSON(ctx context.Context, method, endpoint string, payload any, out any) error {
	var body io.Reader
	headers := map[string]string{}
	if payload != nil {
		raw, _ := json.Marshal(payload)
		body = bytes.NewReader(raw)
		headers["Content-Type"] = "application/json"
	}
	resp, err := d.do(ctx, method, endpoint, body, headers)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("Drive yanıtı %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	if out != nil && len(data) > 0 {
		return json.Unmarshal(data, out)
	}
	return nil
}

// ---------------------------------------------------------------- folder

// EnsureFolder finds a folder by name under a parent, or creates it.
func (d *Drive) EnsureFolder(ctx context.Context, name, parent string) (string, error) {
	esc := strings.NewReplacer("\\", "\\\\", "'", "\\'").Replace(name)
	q := fmt.Sprintf("name = '%s' and mimeType = '%s' and trashed = false and '%s' in parents", esc, folderMime, parent)
	var list struct {
		Files []struct {
			ID string `json:"id"`
		} `json:"files"`
	}
	if err := d.doJSON(ctx, http.MethodGet, driveAPI+"/files?fields=files(id)&q="+url.QueryEscape(q), nil, &list); err != nil {
		return "", err
	}
	if len(list.Files) > 0 {
		return list.Files[0].ID, nil
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := d.doJSON(ctx, http.MethodPost, driveAPI+"/files?fields=id", map[string]any{"name": name, "mimeType": folderMime, "parents": []string{parent}}, &created); err != nil {
		return "", err
	}
	return created.ID, nil
}

// Folder finds or creates the app's root folder in the account's Drive.
func (d *Drive) Folder(ctx context.Context) (string, error) {
	if id := d.setting(ctx, keyDriveFolder); id != "" {
		return id, nil
	}
	id, err := d.EnsureFolder(ctx, d.cfg.FolderName, "root")
	if err != nil {
		return "", err
	}
	_ = d.store(ctx, keyDriveFolder, id)
	return id, nil
}

// Rename changes a file or folder's name.
func (d *Drive) Rename(ctx context.Context, id, name string) error {
	return d.doJSON(ctx, http.MethodPatch, driveAPI+"/files/"+url.PathEscape(id)+"?fields=id", map[string]any{"name": name}, nil)
}

// ---------------------------------------------------------------- files

// DriveFile is what the server checks after a browser upload.
type DriveFile struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	MimeType string `json:"mimeType"`
	Size     int64  `json:"size,string"`
}

// StartUpload opens a resumable session the browser will fill. The Origin
// is what Google will allow to PUT into the session.
func (d *Drive) StartUpload(ctx context.Context, folder, name, mime string, size int64, origin string) (string, error) {
	raw, _ := json.Marshal(map[string]any{"name": name, "parents": []string{folder}, "mimeType": mime})
	headers := map[string]string{
		"Content-Type":            "application/json; charset=UTF-8",
		"X-Upload-Content-Type":   mime,
		"X-Upload-Content-Length": strconv.FormatInt(size, 10),
	}
	if origin != "" {
		headers["Origin"] = origin
	}
	resp, err := d.do(ctx, http.MethodPost, driveUpload+"?uploadType=resumable&fields=id,name,mimeType,size", bytes.NewReader(raw), headers)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
		return "", fmt.Errorf("yükleme oturumu açılamadı (%d): %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	loc := resp.Header.Get("Location")
	if loc == "" {
		return "", errors.New("yükleme oturumu adresi gelmedi")
	}
	return loc, nil
}

// Put stores bytes as a new file from the server itself and returns its
// id. Used where the bytes are already here (WhatsApp media).
func (d *Drive) Put(ctx context.Context, folder, name, mime string, data []byte) (string, error) {
	session, err := d.StartUpload(ctx, folder, name, mime, int64(len(data)), "")
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, session, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", mime)
	resp, err := d.bulk.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode/100 != 2 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
		return "", fmt.Errorf("Drive yükleme yanıtı %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var f DriveFile
	if err := json.NewDecoder(resp.Body).Decode(&f); err != nil || f.ID == "" {
		return "", errors.New("Drive dosya kimliği gelmedi")
	}
	return f.ID, nil
}

// File reads one file's card.
func (d *Drive) File(ctx context.Context, id string) (*DriveFile, error) {
	var f DriveFile
	if err := d.doJSON(ctx, http.MethodGet, driveAPI+"/files/"+url.PathEscape(id)+"?fields=id,name,mimeType,size", nil, &f); err != nil {
		return nil, err
	}
	return &f, nil
}

// Delete removes a file for good (not to the bin).
func (d *Drive) Delete(ctx context.Context, id string) error {
	resp, err := d.do(ctx, http.MethodDelete, driveAPI+"/files/"+url.PathEscape(id), nil, nil)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode/100 != 2 && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("Drive silme yanıtı %d", resp.StatusCode)
	}
	return nil
}

// Open streams a file's bytes; rangeHeader is forwarded so video can seek.
// The caller must close the body.
func (d *Drive) Open(ctx context.Context, id, rangeHeader string) (*http.Response, error) {
	headers := map[string]string{}
	if rangeHeader != "" {
		headers["Range"] = rangeHeader
	}
	resp, err := d.doWith(ctx, d.bulk, http.MethodGet, driveAPI+"/files/"+url.PathEscape(id)+"?alt=media", nil, headers)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
		_ = resp.Body.Close()
		return nil, fmt.Errorf("Drive okuma yanıtı %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	return resp, nil
}

// Quota is the account's storage use.
type Quota struct {
	Limit int64 `json:"limit"`
	Usage int64 `json:"usage"`
}

// About reads the account's storage quota.
func (d *Drive) About(ctx context.Context) (*Quota, error) {
	var out struct {
		StorageQuota struct {
			Limit string `json:"limit"`
			Usage string `json:"usage"`
		} `json:"storageQuota"`
	}
	if err := d.doJSON(ctx, http.MethodGet, driveAPI+"/about?fields=storageQuota", nil, &out); err != nil {
		return nil, err
	}
	limit, _ := strconv.ParseInt(out.StorageQuota.Limit, 10, 64)
	usage, _ := strconv.ParseInt(out.StorageQuota.Usage, 10, 64)
	return &Quota{Limit: limit, Usage: usage}, nil
}
