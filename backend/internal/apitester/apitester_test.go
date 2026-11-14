package apitester_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/toprakgureli/santral-c/backend/internal/apitester"
	"github.com/toprakgureli/santral-c/backend/internal/audit"
	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/pkg/crypt"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
)

type fakeActors struct {
	users map[uint]*models.User
}

func (f fakeActors) GetByID(_ context.Context, id uint) (*models.User, error) {
	return f.users[id], nil
}

type fakeAudit struct {
	entries []audit.Entry
}

func (f *fakeAudit) Record(_ context.Context, e audit.Entry) {
	f.entries = append(f.entries, e)
}

type memRepo struct {
	enabled     bool
	collections map[uint]*models.APICollection
	requests    map[uint]*models.APIRequest
	logs        []models.APIRunLog
	nextColID   uint
	nextReqID   uint
	nextLogID   uint
}

func newMemRepo() *memRepo {
	return &memRepo{
		enabled:     true,
		collections: map[uint]*models.APICollection{},
		requests:    map[uint]*models.APIRequest{},
		nextColID:   1,
		nextReqID:   1,
		nextLogID:   1,
	}
}

func (m *memRepo) IsEnabled(context.Context) (bool, error) { return m.enabled, nil }
func (m *memRepo) SetEnabled(_ context.Context, enabled bool) error {
	m.enabled = enabled
	return nil
}

func (m *memRepo) ListCollections(_ context.Context, onlyActive bool) ([]models.APICollection, error) {
	var out []models.APICollection
	for _, c := range m.collections {
		if onlyActive && !c.Active {
			continue
		}
		copyCol := *c
		copyCol.Requests = nil
		for _, r := range m.requests {
			if r.CollectionID != c.ID {
				continue
			}
			if onlyActive && !r.Active {
				continue
			}
			copyCol.Requests = append(copyCol.Requests, *r)
		}
		out = append(out, copyCol)
	}
	return out, nil
}

func (m *memRepo) GetCollection(_ context.Context, id uint) (*models.APICollection, error) {
	return m.collections[id], nil
}

func (m *memRepo) NextCollectionOrder(context.Context) (int, error) {
	return len(m.collections) + 1, nil
}

func (m *memRepo) CreateCollection(_ context.Context, col *models.APICollection) error {
	col.ID = m.nextColID
	m.nextColID++
	cp := *col
	m.collections[col.ID] = &cp
	return nil
}

func (m *memRepo) ImportCollection(ctx context.Context, col *models.APICollection, reqs []*models.APIRequest) error {
	if err := m.CreateCollection(ctx, col); err != nil {
		return err
	}
	for i, r := range reqs {
		r.CollectionID = col.ID
		r.SortOrder = i + 1
		if err := m.CreateRequest(ctx, r); err != nil {
			return err
		}
	}
	return nil
}

func (m *memRepo) UpdateCollection(_ context.Context, col *models.APICollection) error {
	cp := *col
	m.collections[col.ID] = &cp
	return nil
}

func (m *memRepo) DeleteCollection(_ context.Context, id uint) error {
	delete(m.collections, id)
	for rid, r := range m.requests {
		if r.CollectionID == id {
			delete(m.requests, rid)
		}
	}
	return nil
}

func (m *memRepo) GetRequest(_ context.Context, id uint) (*models.APIRequest, error) {
	return m.requests[id], nil
}

func (m *memRepo) NextRequestOrder(_ context.Context, colID uint) (int, error) {
	n := 0
	for _, r := range m.requests {
		if r.CollectionID == colID {
			n++
		}
	}
	return n + 1, nil
}

func (m *memRepo) CreateRequest(_ context.Context, req *models.APIRequest) error {
	req.ID = m.nextReqID
	m.nextReqID++
	cp := *req
	m.requests[req.ID] = &cp
	return nil
}

func (m *memRepo) UpdateRequest(_ context.Context, req *models.APIRequest) error {
	cp := *req
	m.requests[req.ID] = &cp
	return nil
}

