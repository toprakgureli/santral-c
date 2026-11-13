// Package apitester manages reusable API test collections and templates for
// technical support specialists. Requests default to client-side execution
// from the specialist's own IP (via browser or SantralC Chrome extension) so
// customer IP whitelists reflect the technician's address, with an optional
// SSRF-guarded server proxy fallback.
package apitester

import "time"

// MaskedSecret is returned to callers who should not view a server-only secret.
const MaskedSecret = "••••••••"

// Execution modes for a saved API request.
const (
	ModeClient = "client"
	ModeServer = "server"
)

// Client log modes accepted when reporting a browser or extension run.
const (
	LogModeExtension = "extension"
	LogModeBrowser   = "browser"
	LogModeServer    = "server"
)

// KeyValue represents a header or query parameter entry.
type KeyValue struct {
	Key    string `json:"key"`
	Value  string `json:"value"`
	Secret bool   `json:"secret,omitempty"`
}

// VariableDef describes an input parameter rendered into URL, query, headers or body.
type VariableDef struct {
	Key          string `json:"key"`
	Label        string `json:"label"`
	DefaultValue string `json:"defaultValue"`
	Required     bool   `json:"required"`
	Source       string `json:"source"` // manual | call_phone | call_name
	Placeholder  string `json:"placeholder"`
}

// RequestDTO is the API representation of a saved request template.
type RequestDTO struct {
	ID                  uint          `json:"id"`
	CollectionID        uint          `json:"collectionId"`
	Name                string        `json:"name"`
	Description         string        `json:"description"`
	Method              string        `json:"method"`
	URLTemplate         string        `json:"urlTemplate"`
	Headers             []KeyValue    `json:"headers"`
	QueryParams         []KeyValue    `json:"queryParams"`
	BodyTemplate        string        `json:"bodyTemplate"`
	ContentType         string        `json:"contentType"`
	Variables           []VariableDef `json:"variables"`
	ExecutionMode       string        `json:"executionMode"`
	AllowServerFallback bool          `json:"allowServerFallback"`
	Runbook             string        `json:"runbook"`
	SortOrder           int           `json:"sortOrder"`
	Active              bool          `json:"active"`
	CreatedAt           time.Time     `json:"createdAt"`
	UpdatedAt           time.Time     `json:"updatedAt"`
}

// CollectionDTO groups request templates for one product or project.
type CollectionDTO struct {
	ID          uint         `json:"id"`
	Name        string       `json:"name"`
	Description string       `json:"description"`
	ProjectTag  string       `json:"projectTag"`
	BaseURL     string       `json:"baseUrl"`
	SortOrder   int          `json:"sortOrder"`
	Active      bool         `json:"active"`
	Requests    []RequestDTO `json:"requests"`
	CreatedAt   time.Time    `json:"createdAt"`
	UpdatedAt   time.Time    `json:"updatedAt"`
}

// ListResponse returns module toggle state and visible collections.
type ListResponse struct {
	Enabled     bool            `json:"enabled"`
	Collections []CollectionDTO `json:"collections"`
}

// CollectionInput creates or updates a collection.
type CollectionInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	ProjectTag  string `json:"projectTag"`
	BaseURL     string `json:"baseUrl"`
	Active      *bool  `json:"active"`
}

// RequestInput creates or updates a request template.
type RequestInput struct {
	Name                string        `json:"name"`
	Description         string        `json:"description"`
	Method              string        `json:"method"`
	URLTemplate         string        `json:"urlTemplate"`
	Headers             []KeyValue    `json:"headers"`
	QueryParams         []KeyValue    `json:"queryParams"`
	BodyTemplate        string        `json:"bodyTemplate"`
	ContentType         string        `json:"contentType"`
	Variables           []VariableDef `json:"variables"`
	ExecutionMode       string        `json:"executionMode"`
	AllowServerFallback *bool         `json:"allowServerFallback"`
	Runbook             string        `json:"runbook"`
	Active              *bool         `json:"active"`
}

// ProxyInput carries parameter values for server-side execution.
type ProxyInput struct {
	Variables map[string]string `json:"variables"`
}

// ProxyResult returns the HTTP response captured by the server proxy.
type ProxyResult struct {
	StatusCode  int        `json:"statusCode"`
	StatusText  string     `json:"statusText"`
	Headers     []KeyValue `json:"headers"`
	Body        string     `json:"body"`
	Truncated   bool       `json:"truncated"`
	DurationMS  int        `json:"durationMs"`
	TargetURL   string     `json:"targetUrl"`
	ExecutedVia string     `json:"executedVia"`
	Error       string     `json:"error,omitempty"`
}

// LogRunInput records metadata for a client-side (browser/extension) execution.
type LogRunInput struct {
	Method        string `json:"method"`
	TargetURL     string `json:"targetUrl"`
	ExecutionMode string `json:"executionMode"`
	StatusCode    int    `json:"statusCode"`
	DurationMS    int    `json:"durationMs"`
	ErrorMessage  string `json:"errorMessage"`
}

// RunLogDTO represents one execution audit row.
type RunLogDTO struct {
	ID             uint      `json:"id"`
	RequestID      *uint     `json:"requestId,omitempty"`
	RequestName    string    `json:"requestName"`
	CollectionName string    `json:"collectionName"`
	UserID         *uint     `json:"userId,omitempty"`
	UserName       string    `json:"userName"`
	Method         string    `json:"method"`
	TargetURL      string    `json:"targetUrl"`
	ExecutionMode  string    `json:"executionMode"`
	StatusCode     int       `json:"statusCode"`
	DurationMS     int       `json:"durationMs"`
	ErrorMessage   string    `json:"errorMessage,omitempty"`
	CreatedAt      time.Time `json:"createdAt"`
}
