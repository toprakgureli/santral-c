package apitester

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/toprakgureli/santral-c/backend/internal/audit"
	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/internal/whatsapp/outside"
	"github.com/toprakgureli/santral-c/backend/pkg/crypt"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
)

const (
	sealPurposeHeaders   = "api_tester_headers"
	maxProxyResponseBody = 1 << 20 // 1 MiB
	maxProxyTimeout      = 20 * time.Second
)

// IActorResolver loads the acting user for permission checks.
type IActorResolver interface {
	GetByID(ctx context.Context, id uint) (*models.User, error)
}

// IAudit records privileged mutations and request executions.
type IAudit interface {
	Record(ctx context.Context, e audit.Entry)
}

// IRepo defines persistence operations for the API tester module.
type IRepo interface {
	IsEnabled(ctx context.Context) (bool, error)
	SetEnabled(ctx context.Context, enabled bool) error
	ListCollections(ctx context.Context, onlyActive bool) ([]models.APICollection, error)
	GetCollection(ctx context.Context, id uint) (*models.APICollection, error)
	NextCollectionOrder(ctx context.Context) (int, error)
	CreateCollection(ctx context.Context, col *models.APICollection) error
	ImportCollection(ctx context.Context, col *models.APICollection, reqs []*models.APIRequest) error
	UpdateCollection(ctx context.Context, col *models.APICollection) error
	DeleteCollection(ctx context.Context, id uint) error
	GetRequest(ctx context.Context, id uint) (*models.APIRequest, error)
	NextRequestOrder(ctx context.Context, collectionID uint) (int, error)
	CreateRequest(ctx context.Context, req *models.APIRequest) error
	UpdateRequest(ctx context.Context, req *models.APIRequest) error
	DeleteRequest(ctx context.Context, id uint) error
	CreateRunLog(ctx context.Context, row *models.APIRunLog) error
	ListRunLogs(ctx context.Context, requestID *uint, limit int) ([]models.APIRunLog, error)
}

// Service implements API tester collections, templates, logs and server proxy execution.
type Service struct {
	repo       IRepo
	actors     IActorResolver
	audit      IAudit
	ring       *crypt.Keyring
	httpClient *http.Client
}

// NewService builds the API tester service.
func NewService(repo IRepo, actors IActorResolver, auditor IAudit, ring *crypt.Keyring) *Service {
	return &Service{
		repo:       repo,
		actors:     actors,
		audit:      auditor,
		ring:       ring,
		httpClient: outside.Client,
	}
}

// SetHTTPClient overrides the HTTP client (intended for tests).
func (s *Service) SetHTTPClient(c *http.Client) {
	if c != nil {
		s.httpClient = c
	}
}

// Status returns whether the optional API tester menu module is active.
func (s *Service) Status(ctx context.Context, actorID uint) (bool, error) {
	if _, err := s.requireUser(ctx, actorID); err != nil {
		return false, err
	}
	enabled, err := s.repo.IsEnabled(ctx)
	if err != nil {
		return false, errs.Internal(err)
	}
	return enabled, nil
}

// SetStatus toggles the optional API tester module on or off.
func (s *Service) SetStatus(ctx context.Context, actorID uint, enabled bool, ip string) (bool, error) {
	actor, err := s.requireAny(ctx, actorID, "Bu modülü açıp kapatma yetkin yok.", enums.APITesterManage, enums.SystemSettings)
	if err != nil {
		return false, err
	}
	if err := s.repo.SetEnabled(ctx, enabled); err != nil {
		return false, errs.Internal(err)
	}
	s.recordAudit(ctx, actor.ID, enums.AuditAPITesterToggled, "api_tester", "enabled", ip, map[string]any{"enabled": enabled})
	return enabled, nil
}