func (m *memRepo) DeleteRequest(_ context.Context, id uint) error {
	delete(m.requests, id)
	return nil
}

func (m *memRepo) CreateRunLog(_ context.Context, row *models.APIRunLog) error {
	row.ID = m.nextLogID
	m.nextLogID++
	m.logs = append(m.logs, *row)
	return nil
}

func (m *memRepo) ListRunLogs(_ context.Context, requestID *uint, _ int) ([]models.APIRunLog, error) {
	var out []models.APIRunLog
	for i := len(m.logs) - 1; i >= 0; i-- {
		if requestID != nil && (m.logs[i].RequestID == nil || *m.logs[i].RequestID != *requestID) {
			continue
		}
		out = append(out, m.logs[i])
	}
	return out, nil
}

func userWithPerms(id uint, perms ...enums.Permission) *models.User {
	ps := make([]models.Permission, 0, len(perms))
	for _, p := range perms {
		ps = append(ps, models.Permission{Key: string(p)})
	}
	return &models.User{
		ID:    id,
		Name:  "Test User",
		Email: "test@example.com",
		Roles: []models.Role{{ID: 1, Name: "custom", Permissions: ps}},
	}
}

func testRing(t *testing.T) *crypt.Keyring {
	t.Helper()
	ring, err := crypt.NewKeyring(strings.Repeat("ab", 32))
	if err != nil {
		t.Fatalf("NewKeyring: %v", err)
	}
	return ring
}

func TestTemplateAndRedact(t *testing.T) {
	defs := []apitester.VariableDef{
		{Key: "phone", Label: "Telefon", Required: true},
		{Key: "env", Label: "Ortam", DefaultValue: "prod"},
	}
	if _, err := apitester.BuildVars("https://api.example.com/", defs, nil); err == nil {
		t.Fatal("expected error for missing required variable")
	}

	vars, err := apitester.BuildVars("https://api.example.com/", defs, map[string]string{"phone": "5551234567"})
	if err != nil {
		t.Fatalf("BuildVars: %v", err)
	}
	target, err := apitester.RenderTarget("{{base_url}}/v1/{{env}}/customers/{{phone}}", []apitester.KeyValue{
		{Key: "apiKey", Value: "secret-123"},
	}, vars)
	if err != nil {
		t.Fatalf("RenderTarget: %v", err)
	}
	want := "https://api.example.com/v1/prod/customers/5551234567?apiKey=secret-123"
	if target != want {
		t.Fatalf("RenderTarget = %q, want %q", target, want)
	}

	redacted := apitester.RedactLogURL(target)
	if strings.Contains(redacted, "secret-123") {
		t.Fatalf("RedactLogURL leaked query value: %s", redacted)
	}
}

