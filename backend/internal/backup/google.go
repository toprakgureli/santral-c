package backup

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// The Google addresses. Tests point them at a stand-in. The token address
// is always Google's own, whatever the pasted key file says: a key file
// naming another address would send the signed request there.
var (
	tokenURL       = "https://oauth2.googleapis.com/token"
	driveFilesURL  = "https://www.googleapis.com/drive/v3/files"
	driveUploadURL = "https://www.googleapis.com/upload/drive/v3/files"
)

// driveScope lets the service account work in the folders shared with it.
// What it may do there is decided by its role in the Shared Drive: as a
// Contributor it can add files and never delete them.
const driveScope = "https://www.googleapis.com/auth/drive"

// lockReason is the note Drive shows on a locked backup file.
const lockReason = "santral-c yedeği: değiştirilemez. Kilidi yalnızca Ortak Drive yöneticisi kaldırabilir."

// serviceAccount is the part of a Google service account key file we use.
type serviceAccount struct {
	Type        string `json:"type"`
	ClientEmail string `json:"client_email"`
	PrivateKey  string `json:"private_key"`
}

// parseServiceAccount reads a key file and checks it is one.
func parseServiceAccount(raw string) (*serviceAccount, error) {
	var sa serviceAccount
	if err := json.Unmarshal([]byte(raw), &sa); err != nil {
		return nil, errors.New("anahtar dosyası okunamadı; Google Cloud'dan indirilen JSON dosyasının içeriğini yapıştır")
	}
	if sa.Type != "service_account" || sa.ClientEmail == "" || sa.PrivateKey == "" {
		return nil, errors.New("bu bir servis hesabı anahtarı değil (type, client_email ve private_key alanları olmalı)")
	}
	if _, err := jwt.ParseRSAPrivateKeyFromPEM([]byte(sa.PrivateKey)); err != nil {
		return nil, errors.New("anahtar dosyasındaki özel anahtar okunamadı")
	}
	return &sa, nil
}

// google talks to Drive as a service account.
type google struct {
	sa   *serviceAccount
	http *http.Client
}

// token gets an access token for the service account.
func (g *google) token(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	key, err := jwt.ParseRSAPrivateKeyFromPEM([]byte(g.sa.PrivateKey))
	if err != nil {
		return "", fmt.Errorf("private key: %w", err)
	}
	now := time.Now()
	assertion, err := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss":   g.sa.ClientEmail,
		"scope": driveScope,
		"aud":   tokenURL,
		"iat":   now.Unix(),
		"exp":   now.Add(time.Hour).Unix(),
	}).SignedString(key)
	if err != nil {
		return "", fmt.Errorf("token request could not be signed: %w", err)
	}
	form := url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"}, "assertion": {assertion}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	var out struct {
		AccessToken string `json:"access_token"`
	}
	if err := g.doJSON(req, &out); err != nil {
		return "", fmt.Errorf("Google girişi yapılamadı: %w", err) //nolint:staticcheck,revive // starts with a proper noun
	}
	if out.AccessToken == "" {
		return "", errors.New("Google giriş anahtarı vermedi") //nolint:staticcheck,revive // starts with a proper noun
	}
	return out.AccessToken, nil
}

// FolderCheck is what the service account may do in the backup folder.
// CanLock is set only by the panel's check, which tries a lock on a small
// file of its own.
type FolderCheck struct {
	Name        string `json:"name"`
	SharedDrive bool   `json:"sharedDrive"`
	CanAdd      bool   `json:"canAdd"`
	CanDelete   bool   `json:"canDelete"`
	CanLock     *bool  `json:"canLock,omitempty"`
	LockError   string `json:"lockError,omitempty"`
}

// Problem says why backups cannot go to this folder, or "".
func (f FolderCheck) Problem() string {
	switch {
	case !f.SharedDrive:
		return "Klasör bir Ortak Drive'da değil. Silme yetkisini Google'ın kendisinin engellemesi için yedekler bir Ortak Drive'a gitmeli."
	case !f.CanAdd:
		return "Servis hesabı bu klasöre dosya ekleyemiyor. Ortak Drive'a \"Katkıda bulunan\" olarak ekle."
	case f.CanDelete:
		return "Servis hesabı bu klasördeki dosyaları silebiliyor. Güvenlik için yedek alınmadı: Ortak Drive'da rolünü \"Katkıda bulunan\" yap."
	case f.CanLock != nil && !*f.CanLock:
		return "Servis hesabı yedek dosyasını kilitleyemiyor, yani yedekler sonradan değiştirilebilir. Bu yüzden yedek alınmaz. (" + f.LockError + ")"
	}
	return ""
}