// List returns collections visible to the acting user.
func (s *Service) List(ctx context.Context, actorID uint) (*ListResponse, error) {
	actor, err := s.requireAny(ctx, actorID, "API testlerini görüntüleme yetkin yok.", enums.APITesterView, enums.APITesterExecute, enums.APITesterManage)
	if err != nil {
		return nil, err
	}
	enabled, err := s.repo.IsEnabled(ctx)
	if err != nil {
		return nil, errs.Internal(err)
	}
	canManage := actor.Can(enums.APITesterManage)
	canExecute := actor.Can(enums.APITesterExecute) || canManage

	if !enabled && !canManage {
		return &ListResponse{Enabled: false, Collections: []CollectionDTO{}}, nil
	}

	cols, err := s.repo.ListCollections(ctx, !canManage)
	if err != nil {
		return nil, errs.Internal(err)
	}

	out := make([]CollectionDTO, 0, len(cols))
	for _, c := range cols {
		reqs := make([]RequestDTO, 0, len(c.Requests))
		for _, r := range c.Requests {
			reqs = append(reqs, s.toRequestDTO(&r, canManage, canExecute))
		}
		out = append(out, toCollectionDTO(&c, reqs))
	}
	return &ListResponse{Enabled: enabled, Collections: out}, nil
}

// CreateCollection creates a new API test collection.
func (s *Service) CreateCollection(ctx context.Context, actorID uint, in CollectionInput, ip string) (*CollectionDTO, error) {
	actor, err := s.requireAny(ctx, actorID, "API koleksiyonu oluşturma yetkin yok.", enums.APITesterManage)
	if err != nil {
		return nil, err
	}
	name, desc, tag, baseURL, active, err := validateCollectionInput(in, true)
	if err != nil {
		return nil, err
	}
	order, err := s.repo.NextCollectionOrder(ctx)
	if err != nil {
		return nil, errs.Internal(err)
	}
	col := &models.APICollection{
		Name:        name,
		Description: desc,
		ProjectTag:  tag,
		BaseURL:     baseURL,
		SortOrder:   order,
		Active:      active,
		CreatedBy:   &actor.ID,
	}
	if err := s.repo.CreateCollection(ctx, col); err != nil {
		return nil, errs.Internal(err)
	}
	s.recordAudit(ctx, actor.ID, enums.AuditAPICollectionCreated, "api_collection", col.ID, ip, map[string]any{"name": col.Name, "projectTag": col.ProjectTag})
	dto := toCollectionDTO(col, []RequestDTO{})
	return &dto, nil
}

// ImportCollection validates and creates a collection along with its request
// templates in a single transaction.
func (s *Service) ImportCollection(ctx context.Context, actorID uint, in ImportCollectionInput, ip string) (*CollectionDTO, error) {
	actor, err := s.requireAny(ctx, actorID, "API koleksiyonu içe aktarma yetkin yok.", enums.APITesterManage)
	if err != nil {
		return nil, err
	}
	if len(in.Requests) > 200 {
		return nil, errs.Invalid("Tek seferde en fazla 200 istek içe aktarılabilir.", nil)
	}
	name, desc, tag, baseURL, active, err := validateCollectionInput(in.Collection, true)
	if err != nil {
		return nil, err
	}
	order, err := s.repo.NextCollectionOrder(ctx)
	if err != nil {
		return nil, errs.Internal(err)
	}
	col := &models.APICollection{
		Name:        name,
		Description: desc,
		ProjectTag:  tag,
		BaseURL:     baseURL,
		SortOrder:   order,
		Active:      active,
		CreatedBy:   &actor.ID,
	}

	reqModels := make([]*models.APIRequest, 0, len(in.Requests))
	for _, rIn := range in.Requests {
		rm, err := s.buildRequestModel(nil, 0, rIn, actor.ID)
		if err != nil {
			return nil, err
		}
		reqModels = append(reqModels, rm)
	}

	if err := s.repo.ImportCollection(ctx, col, reqModels); err != nil {
		return nil, errs.Internal(err)
	}

	reqDTOs := make([]RequestDTO, 0, len(reqModels))
	for _, rm := range reqModels {
		reqDTOs = append(reqDTOs, s.toRequestDTO(rm, true, true))
	}
	s.recordAudit(ctx, actor.ID, enums.AuditAPICollectionCreated, "api_collection", col.ID, ip, map[string]any{
		"name":         col.Name,
		"projectTag":   col.ProjectTag,
		"requestCount": len(reqModels),
		"imported":     true,
	})
	dto := toCollectionDTO(col, reqDTOs)
	return &dto, nil
}