func TestSecretMaskingAndProxyExecution(t *testing.T) {
	ctx := context.Background()
	repo := newMemRepo()
	auditor := &fakeAudit{}
	actors := fakeActors{users: map[uint]*models.User{
		1: userWithPerms(1, enums.APITesterView, enums.APITesterExecute, enums.APITesterManage),
		2: userWithPerms(2, enums.APITesterView, enums.APITesterExecute),
		3: userWithPerms(3, enums.APITesterView),
		4: userWithPerms(4),
	}}
	svc := apitester.NewService(repo, actors, auditor, testRing(t))

	var gotAuth, gotBody, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotQuery = r.URL.RawQuery
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()
	svc.SetHTTPClient(srv.Client())

	// Non-manager cannot create collections.
	if _, err := svc.CreateCollection(ctx, 2, apitester.CollectionInput{Name: "Blocked"}, "1.2.3.4"); err == nil {
		t.Fatal("expected forbidden when non-manager creates collection")
	}

	col, err := svc.CreateCollection(ctx, 1, apitester.CollectionInput{
		Name:       "Müşteri ERP",
		ProjectTag: "ERP",
		BaseURL:    srv.URL,
	}, "1.2.3.4")
	if err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}

	clientReq, err := svc.CreateRequest(ctx, 1, col.ID, apitester.RequestInput{
		Name:          "Sipariş Sorgula",
		Method:        "POST",
		URLTemplate:   "{{base_url}}/orders/{{order_id}}",
		Headers:       []apitester.KeyValue{{Key: "Authorization", Value: "Bearer client-tok", Secret: true}},
		QueryParams:   []apitester.KeyValue{{Key: "token", Value: "qsec"}},
		BodyTemplate:  `{"orderId":"{{order_id}}"}`,
		ExecutionMode: apitester.ModeClient,
		Variables:     []apitester.VariableDef{{Key: "order_id", Label: "Sipariş ID", Required: true}},
	}, "1.2.3.4")
	if err != nil {
		t.Fatalf("CreateRequest client: %v", err)
	}

	// Verify headers were actually encrypted at rest in repository.
	stored := repo.requests[clientReq.ID]
	if strings.Contains(stored.HeadersEnc, "client-tok") {
		t.Fatal("expected headers to be encrypted at rest")
	}

	serverReq, err := svc.CreateRequest(ctx, 1, col.ID, apitester.RequestInput{
		Name:          "Gizli Sunucu Sorgusu",
		Method:        "POST",
		URLTemplate:   "{{base_url}}/internal/{{phone}}",
		Headers:       []apitester.KeyValue{{Key: "Authorization", Value: "Bearer srv-secret", Secret: true}},
		BodyTemplate:  `{"phone":"{{phone}}"}`,
		ExecutionMode: apitester.ModeServer,
		Variables:     []apitester.VariableDef{{Key: "phone", Label: "Telefon", Required: true}},
	}, "1.2.3.4")
	if err != nil {
		t.Fatalf("CreateRequest server: %v", err)
	}

	// View-only user sees masked secret on BOTH client and server requests.
	viewList, err := svc.List(ctx, 3)
	if err != nil {
		t.Fatalf("List view-only: %v", err)
	}
	for _, r := range viewList.Collections[0].Requests {
		if r.Headers[0].Value != apitester.MaskedSecret {
			t.Fatalf("expected masked secret for viewer on request %s, got %q", r.Name, r.Headers[0].Value)
		}
	}

	// Executor sees client request secret (needed for client-IP fetch) but NEVER server request secret.
	execList, err := svc.List(ctx, 2)
	if err != nil {
		t.Fatalf("List executor: %v", err)
	}
	for _, r := range execList.Collections[0].Requests {
		if r.ID == clientReq.ID && r.Headers[0].Value != "Bearer client-tok" {
			t.Fatalf("expected decrypted client header for executor, got %q", r.Headers[0].Value)
		}
		if r.ID == serverReq.ID && r.Headers[0].Value != apitester.MaskedSecret {
			t.Fatalf("expected masked server secret for executor, got %q", r.Headers[0].Value)
		}
	}

	// Proxy execution injects the decrypted server secret and logs redacted URL.
	res, err := svc.ExecuteProxy(ctx, 2, serverReq.ID, apitester.ProxyInput{
		Variables: map[string]string{"phone": "5559998877"},
	}, "1.2.3.4")
	if err != nil {
		t.Fatalf("ExecuteProxy: %v", err)
	}
	if res.StatusCode != 200 || gotAuth != "Bearer srv-secret" || gotBody != `{"phone":"5559998877"}` {
		t.Fatalf("unexpected proxy result/request: res=%+v auth=%q body=%q", res, gotAuth, gotBody)
	}

	// Also verify query param redaction when running fallback on clientReq.
	res2, err := svc.ExecuteProxy(ctx, 2, clientReq.ID, apitester.ProxyInput{
		Variables: map[string]string{"order_id": "ORD-9"},
	}, "1.2.3.4")
	if err != nil || res2.StatusCode != 200 || gotQuery != "token=qsec" {
		t.Fatalf("fallback proxy failed: err=%v res=%+v query=%q", err, res2, gotQuery)
	}
	logs, err := svc.ListRunLogs(ctx, 2, nil, 10)
	if err != nil || len(logs) != 2 {
		t.Fatalf("expected 2 run logs, got %d (err=%v)", len(logs), err)
	}
	if strings.Contains(logs[0].TargetURL, "qsec") {
		t.Fatalf("run log leaked query secret: %s", logs[0].TargetURL)
	}

	// When disabled, non-managers get empty collections and cannot execute proxy.
	if _, err := svc.SetStatus(ctx, 1, false, "1.2.3.4"); err != nil {
		t.Fatalf("SetStatus: %v", err)
	}
	disabledList, err := svc.List(ctx, 2)
	if err != nil || disabledList.Enabled || len(disabledList.Collections) != 0 {
		t.Fatalf("expected disabled empty list, got %+v err=%v", disabledList, err)
	}
	if _, err := svc.ExecuteProxy(ctx, 2, serverReq.ID, apitester.ProxyInput{
		Variables: map[string]string{"phone": "555"},
	}, "1.2.3.4"); err == nil {
		t.Fatal("expected error executing while disabled")
	}

	// User without permissions is denied.
	if _, err := svc.List(ctx, 4); err == nil {
		t.Fatal("expected forbidden for user without permissions")
	}
}