// checkFolder asks Drive what the service account may do in folder.
func (g *google) checkFolder(ctx context.Context, token, folder string) (*FolderCheck, error) {
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	q := url.Values{
		"fields":            {"id,name,mimeType,driveId,capabilities(canAddChildren,canDeleteChildren,canTrashChildren,canRemoveChildren)"},
		"supportsAllDrives": {"true"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, driveFilesURL+"/"+url.PathEscape(folder)+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	var f struct {
		Name         string `json:"name"`
		MimeType     string `json:"mimeType"`
		DriveID      string `json:"driveId"`
		Capabilities struct {
			CanAddChildren    bool `json:"canAddChildren"`
			CanDeleteChildren bool `json:"canDeleteChildren"`
			CanTrashChildren  bool `json:"canTrashChildren"`
			CanRemoveChildren bool `json:"canRemoveChildren"`
		} `json:"capabilities"`
	}
	if err := g.doJSON(req, &f); err != nil {
		return nil, fmt.Errorf("klasöre ulaşılamadı (kimliği ve paylaşımı kontrol et): %w", err)
	}
	if f.MimeType != "application/vnd.google-apps.folder" {
		return nil, errors.New("bu kimlik bir klasöre ait değil")
	}
	c := f.Capabilities
	return &FolderCheck{
		Name:        f.Name,
		SharedDrive: f.DriveID != "",
		CanAdd:      c.CanAddChildren,
		CanDelete:   c.CanDeleteChildren || c.CanTrashChildren || c.CanRemoveChildren,
	}, nil
}

// upload sends size bytes from body into folder as a new file and returns
// its Drive id. Nothing here deletes, renames or replaces: a backup only
// ever adds a new file.
func (g *google) upload(ctx context.Context, token, folder, name string, body io.Reader, size int64) (string, error) {
	meta, _ := json.Marshal(map[string]any{"name": name, "parents": []string{folder}, "mimeType": "application/octet-stream"})
	start, err := http.NewRequestWithContext(ctx, http.MethodPost, driveUploadURL+"?uploadType=resumable&supportsAllDrives=true&fields=id,size", bytes.NewReader(meta))
	if err != nil {
		return "", err
	}
	start.Header.Set("Authorization", "Bearer "+token)
	start.Header.Set("Content-Type", "application/json; charset=UTF-8")
	start.Header.Set("X-Upload-Content-Type", "application/octet-stream")
	start.Header.Set("X-Upload-Content-Length", fmt.Sprint(size))
	res, err := g.http.Do(start)
	if err != nil {
		return "", fmt.Errorf("Drive'a ulaşılamadı: %w", err) //nolint:staticcheck,revive // starts with a proper noun
	}
	_ = res.Body.Close()
	session := res.Header.Get("Location")
	if res.StatusCode != http.StatusOK || session == "" {
		return "", fmt.Errorf("yükleme başlatılamadı (Drive %d)", res.StatusCode)
	}
	put, err := http.NewRequestWithContext(ctx, http.MethodPut, session, body)
	if err != nil {
		return "", err
	}
	put.ContentLength = size
	put.Header.Set("Content-Type", "application/octet-stream")
	var out struct {
		ID string `json:"id"`
	}
	if err := g.doJSON(put, &out); err != nil {
		return "", fmt.Errorf("yükleme tamamlanamadı: %w", err)
	}
	if out.ID == "" {
		return "", errors.New("Drive dosya kimliği vermedi") //nolint:staticcheck,revive // starts with a proper noun
	}
	return out.ID, nil
}

// lock makes a file read-only with a restriction only a Shared Drive
// organizer can lift, then reads the file back to see that the service
// account itself can no longer change its content. Uploading over a backup
// or removing its older revisions both need that right; a Contributor
// otherwise has it on the files it added.
func (g *google) lock(ctx context.Context, token, fileID string) error {
	body, _ := json.Marshal(map[string]any{
		"contentRestrictions": []map[string]any{{"readOnly": true, "ownerRestricted": true, "reason": lockReason}},
	})
	q := url.Values{"supportsAllDrives": {"true"}, "fields": {"id"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, driveFilesURL+"/"+url.PathEscape(fileID)+"?"+q.Encode(), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")
	if err := g.doJSON(req, nil); err != nil {
		return fmt.Errorf("kilit konamadı: %w", err)
	}

	q = url.Values{"supportsAllDrives": {"true"}, "fields": {"capabilities(canModifyContent),contentRestrictions(readOnly)"}}
	req, err = http.NewRequestWithContext(ctx, http.MethodGet, driveFilesURL+"/"+url.PathEscape(fileID)+"?"+q.Encode(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	var f struct {
		Capabilities struct {
			CanModifyContent *bool `json:"canModifyContent"`
		} `json:"capabilities"`
		ContentRestrictions []struct {
			ReadOnly bool `json:"readOnly"`
		} `json:"contentRestrictions"`
	}
	if err := g.doJSON(req, &f); err != nil {
		return fmt.Errorf("kilit denetlenemedi: %w", err)
	}
	locked := false
	for _, r := range f.ContentRestrictions {
		locked = locked || r.ReadOnly
	}
	if !locked {
		return errors.New("Drive kilidi kaydetmedi") //nolint:staticcheck,revive // starts with a proper noun
	}
	if f.Capabilities.CanModifyContent == nil || *f.Capabilities.CanModifyContent {
		return errors.New("kilide rağmen servis hesabı dosyayı hâlâ değiştirebiliyor; Ortak Drive'da rolünün \"Katkıda bulunan\" olduğunu kontrol et")
	}
	return nil
}

// tryLock adds a small file of its own to the folder and locks it, to see
// that backups can be locked before one is needed. The file stays: the
// account cannot delete, and should not.
func (g *google) tryLock(ctx context.Context, token, folder string) error {
	stamp := time.Now().UTC().Format("2006-01-02-150405")
	text := []byte("santral-c kilit denemesi " + stamp + "\nBu dosya, yedeklerin kilitlenebildiğini denemek için eklendi.\n")
	id, err := g.upload(ctx, token, folder, "santral-kilit-denemesi-"+stamp+".txt", bytes.NewReader(text), int64(len(text)))
	if err != nil {
		return err
	}
	return g.lock(ctx, token, id)
}

// doJSON sends req and decodes a 2xx answer into out. A refusal is told by
// its status and Google's short error code only: the rest of the remote
// answer is never passed on to the panel or the logs.
func (g *google) doJSON(req *http.Request, out any) error {
	res, err := g.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode/100 != 2 {
		if code := remoteCode(body); code != "" {
			return fmt.Errorf("Google %d yanıtı verdi (%s)", res.StatusCode, code) //nolint:staticcheck,revive // starts with a proper noun
		}
		return fmt.Errorf("Google %d yanıtı verdi", res.StatusCode) //nolint:staticcheck,revive // starts with a proper noun
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("answer could not be read: %w", err)
	}
	return nil
}

// remoteCode picks Google's machine-readable error code out of an error
// answer ("invalid_grant", "PERMISSION_DENIED", "insufficientFilePermissions"),
// keeping only letters, digits and _.- and at most 60 of them.
func remoteCode(body []byte) string {
	var e struct {
		Error json.RawMessage `json:"error"`
	}
	if json.Unmarshal(body, &e) != nil || len(e.Error) == 0 {
		return ""
	}
	var flat string
	if json.Unmarshal(e.Error, &flat) == nil {
		return clean(flat)
	}
	var nested struct {
		Status string `json:"status"`
		Errors []struct {
			Reason string `json:"reason"`
		} `json:"errors"`
	}
	if json.Unmarshal(e.Error, &nested) != nil {
		return ""
	}
	if len(nested.Errors) > 0 && nested.Errors[0].Reason != "" {
		return clean(nested.Errors[0].Reason)
	}
	return clean(nested.Status)
}

func clean(s string) string {
	var b strings.Builder
	for _, r := range s {
		if b.Len() >= 60 {
			break
		}
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '.' || r == '-' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