// UpdateCollection edits an existing collection.
func (s *Service) UpdateCollection(ctx context.Context, actorID uint, id uint, in CollectionInput, ip string) (*CollectionDTO, error) {
	actor, err := s.requireAny(ctx, actorID, "API koleksiyonu düzenleme yetkin yok.", enums.APITesterManage)
	if err != nil {
		return nil, err
	}
	col, err := s.repo.GetCollection(ctx, id)
	if err != nil {
		return nil, errs.Internal(err)
	}
	if col == nil {
		return nil, errs.NotFound("Koleksiyon bulunamadı.")
	}
	name, desc, tag, baseURL, active, err := validateCollectionInput(in, col.Active)
	if err != nil {
		return nil, err
	}
	col.Name = name
	col.Description = desc
	col.ProjectTag = tag
	col.BaseURL = baseURL
	col.Active = active
	if err := s.repo.UpdateCollection(ctx, col); err != nil {
		return nil, errs.Internal(err)
	}
	s.recordAudit(ctx, actor.ID, enums.AuditAPICollectionUpdated, "api_collection", col.ID, ip, map[string]any{"name": col.Name, "active": col.Active})
	dto := toCollectionDTO(col, []RequestDTO{})
	return &dto, nil
}

// DeleteCollection deletes a collection and all its saved requests.
func (s *Service) DeleteCollection(ctx context.Context, actorID uint, id uint, ip string) error {
	actor, err := s.requireAny(ctx, actorID, "API koleksiyonu silme yetkin yok.", enums.APITesterManage)
	if err != nil {
		return err
	}
	col, err := s.repo.GetCollection(ctx, id)
	if err != nil {
		return errs.Internal(err)
	}
	if col == nil {
		return errs.NotFound("Koleksiyon bulunamadı.")
	}
	if err := s.repo.DeleteCollection(ctx, id); err != nil {
		return errs.Internal(err)
	}
	s.recordAudit(ctx, actor.ID, enums.AuditAPICollectionDeleted, "api_collection", id, ip, map[string]any{"name": col.Name})
	return nil
}

// CreateRequest creates a request template inside collectionID.
func (s *Service) CreateRequest(ctx context.Context, actorID uint, collectionID uint, in RequestInput, ip string) (*RequestDTO, error) {
	actor, err := s.requireAny(ctx, actorID, "API isteği oluşturma yetkin yok.", enums.APITesterManage)
	if err != nil {
		return nil, err
	}
	col, err := s.repo.GetCollection(ctx, collectionID)
	if err != nil {
		return nil, errs.Internal(err)
	}
	if col == nil {
		return nil, errs.NotFound("Koleksiyon bulunamadı.")
	}
	req, err := s.buildRequestModel(nil, collectionID, in, actor.ID)
	if err != nil {
		return nil, err
	}
	order, err := s.repo.NextRequestOrder(ctx, collectionID)
	if err != nil {
		return nil, errs.Internal(err)
	}
	req.SortOrder = order
	if err := s.repo.CreateRequest(ctx, req); err != nil {
		return nil, errs.Internal(err)
	}
	s.recordAudit(ctx, actor.ID, enums.AuditAPIRequestCreated, "api_request", req.ID, ip, map[string]any{"collectionId": collectionID, "name": req.Name, "method": req.Method})
	dto := s.toRequestDTO(req, true, true)
	return &dto, nil
}

// UpdateRequest edits an existing request template.
func (s *Service) UpdateRequest(ctx context.Context, actorID uint, id uint, in RequestInput, ip string) (*RequestDTO, error) {
	actor, err := s.requireAny(ctx, actorID, "API isteği düzenleme yetkin yok.", enums.APITesterManage)
	if err != nil {
		return nil, err
	}
	existing, err := s.repo.GetRequest(ctx, id)
	if err != nil {
		return nil, errs.Internal(err)
	}
	if existing == nil {
		return nil, errs.NotFound("İstek şablonu bulunamadı.")
	}
	updated, err := s.buildRequestModel(existing, existing.CollectionID, in, actor.ID)
	if err != nil {
		return nil, err
	}
	if err := s.repo.UpdateRequest(ctx, updated); err != nil {
		return nil, errs.Internal(err)
	}
	s.recordAudit(ctx, actor.ID, enums.AuditAPIRequestUpdated, "api_request", updated.ID, ip, map[string]any{"name": updated.Name, "method": updated.Method, "active": updated.Active})
	dto := s.toRequestDTO(updated, true, true)
	return &dto, nil
}

