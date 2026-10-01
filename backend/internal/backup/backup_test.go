package backup

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"gorm.io/gorm"

	"github.com/toprakgureli/santral-c/backend/configs"
	"github.com/toprakgureli/santral-c/backend/internal/audit"
	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/internal/testdb"
	"github.com/toprakgureli/santral-c/backend/pkg/crypt"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
)

// fakeDrive stands in for Google: it signs the service account in (checking
// the signed request with the account's public key), answers what the
// account may do in the folder, keeps what was uploaded and the locks put
// on files. refuseLock answers a lock with a refusal; stillWritable accepts
// the lock but keeps the file writable for the account.
type fakeDrive struct {
	t             *testing.T
	public        *rsa.PublicKey
	sharedOK      bool
	canDelete     bool
	refuseLock    bool
	stillWritable bool
	mu            sync.Mutex
	uploaded      map[string][]byte
	ids           map[string]string // file id -> name
	locks         []lockCall
	deleteSeen    bool
	foreignToken  bool
}

// lockCall is one content restriction asked for.
type lockCall struct {
	ID       string
	ReadOnly bool
	Owner    bool
	Reason   string
}

// secretRemote is part of an error answer that must never reach the panel.
const secretRemote = "internal-detail-from-google-0451"

func (f *fakeDrive) locked(id string) bool {
	for _, l := range f.locks {
		if l.ID == id && l.ReadOnly {
			return true
		}
	}
	return false
}

func (f *fakeDrive) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		_, err := jwt.Parse(r.Form.Get("assertion"), func(*jwt.Token) (any, error) { return f.public, nil }, jwt.WithValidMethods([]string{"RS256"}))
		if err != nil {
			http.Error(w, "bad assertion", http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "tok"})
	})
	mux.HandleFunc("/elsewhere/token", func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		f.foreignToken = true
		f.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "tok"})
	})
	mux.HandleFunc("/files/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			f.deleteSeen = true
		}
		if r.Header.Get("Authorization") != "Bearer tok" {
			http.Error(w, "no token", http.StatusUnauthorized)
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/files/")
		f.mu.Lock()
		_, isFile := f.ids[id]
		f.mu.Unlock()
		if isFile {
			f.file(w, r, id)
			return
		}
		drive := ""
		if f.sharedOK {
			drive = "shared-1"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name": "Yedekler", "mimeType": "application/vnd.google-apps.folder", "driveId": drive,
			"capabilities": map[string]bool{"canAddChildren": true, "canDeleteChildren": f.canDelete, "canTrashChildren": f.canDelete},
		})
	})
	mux.HandleFunc("/upload", func(w http.ResponseWriter, r *http.Request) {
		var meta struct {
			Name    string   `json:"name"`
			Parents []string `json:"parents"`
		}
		_ = json.NewDecoder(r.Body).Decode(&meta)
		w.Header().Set("Location", "http://"+r.Host+"/session/"+meta.Name)
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/session/", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		name := strings.TrimPrefix(r.URL.Path, "/session/")
		f.mu.Lock()
		f.uploaded[name] = body
		id := "file-" + strconv.Itoa(len(f.ids)+1)
		f.ids[id] = name
		f.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]string{"id": id, "size": strconv.Itoa(len(body))})
	})
	return mux
}