func TestImportCollectionAtomicAndEncrypted(t *testing.T) {
	ctx := context.Background()
	repo := newMemRepo()
	actors := fakeActors{users: map[uint]*models.User{
		1: userWithPerms(1, enums.APITesterView, enums.APITesterExecute, enums.APITesterManage),
		2: userWithPerms(2, enums.APITesterView, enums.APITesterExecute),
	}}
	svc := apitester.NewService(repo, actors, &fakeAudit{}, testRing(t))

	// Executor without manage permission cannot import.
	if _, err := svc.ImportCollection(ctx, 2, apitester.ImportCollectionInput{
		Collection: apitester.CollectionInput{Name: "Postman Paketi"},
	}, "1.2.3.4"); err == nil {
		t.Fatal("expected forbidden when executor imports collection")
	}

	// Invalid request in batch rejects before creating the collection.
	if _, err := svc.ImportCollection(ctx, 1, apitester.ImportCollectionInput{
		Collection: apitester.CollectionInput{Name: "Bozuk Paket"},
		Requests: []apitester.RequestInput{
			{Name: "Ok", Method: "GET", URLTemplate: "https://api.example.com/ok"},
			{Name: "Bad", Method: "BADMETHOD", URLTemplate: "https://api.example.com/bad"},
		},
	}, "1.2.3.4"); err == nil {
		t.Fatal("expected validation failure for invalid request in import batch")
	}
	if len(repo.collections) != 0 {
		t.Fatalf("expected 0 collections after failed import validation, got %d", len(repo.collections))
	}

	col, err := svc.ImportCollection(ctx, 1, apitester.ImportCollectionInput{
		Collection: apitester.CollectionInput{
			Name:       "Postman Entegrasyon",
			ProjectTag: "CRM",
			BaseURL:    "https://api.crm.example",
		},
		Requests: []apitester.RequestInput{
			{
				Name:        "Müşteri Sorgula",
				Method:      "GET",
				URLTemplate: "{{base_url}}/customers/{{phone}}",
				Headers:     []apitester.KeyValue{{Key: "Authorization", Value: "Bearer secret-pm-tok", Secret: true}},
				Variables:   []apitester.VariableDef{{Key: "phone", Label: "Telefon", Required: true, Source: "call_phone"}},
			},
			{
				Name:         "Sipariş Oluştur",
				Method:       "POST",
				URLTemplate:  "{{base_url}}/orders",
				BodyTemplate: `{"phone":"{{phone}}"}`,
			},
		},
	}, "1.2.3.4")
	if err != nil {
		t.Fatalf("ImportCollection: %v", err)
	}
	if len(col.Requests) != 2 {
		t.Fatalf("expected 2 imported requests, got %d", len(col.Requests))
	}
	if strings.Contains(repo.requests[col.Requests[0].ID].HeadersEnc, "secret-pm-tok") {
		t.Fatal("expected imported headers to be encrypted at rest")
	}
}