// DeleteRequest removes a request template.
func (s *Service) DeleteRequest(ctx context.Context, actorID uint, id uint, ip string) error {
	actor, err := s.requireAny(ctx, actorID, "API isteği silme yetkin yok.", enums.APITesterManage)
	if err != nil {
		return err
	}
	existing, err := s.repo.GetRequest(ctx, id)
	if err != nil {
		return errs.Internal(err)
	}
	if existing == nil {
		return errs.NotFound("İstek şablonu bulunamadı.")
	}
	if err := s.repo.DeleteRequest(ctx, id); err != nil {
		return errs.Internal(err)
	}
	s.recordAudit(ctx, actor.ID, enums.AuditAPIRequestDeleted, "api_request", id, ip, map[string]any{"name": existing.Name})
	return nil
}

// ExecuteProxy executes a saved request through Santral-C's SSRF-guarded HTTP client.
func (s *Service) ExecuteProxy(ctx context.Context, actorID uint, reqID uint, in ProxyInput, ip string) (*ProxyResult, error) {
	actor, err := s.requireAny(ctx, actorID, "API isteği çalıştırma yetkin yok.", enums.APITesterExecute, enums.APITesterManage)
	if err != nil {
		return nil, err
	}
	enabled, err := s.repo.IsEnabled(ctx)
	if err != nil {
		return nil, errs.Internal(err)
	}
	if !enabled && !actor.Can(enums.APITesterManage) {
		return nil, errs.Conflict("API test modülü şu anda pasif.", nil)
	}

	reqModel, col, err := s.loadExecutableRequest(ctx, reqID, actor.Can(enums.APITesterManage))
	if err != nil {
		return nil, err
	}
	if reqModel.ExecutionMode != ModeServer && !reqModel.AllowServerFallback {
		return nil, errs.Conflict("Bu istek sadece kullanıcı IP'sinden çalıştırılabilir; sunucu proxysine izin verilmemiş.", nil)
	}

	headers := s.openHeaders(reqModel.HeadersEnc)
	queryParams := decodeKeyValues(reqModel.QueryParamsJSON)
	varsDef := decodeVariables(reqModel.VariablesJSON)

	vars, err := BuildVars(col.BaseURL, varsDef, in.Variables)
	if err != nil {
		return nil, err
	}
	targetURL, err := RenderTarget(reqModel.URLTemplate, queryParams, vars)
	if err != nil {
		return nil, err
	}

	var bodyReader io.Reader
	if reqModel.BodyTemplate != "" && reqModel.Method != "GET" && reqModel.Method != "HEAD" {
		renderedBody := Interpolate(reqModel.BodyTemplate, vars)
		bodyReader = strings.NewReader(renderedBody)
	}

	callCtx, cancel := context.WithTimeout(ctx, maxProxyTimeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(callCtx, reqModel.Method, targetURL, bodyReader)
	if err != nil {
		return nil, errs.Invalid("HTTP isteği oluşturulamadı.", err)
	}

	hasContentType := false
	for _, h := range headers {
		k := strings.TrimSpace(h.Key)
		if k == "" {
			continue
		}
		if strings.EqualFold(k, "Content-Type") {
			hasContentType = true
		}
		httpReq.Header.Add(k, Interpolate(h.Value, vars))
	}
	if !hasContentType && bodyReader != nil && strings.TrimSpace(reqModel.ContentType) != "" {
		httpReq.Header.Set("Content-Type", reqModel.ContentType)
	}
	httpReq.Header.Set("User-Agent", "SantralC-APITester/1.0")

	start := time.Now()
	resp, doErr := s.httpClient.Do(httpReq)
	durationMS := int(time.Since(start).Milliseconds())

	redactedTarget := RedactLogURL(targetURL)
	if doErr != nil {
		msg := friendlyProxyError(doErr)
		s.saveRunLog(ctx, reqModel, col, actor, redactedTarget, LogModeServer, 0, durationMS, msg)
		return &ProxyResult{
			StatusCode:  0,
			DurationMS:  durationMS,
			TargetURL:   targetURL,
			ExecutedVia: LogModeServer,
			Error:       msg,
		}, nil
	}
	defer func() { _ = resp.Body.Close() }()

	limited := io.LimitReader(resp.Body, maxProxyResponseBody+1)
	rawBody, readErr := io.ReadAll(limited)
	if readErr != nil {
		msg := "Yanıt gövdesi okunamadı: " + readErr.Error()
		s.saveRunLog(ctx, reqModel, col, actor, redactedTarget, LogModeServer, resp.StatusCode, durationMS, msg)
		return &ProxyResult{
			StatusCode:  resp.StatusCode,
			StatusText:  resp.Status,
			DurationMS:  durationMS,
			TargetURL:   targetURL,
			ExecutedVia: LogModeServer,
			Error:       msg,
		}, nil
	}
	truncated := len(rawBody) > maxProxyResponseBody
	if truncated {
		rawBody = rawBody[:maxProxyResponseBody]
	}

	respHeaders := make([]KeyValue, 0, len(resp.Header))
	for k, vals := range resp.Header {
		respHeaders = append(respHeaders, KeyValue{Key: k, Value: strings.Join(vals, ", ")})
	}

	s.saveRunLog(ctx, reqModel, col, actor, redactedTarget, LogModeServer, resp.StatusCode, durationMS, "")
	s.recordAudit(ctx, actor.ID, enums.AuditAPIRequestExecuted, "api_request", reqModel.ID, ip, map[string]any{
		"mode":       LogModeServer,
		"statusCode": resp.StatusCode,
		"durationMs": durationMS,
		"targetUrl":  redactedTarget,
	})

	return &ProxyResult{
		StatusCode:  resp.StatusCode,
		StatusText:  resp.Status,
		Headers:     respHeaders,
		Body:        string(rawBody),
		Truncated:   truncated,
		DurationMS:  durationMS,
		TargetURL:   targetURL,
		ExecutedVia: LogModeServer,
	}, nil
}

// LogClientRun records metadata for an execution that happened directly from the
// specialist's browser or SantralC Chrome extension.
func (s *Service) LogClientRun(ctx context.Context, actorID uint, reqID uint, in LogRunInput, ip string) error {
	actor, err := s.requireAny(ctx, actorID, "API isteği çalıştırma yetkin yok.", enums.APITesterExecute, enums.APITesterManage)
	if err != nil {
		return err
	}
	reqModel, col, err := s.loadExecutableRequest(ctx, reqID, actor.Can(enums.APITesterManage))
	if err != nil {
		return err
	}
	mode := strings.ToLower(strings.TrimSpace(in.ExecutionMode))
	if mode != LogModeExtension && mode != LogModeBrowser {
		mode = LogModeBrowser
	}
	method, err := NormalizeMethod(in.Method)
	if err != nil {
		method = reqModel.Method
	}
	dur := in.DurationMS
	if dur < 0 {
		dur = 0
	}
	errMsg := strings.TrimSpace(in.ErrorMessage)
	if len(errMsg) > 500 {
		errMsg = errMsg[:500]
	}
	redactedTarget := RedactLogURL(in.TargetURL)
	reqCopy := *reqModel
	reqCopy.Method = method
	s.saveRunLog(ctx, &reqCopy, col, actor, redactedTarget, mode, in.StatusCode, dur, errMsg)
	s.recordAudit(ctx, actor.ID, enums.AuditAPIRequestExecuted, "api_request", reqModel.ID, ip, map[string]any{
		"mode":       mode,
		"statusCode": in.StatusCode,
		"durationMs": dur,
		"targetUrl":  redactedTarget,
	})
	return nil
}

// ListRunLogs returns recent run logs.
func (s *Service) ListRunLogs(ctx context.Context, actorID uint, requestID *uint, limit int) ([]RunLogDTO, error) {
	if _, err := s.requireAny(ctx, actorID, "API çalışma loglarını görme yetkin yok.", enums.APITesterView, enums.APITesterExecute, enums.APITesterManage); err != nil {
		return nil, err
	}
	rows, err := s.repo.ListRunLogs(ctx, requestID, limit)
	if err != nil {
		return nil, errs.Internal(err)
	}
	out := make([]RunLogDTO, 0, len(rows))
	for _, r := range rows {
		out = append(out, RunLogDTO{
			ID:             r.ID,
			RequestID:      r.RequestID,
			RequestName:    r.RequestName,
			CollectionName: r.CollectionName,
			UserID:         r.UserID,
			UserName:       r.UserName,
			Method:         r.Method,
			TargetURL:      r.TargetURL,
			ExecutionMode:  r.ExecutionMode,
			StatusCode:     r.StatusCode,
			DurationMS:     r.DurationMS,
			ErrorMessage:   r.ErrorMessage,
			CreatedAt:      r.CreatedAt,
		})
	}
	return out, nil
}

func (s *Service) loadExecutableRequest(ctx context.Context, reqID uint, canManage bool) (*models.APIRequest, *models.APICollection, error) {
	reqModel, err := s.repo.GetRequest(ctx, reqID)
	if err != nil {
		return nil, nil, errs.Internal(err)
	}
	if reqModel == nil || (!reqModel.Active && !canManage) {
		return nil, nil, errs.NotFound("API isteği bulunamadı.")
	}
	col, err := s.repo.GetCollection(ctx, reqModel.CollectionID)
	if err != nil {
		return nil, nil, errs.Internal(err)
	}
	if col == nil || (!col.Active && !canManage) {
		return nil, nil, errs.NotFound("Koleksiyon bulunamadı.")
	}
	return reqModel, col, nil
}

func (s *Service) buildRequestModel(existing *models.APIRequest, collectionID uint, in RequestInput, actorID uint) (*models.APIRequest, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return nil, errs.Invalid("İstek adı boş olamaz.", nil)
	}
	if len(name) > 120 {
		return nil, errs.Invalid("İstek adı en fazla 120 karakter olabilir.", nil)
	}
	method, err := NormalizeMethod(in.Method)
	if err != nil {
		return nil, err
	}
	urlTemplate, err := ValidateURLTemplate(in.URLTemplate)
	if err != nil {
		return nil, err
	}
	execMode := strings.ToLower(strings.TrimSpace(in.ExecutionMode))
	if execMode == "" {
		execMode = ModeClient
	}
	if execMode != ModeClient && execMode != ModeServer {
		return nil, errs.Invalid("Geçersiz çalışma modu.", nil)
	}

	var prevHeaders []KeyValue
	if existing != nil {
		prevHeaders = s.openHeaders(existing.HeadersEnc)
	}
	cleanHeaders := sanitizeHeaders(in.Headers, prevHeaders)
	sealedHeaders, err := s.sealHeaders(cleanHeaders)
	if err != nil {
		return nil, errs.Internal(err)
	}

	cleanQuery := sanitizeKeyValues(in.QueryParams, false)
	queryJSON, _ := json.Marshal(cleanQuery)

	cleanVars := sanitizeVariables(in.Variables)
	varsJSON, _ := json.Marshal(cleanVars)

	contentType := strings.TrimSpace(in.ContentType)
	if contentType == "" {
		contentType = "application/json"
	}

	allowFallback := true
	if existing != nil {
		allowFallback = existing.AllowServerFallback
	}
	if in.AllowServerFallback != nil {
		allowFallback = *in.AllowServerFallback
	}

	active := true
	if existing != nil {
		active = existing.Active
	}
	if in.Active != nil {
		active = *in.Active
	}

	desc := strings.TrimSpace(in.Description)
	runbook := strings.TrimSpace(in.Runbook)

	if existing != nil {
		existing.Name = name
		existing.Description = desc
		existing.Method = method
		existing.URLTemplate = urlTemplate
		existing.HeadersEnc = sealedHeaders
		existing.QueryParamsJSON = string(queryJSON)
		existing.BodyTemplate = in.BodyTemplate
		existing.ContentType = contentType
		existing.VariablesJSON = string(varsJSON)
		existing.ExecutionMode = execMode
		existing.AllowServerFallback = allowFallback
		existing.Runbook = runbook
		existing.Active = active
		return existing, nil
	}

	return &models.APIRequest{
		CollectionID:        collectionID,
		Name:                name,
		Description:         desc,
		Method:              method,
		URLTemplate:         urlTemplate,
		HeadersEnc:          sealedHeaders,
		QueryParamsJSON:     string(queryJSON),
		BodyTemplate:        in.BodyTemplate,
		ContentType:         contentType,
		VariablesJSON:       string(varsJSON),
		ExecutionMode:       execMode,
		AllowServerFallback: allowFallback,
		Runbook:             runbook,
		Active:              active,
		CreatedBy:           &actorID,
	}, nil
}