// file answers for an uploaded file: a lock (PATCH) and its rights (GET).
func (f *fakeDrive) file(w http.ResponseWriter, r *http.Request, id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch r.Method {
	case http.MethodPatch:
		if f.refuseLock {
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{
				"code": 403, "message": "The user does not have permission " + secretRemote, "status": "PERMISSION_DENIED",
				"errors": []map[string]string{{"reason": "insufficientFilePermissions", "message": secretRemote}},
			}})
			return
		}
		var in struct {
			ContentRestrictions []struct {
				ReadOnly        bool   `json:"readOnly"`
				OwnerRestricted bool   `json:"ownerRestricted"`
				Reason          string `json:"reason"`
			} `json:"contentRestrictions"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		for _, c := range in.ContentRestrictions {
			f.locks = append(f.locks, lockCall{ID: id, ReadOnly: c.ReadOnly, Owner: c.OwnerRestricted, Reason: c.Reason})
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"id": id})
	case http.MethodGet:
		restrictions := []map[string]bool{}
		if f.locked(id) {
			restrictions = append(restrictions, map[string]bool{"readOnly": true})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"capabilities":        map[string]bool{"canModifyContent": !f.locked(id) || f.stillWritable},
			"contentRestrictions": restrictions,
		})
	default:
		http.Error(w, "unexpected", http.StatusMethodNotAllowed)
	}
}

type allowAll struct{ id uint }

func (a allowAll) GetByID(_ context.Context, _ uint) (*models.User, error) {
	return &models.User{ID: a.id, Roles: []models.Role{{Name: "tester", Permissions: []models.Permission{{Key: string(enums.SystemBackup)}}}}}, nil
}

type noAudit struct{}

func (noAudit) Record(context.Context, audit.Entry) {}

func setup(t *testing.T, f *fakeDrive) (*Service, string) {
	t.Helper()
	db := testdb.Open(t)
	clean := func() { db.Exec("DELETE FROM backup_runs"); db.Exec("DELETE FROM backup_settings") }
	clean()
	t.Cleanup(clean)

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	f.public = &key.PublicKey
	f.uploaded = map[string][]byte{}
	f.ids = map[string]string{}
	srv := httptest.NewServer(f.handler())
	t.Cleanup(srv.Close)
	oldToken, oldFiles, oldUpload := tokenURL, driveFilesURL, driveUploadURL
	tokenURL, driveFilesURL, driveUploadURL = srv.URL+"/token", srv.URL+"/files", srv.URL+"/upload"
	t.Cleanup(func() { tokenURL, driveFilesURL, driveUploadURL = oldToken, oldFiles, oldUpload })

	oldDump := dumpDatabase
	dumpDatabase = func(context.Context, configs.Database) (string, string, error) {
		dir := t.TempDir()
		path := filepath.Join(dir, "santral.dump")
		return dir, path, os.WriteFile(path, []byte("PGDMP-test-content"), 0o600)
	}
	t.Cleanup(func() { dumpDatabase = oldDump })

	pemKey := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	// The key file names another token address; it must not be used.
	sa, _ := json.Marshal(map[string]string{"type": "service_account", "client_email": "yedek@proje.iam.gserviceaccount.com", "private_key": string(pemKey), "token_uri": srv.URL + "/elsewhere/token"})
	ring, err := crypt.NewKeyring(strings.Repeat("cd", 32))
	if err != nil {
		t.Fatal(err)
	}
	return NewService(db, configs.Database{}, ring, allowAll{id: 1}, noAudit{}), string(sa)
}

func lastRun(t *testing.T, db *gorm.DB) models.BackupRun {
	t.Helper()
	var r models.BackupRun
	if err := db.Order("id DESC").First(&r).Error; err != nil {
		t.Fatal(err)
	}
	return r
}

func TestBackupRefusesAnAccountThatCanDelete(t *testing.T) {
	f := &fakeDrive{t: t, sharedOK: true, canDelete: true}
	s, sa := setup(t, f)
	ctx := context.Background()
	if _, err := s.Save(ctx, 1, Input{Enabled: true, FolderID: "https://drive.google.com/drive/folders/abc123?usp=sharing", Credentials: sa}, ""); err != nil {
		t.Fatal(err)
	}
	s.run(ctx, nil)
	r := lastRun(t, s.db)
	if r.OK || !strings.Contains(r.Error, "silebiliyor") {
		t.Fatalf("run = %+v, want refused for delete rights", r)
	}
	if len(f.uploaded) != 0 {
		t.Fatal("a backup was uploaded with an account that can delete")
	}
}

func TestBackupRefusesAFolderOutsideASharedDrive(t *testing.T) {
	f := &fakeDrive{t: t, sharedOK: false}
	s, sa := setup(t, f)
	ctx := context.Background()
	if _, err := s.Save(ctx, 1, Input{Enabled: true, FolderID: "abc123", Credentials: sa}, ""); err != nil {
		t.Fatal(err)
	}
	s.run(ctx, nil)
	if r := lastRun(t, s.db); r.OK || !strings.Contains(r.Error, "Ortak Drive") {
		t.Fatalf("run = %+v, want refused outside a shared drive", r)
	}
}

func TestBackupUploadsWithAContributor(t *testing.T) {
	f := &fakeDrive{t: t, sharedOK: true}
	s, sa := setup(t, f)
	ctx := context.Background()
	v, err := s.Save(ctx, 1, Input{Enabled: true, FolderID: "https://drive.google.com/drive/folders/abc123", Credentials: sa}, "")
	if err != nil {
		t.Fatal(err)
	}
	if v.FolderID != "abc123" || v.Account != "yedek@proje.iam.gserviceaccount.com" || !v.HasKey {
		t.Fatalf("settings = %+v", v)
	}
	check, err := s.Check(ctx, 1)
	if err != nil || check.Problem() != "" {
		t.Fatalf("check = %+v %v", check, err)
	}
	s.run(ctx, nil)
	r := lastRun(t, s.db)
	if !r.OK || !strings.HasPrefix(r.File, "santral-") || !strings.HasSuffix(r.File, ".dump") {
		t.Fatalf("run = %+v", r)
	}
	if got := string(f.uploaded[r.File]); got != "PGDMP-test-content" {
		t.Fatalf("uploaded %q", got)
	}
	if f.deleteSeen {
		t.Fatal("the backup asked Drive to delete something")
	}
	if f.foreignToken {
		t.Fatal("the token address inside the key file was used")
	}
	if check.CanLock == nil || !*check.CanLock {
		t.Fatalf("check did not lock its test file: %+v", check)
	}
	// The check's own file and the backup are both locked for good.
	var backupID string
	for id, name := range f.ids {
		if name == r.File {
			backupID = id
		}
	}
	if len(f.ids) != 2 || len(f.locks) != 2 {
		t.Fatalf("files %v, locks %+v; want the test file and the backup, both locked", f.ids, f.locks)
	}
	for _, l := range f.locks {
		if !l.ReadOnly || !l.Owner || l.Reason == "" {
			t.Fatalf("lock %+v is not read-only for everyone but an organizer", l)
		}
	}
	if !f.locked(backupID) {
		t.Fatalf("the backup %s was not locked: %+v", backupID, f.locks)
	}
	// Not due again for six hours.
	before := len(f.uploaded)
	s.dueRun(ctx)
	if len(f.uploaded) != before {
		t.Fatal("a second backup ran before six hours passed")
	}
}

func TestBackupKeyIsChecked(t *testing.T) {
	s, _ := setup(t, &fakeDrive{t: t})
	if _, err := s.Save(context.Background(), 1, Input{FolderID: "abc", Credentials: `{"type":"user"}`}, ""); err == nil {
		t.Fatal("a non service account key was accepted")
	}
	if _, err := s.Save(context.Background(), 1, Input{Enabled: true, FolderID: "abc"}, ""); err == nil {
		t.Fatal("backups were switched on without a key")
	}
}

// TestRealDump runs the real pg_dump against the test database when it is
// installed (it is in CI) and checks the file is one pg_restore reads.
func TestRealDump(t *testing.T) {
	testdb.Open(t)
	if _, err := lookPath(pgDump()); err != nil {
		t.Skip("pg_dump is not installed here")
	}
	cfg := configs.Database{Host: "localhost", Port: "5432", User: "santral", Password: "santral", Name: "santral_test", SSLMode: "disable"}
	for _, kv := range strings.Fields(os.Getenv(testdb.EnvDSN)) {
		k, v, _ := strings.Cut(kv, "=")
		switch k {
		case "host":
			cfg.Host = v
		case "port":
			cfg.Port = v
		case "user":
			cfg.User = v
		case "password":
			cfg.Password = v
		case "dbname":
			cfg.Name = v
		}
	}
	dir, path, err := dump(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	head := make([]byte, 5)
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	if _, err := io.ReadFull(file, head); err != nil || string(head) != "PGDMP" {
		t.Fatalf("not a custom-format dump: %q %v", head, err)
	}
}

// TestBackupFailsWhenItCannotBeLocked: Drive refuses the lock. The copy is
// reported as failed with a plain reason, Google's own words stay out, and
// the panel's check says the account cannot lock.
func TestBackupFailsWhenItCannotBeLocked(t *testing.T) {
	f := &fakeDrive{t: t, sharedOK: true, refuseLock: true}
	s, sa := setup(t, f)
	ctx := context.Background()
	if _, err := s.Save(ctx, 1, Input{Enabled: true, FolderID: "abc123", Credentials: sa}, ""); err != nil {
		t.Fatal(err)
	}
	s.run(ctx, nil)
	r := lastRun(t, s.db)
	if r.OK || !strings.Contains(r.Error, "kilitlenemedi") {
		t.Fatalf("run = %+v, want failed for the lock", r)
	}
	if strings.Contains(r.Error, secretRemote) || !strings.Contains(r.Error, "insufficientFilePermissions") {
		t.Fatalf("error %q should give Google's code and nothing else of its answer", r.Error)
	}
	check, err := s.Check(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if check.CanLock == nil || *check.CanLock || !strings.Contains(check.Problem(), "kilitleyemiyor") {
		t.Fatalf("check = %+v, problem %q; want cannot lock", check, check.Problem())
	}
	if strings.Contains(check.Problem(), secretRemote) {
		t.Fatalf("problem %q repeats Google's answer", check.Problem())
	}
	if f.deleteSeen {
		t.Fatal("a file was deleted")
	}
}

// TestBackupFailsWhenTheLockDoesNotHold: Drive takes the lock but the
// account can still change the file, so the copy is not counted as safe.
func TestBackupFailsWhenTheLockDoesNotHold(t *testing.T) {
	f := &fakeDrive{t: t, sharedOK: true, stillWritable: true}
	s, sa := setup(t, f)
	ctx := context.Background()
	if _, err := s.Save(ctx, 1, Input{Enabled: true, FolderID: "abc123", Credentials: sa}, ""); err != nil {
		t.Fatal(err)
	}
	s.run(ctx, nil)
	if r := lastRun(t, s.db); r.OK || !strings.Contains(r.Error, "hâlâ değiştirebiliyor") {
		t.Fatalf("run = %+v, want failed because the file stays writable", r)
	}
}

func TestRemoteCodeKeepsOnlyTheCode(t *testing.T) {
	for body, want := range map[string]string{
		`{"error":"invalid_grant","error_description":"Invalid JWT: secret detail"}`:              "invalid_grant",
		`{"error":{"code":404,"message":"File not found: x","status":"NOT_FOUND"}}`:               "NOT_FOUND",
		`{"error":{"errors":[{"reason":"notFound<script>","message":"x"}],"status":"NOT_FOUND"}}`: "notFoundscript",
		`<html>proxy error</html>`:                     "",
		`{"error":"` + strings.Repeat("a", 200) + `"}`: strings.Repeat("a", 60),
	} {
		if got := remoteCode([]byte(body)); got != want {
			t.Errorf("remoteCode(%s) = %q, want %q", body, got, want)
		}
	}
}
