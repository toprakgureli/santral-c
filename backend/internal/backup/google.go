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
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// The Drive API addresses. Tests point them at a stand-in.
var (
	driveFilesURL  = "https://www.googleapis.com/drive/v3/files"
	driveUploadURL = "https://www.googleapis.com/upload/drive/v3/files"
)

// driveScope lets the service account work in the folders shared with it.
// What it may do there is decided by its role in the Shared Drive: as a
// Contributor it can add files and never delete them.
const driveScope = "https://www.googleapis.com/auth/drive"

// serviceAccount is the part of a Google service account key file we use.
type serviceAccount struct {
	Type        string `json:"type"`
	ClientEmail string `json:"client_email"`
	PrivateKey  string `json:"private_key"`
	TokenURI    string `json:"token_uri"`
}

// parseServiceAccount reads a key file and checks it is one.
func parseServiceAccount(raw string) (*serviceAccount, error) {
	var sa serviceAccount
	if err := json.Unmarshal([]byte(raw), &sa); err != nil {
		return nil, errors.New("anahtar dosyası okunamadı; Google Cloud'dan indirilen JSON dosyasının içeriğini yapıştırın")
	}
	if sa.Type != "service_account" || sa.ClientEmail == "" || sa.PrivateKey == "" {
		return nil, errors.New("bu bir servis hesabı anahtarı değil (type, client_email ve private_key alanları olmalı)")
	}
	if sa.TokenURI == "" {
		sa.TokenURI = "https://oauth2.googleapis.com/token"
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
	key, err := jwt.ParseRSAPrivateKeyFromPEM([]byte(g.sa.PrivateKey))
	if err != nil {
		return "", fmt.Errorf("private key: %w", err)
	}
	now := time.Now()
	assertion, err := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss":   g.sa.ClientEmail,
		"scope": driveScope,
		"aud":   g.sa.TokenURI,
		"iat":   now.Unix(),
		"exp":   now.Add(time.Hour).Unix(),
	}).SignedString(key)
	if err != nil {
		return "", fmt.Errorf("token request could not be signed: %w", err)
	}
	form := url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"}, "assertion": {assertion}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.sa.TokenURI, strings.NewReader(form.Encode()))
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
type FolderCheck struct {
	Name        string `json:"name"`
	SharedDrive bool   `json:"sharedDrive"`
	CanAdd      bool   `json:"canAdd"`
	CanDelete   bool   `json:"canDelete"`
}

// Problem says why backups cannot go to this folder, or "".
func (f FolderCheck) Problem() string {
	switch {
	case !f.SharedDrive:
		return "Klasör bir Ortak Drive'da değil. Silme yetkisini Google'ın kendisinin engellemesi için yedekler bir Ortak Drive'a gitmeli."
	case !f.CanAdd:
		return "Servis hesabı bu klasöre dosya ekleyemiyor. Ortak Drive'a \"Katkıda bulunan\" olarak ekleyin."
	case f.CanDelete:
		return "Servis hesabı bu klasördeki dosyaları silebiliyor. Güvenlik için yedek alınmadı: Ortak Drive'da rolünü \"Katkıda bulunan\" yapın."
	}
	return ""
}

// checkFolder asks Drive what the service account may do in folder.
func (g *google) checkFolder(ctx context.Context, token, folder string) (*FolderCheck, error) {
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
		return nil, fmt.Errorf("klasöre ulaşılamadı (kimliği ve paylaşımı kontrol edin): %w", err)
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

// upload sends a file into folder and returns its Drive id. Nothing here
// deletes, renames or replaces: a backup only ever adds a new file.
func (g *google) upload(ctx context.Context, token, folder, name, path string) (string, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, fmt.Errorf("dump could not be opened: %w", err)
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return "", 0, fmt.Errorf("dump could not be read: %w", err)
	}
	meta, _ := json.Marshal(map[string]any{"name": name, "parents": []string{folder}, "mimeType": "application/octet-stream"})
	start, err := http.NewRequestWithContext(ctx, http.MethodPost, driveUploadURL+"?uploadType=resumable&supportsAllDrives=true&fields=id,size", bytes.NewReader(meta))
	if err != nil {
		return "", 0, err
	}
	start.Header.Set("Authorization", "Bearer "+token)
	start.Header.Set("Content-Type", "application/json; charset=UTF-8")
	start.Header.Set("X-Upload-Content-Type", "application/octet-stream")
	start.Header.Set("X-Upload-Content-Length", fmt.Sprint(info.Size()))
	res, err := g.http.Do(start)
	if err != nil {
		return "", 0, fmt.Errorf("Drive'a ulaşılamadı: %w", err) //nolint:staticcheck,revive // starts with a proper noun
	}
	_ = res.Body.Close()
	session := res.Header.Get("Location")
	if res.StatusCode != http.StatusOK || session == "" {
		return "", 0, fmt.Errorf("yükleme başlatılamadı (Drive %d)", res.StatusCode)
	}
	put, err := http.NewRequestWithContext(ctx, http.MethodPut, session, file)
	if err != nil {
		return "", 0, err
	}
	put.ContentLength = info.Size()
	put.Header.Set("Content-Type", "application/octet-stream")
	var out struct {
		ID   string `json:"id"`
		Size string `json:"size"`
	}
	if err := g.doJSON(put, &out); err != nil {
		return "", 0, fmt.Errorf("yükleme tamamlanamadı: %w", err)
	}
	if out.ID == "" {
		return "", 0, errors.New("Drive dosya kimliği vermedi") //nolint:staticcheck,revive // starts with a proper noun
	}
	return out.ID, info.Size(), nil
}

func (g *google) doJSON(req *http.Request, out any) error {
	res, err := g.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode/100 != 2 {
		return fmt.Errorf("%d: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("answer could not be read: %w", err)
	}
	return nil
}