func (s *Service) sealHeaders(headers []KeyValue) (string, error) {
	if len(headers) == 0 {
		return "", nil
	}
	raw, err := json.Marshal(headers)
	if err != nil {
		return "", err
	}
	if s.ring == nil {
		return string(raw), nil
	}
	return s.ring.Seal(sealPurposeHeaders, string(raw))
}

func (s *Service) openHeaders(enc string) []KeyValue {
	if strings.TrimSpace(enc) == "" {
		return []KeyValue{}
	}
	plain := enc
	if s.ring != nil && !strings.HasPrefix(strings.TrimSpace(enc), "[") {
		dec, err := s.ring.Open(sealPurposeHeaders, enc)
		if err != nil {
			slog.Error("api tester headers could not be decrypted", "error", err)
			return []KeyValue{}
		}
		plain = dec
	}
	return decodeKeyValues(plain)
}

func (s *Service) toRequestDTO(r *models.APIRequest, canManage, canExecute bool) RequestDTO {
	headers := s.openHeaders(r.HeadersEnc)
	// Mask secret headers for read-only viewers, and also hide secret values on
	// server-only requests from executors so tokens remain purely backend-side.
	hideSecrets := !canManage && (!canExecute || r.ExecutionMode == ModeServer)
	outHeaders := make([]KeyValue, 0, len(headers))
	for _, h := range headers {
		item := h
		if h.Secret && hideSecrets {
			item.Value = MaskedSecret
		}
		outHeaders = append(outHeaders, item)
	}
	return RequestDTO{
		ID:                  r.ID,
		CollectionID:        r.CollectionID,
		Name:                r.Name,
		Description:         r.Description,
		Method:              r.Method,
		URLTemplate:         r.URLTemplate,
		Headers:             outHeaders,
		QueryParams:         decodeKeyValues(r.QueryParamsJSON),
		BodyTemplate:        r.BodyTemplate,
		ContentType:         r.ContentType,
		Variables:           decodeVariables(r.VariablesJSON),
		ExecutionMode:       r.ExecutionMode,
		AllowServerFallback: r.AllowServerFallback,
		Runbook:             r.Runbook,
		SortOrder:           r.SortOrder,
		Active:              r.Active,
		CreatedAt:           r.CreatedAt,
		UpdatedAt:           r.UpdatedAt,
	}
}

func toCollectionDTO(c *models.APICollection, reqs []RequestDTO) CollectionDTO {
	if reqs == nil {
		reqs = []RequestDTO{}
	}
	return CollectionDTO{
		ID:          c.ID,
		Name:        c.Name,
		Description: c.Description,
		ProjectTag:  c.ProjectTag,
		BaseURL:     c.BaseURL,
		SortOrder:   c.SortOrder,
		Active:      c.Active,
		Requests:    reqs,
		CreatedAt:   c.CreatedAt,
		UpdatedAt:   c.UpdatedAt,
	}
}

func (s *Service) requireUser(ctx context.Context, actorID uint) (*models.User, error) {
	if s.actors == nil || actorID == 0 {
		return nil, errs.Unauthorized("Oturum bulunamadı. Lütfen giriş yap.")
	}
	u, err := s.actors.GetByID(ctx, actorID)
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, errs.Unauthorized("Kullanıcı bulunamadı.")
	}
	return u, nil
}

func (s *Service) requireAny(ctx context.Context, actorID uint, deniedMsg string, perms ...enums.Permission) (*models.User, error) {
	u, err := s.requireUser(ctx, actorID)
	if err != nil {
		return nil, err
	}
	for _, p := range perms {
		if u.Can(p) {
			return u, nil
		}
	}
	return nil, errs.Forbidden(deniedMsg)
}

func (s *Service) saveRunLog(ctx context.Context, reqModel *models.APIRequest, col *models.APICollection, actor *models.User, targetURL, mode string, statusCode, durationMS int, errMsg string) {
	reqID := reqModel.ID
	var uid *uint
	userName := ""
	if actor != nil {
		uid = &actor.ID
		userName = strings.TrimSpace(actor.Name)
		if userName == "" {
			userName = actor.Email
		}
	}
	row := &models.APIRunLog{
		RequestID:      &reqID,
		RequestName:    reqModel.Name,
		CollectionName: col.Name,
		UserID:         uid,
		UserName:       userName,
		Method:         reqModel.Method,
		TargetURL:      targetURL,
		ExecutionMode:  mode,
		StatusCode:     statusCode,
		DurationMS:     durationMS,
		ErrorMessage:   errMsg,
	}
	if err := s.repo.CreateRunLog(ctx, row); err != nil {
		slog.WarnContext(ctx, "api tester run log could not be saved", "error", err)
	}
}

func (s *Service) recordAudit(ctx context.Context, actorID uint, action, targetType string, targetID any, ip string, detail map[string]any) {
	if s.audit == nil {
		return
	}
	s.audit.Record(ctx, audit.Entry{
		ActorID:    &actorID,
		Action:     action,
		TargetType: targetType,
		TargetID:   toStringID(targetID),
		IP:         ip,
		Detail:     detail,
	})
}

func validateCollectionInput(in CollectionInput, defaultActive bool) (name, desc, tag, baseURL string, active bool, err error) {
	name = strings.TrimSpace(in.Name)
	if name == "" {
		return "", "", "", "", false, errs.Invalid("Koleksiyon adı boş olamaz.", nil)
	}
	if len(name) > 120 {
		return "", "", "", "", false, errs.Invalid("Koleksiyon adı en fazla 120 karakter olabilir.", nil)
	}
	tag = strings.TrimSpace(in.ProjectTag)
	if len(tag) > 64 {
		return "", "", "", "", false, errs.Invalid("Proje etiketi en fazla 64 karakter olabilir.", nil)
	}
	baseURL, err = ValidateCollectionBaseURL(in.BaseURL)
	if err != nil {
		return "", "", "", "", false, err
	}
	desc = strings.TrimSpace(in.Description)
	active = defaultActive
	if in.Active != nil {
		active = *in.Active
	}
	return name, desc, tag, baseURL, active, nil
}

func sanitizeHeaders(in, prev []KeyValue) []KeyValue {
	prevByKey := make(map[string]string, len(prev))
	for _, p := range prev {
		prevByKey[strings.ToLower(strings.TrimSpace(p.Key))] = p.Value
	}
	out := make([]KeyValue, 0, len(in))
	for _, kv := range in {
		k := strings.TrimSpace(kv.Key)
		if k == "" {
			continue
		}
		val := kv.Value
		if kv.Secret && val == MaskedSecret {
			if old, ok := prevByKey[strings.ToLower(k)]; ok {
				val = old
			}
		}
		out = append(out, KeyValue{Key: k, Value: val, Secret: kv.Secret})
	}
	return out
}

func sanitizeKeyValues(in []KeyValue, keepSecret bool) []KeyValue {
	out := make([]KeyValue, 0, len(in))
	for _, kv := range in {
		k := strings.TrimSpace(kv.Key)
		if k == "" {
			continue
		}
		out = append(out, KeyValue{Key: k, Value: kv.Value, Secret: keepSecret && kv.Secret})
	}
	return out
}

func sanitizeVariables(in []VariableDef) []VariableDef {
	out := make([]VariableDef, 0, len(in))
	seen := make(map[string]bool, len(in))
	for _, v := range in {
		k := strings.TrimSpace(v.Key)
		if k == "" || seen[k] {
			continue
		}
		seen[k] = true
		src := strings.TrimSpace(v.Source)
		if src != "call_phone" && src != "call_name" {
			src = "manual"
		}
		out = append(out, VariableDef{
			Key:          k,
			Label:        strings.TrimSpace(v.Label),
			DefaultValue: v.DefaultValue,
			Required:     v.Required,
			Source:       src,
			Placeholder:  strings.TrimSpace(v.Placeholder),
		})
	}
	return out
}

func decodeKeyValues(raw string) []KeyValue {
	if strings.TrimSpace(raw) == "" {
		return []KeyValue{}
	}
	var out []KeyValue
	if err := json.Unmarshal([]byte(raw), &out); err != nil || out == nil {
		return []KeyValue{}
	}
	return out
}

func decodeVariables(raw string) []VariableDef {
	if strings.TrimSpace(raw) == "" {
		return []VariableDef{}
	}
	var out []VariableDef
	if err := json.Unmarshal([]byte(raw), &out); err != nil || out == nil {
		return []VariableDef{}
	}
	return out
}

func friendlyProxyError(err error) string {
	if errors.Is(err, outside.ErrInternalAddress) {
		return "Güvenlik politikası gereği iç ağ veya localhost adreslerine sunucu üzerinden istek gönderilemez. Kendi IP'nizden test edin."
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "Hedef sunucu 20 saniye içinde yanıt vermedi (zaman aşımı)."
	}
	return "Sunucu üzerinden istek gönderilemedi: " + err.Error()
}

func toStringID(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case uint:
		if x == 0 {
			return ""
		}
		return formatUint(x)
	default:
		return ""
	}
}

func formatUint(v uint) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}
