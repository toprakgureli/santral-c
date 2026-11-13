package models

import "time"

// APICollection groups related API test request templates (for example by
// product or customer project).
type APICollection struct {
	ID          uint         `gorm:"column:id;primarykey"`
	Name        string       `gorm:"column:name;size:120;not null"`
	Description string       `gorm:"column:description;type:text;not null;default:''"`
	ProjectTag  string       `gorm:"column:project_tag;size:64;not null;default:''"`
	BaseURL     string       `gorm:"column:base_url;size:500;not null;default:''"`
	SortOrder   int          `gorm:"column:sort_order;not null;default:0"`
	Active      bool         `gorm:"column:active;not null;default:true"`
	CreatedBy   *uint        `gorm:"column:created_by"`
	Requests    []APIRequest `gorm:"foreignKey:CollectionID"`
	CreatedAt   time.Time    `gorm:"column:created_at"`
	UpdatedAt   time.Time    `gorm:"column:updated_at"`
}

// TableName pins the table name.
func (APICollection) TableName() string { return "api_collections" }

// APIRequest defines a saved HTTP request template inside an APICollection.
// HeadersEnc holds the AES-GCM sealed JSON header list when stored at rest.
type APIRequest struct {
	ID                  uint      `gorm:"column:id;primarykey"`
	CollectionID        uint      `gorm:"column:collection_id;not null;index"`
	Name                string    `gorm:"column:name;size:120;not null"`
	Description         string    `gorm:"column:description;type:text;not null;default:''"`
	Method              string    `gorm:"column:method;size:12;not null;default:GET"`
	URLTemplate         string    `gorm:"column:url_template;type:text;not null"`
	HeadersEnc          string    `gorm:"column:headers_enc;type:text;not null;default:''"`
	QueryParamsJSON     string    `gorm:"column:query_params_json;type:text;not null;default:'[]'"`
	BodyTemplate        string    `gorm:"column:body_template;type:text;not null;default:''"`
	ContentType         string    `gorm:"column:content_type;size:120;not null;default:'application/json'"`
	VariablesJSON       string    `gorm:"column:variables_json;type:text;not null;default:'[]'"`
	ExecutionMode       string    `gorm:"column:execution_mode;size:20;not null;default:client"`
	AllowServerFallback bool      `gorm:"column:allow_server_fallback;not null;default:true"`
	Runbook             string    `gorm:"column:runbook;type:text;not null;default:''"`
	SortOrder           int       `gorm:"column:sort_order;not null;default:0"`
	Active              bool      `gorm:"column:active;not null;default:true"`
	CreatedBy           *uint     `gorm:"column:created_by"`
	CreatedAt           time.Time `gorm:"column:created_at"`
	UpdatedAt           time.Time `gorm:"column:updated_at"`
}

// TableName pins the table name.
func (APIRequest) TableName() string { return "api_requests" }

// APIRunLog records when and how a specialist executed a saved API test request.
// Secret headers and response bodies are intentionally never stored here.
type APIRunLog struct {
	ID             uint      `gorm:"column:id;primarykey"`
	RequestID      *uint     `gorm:"column:request_id;index"`
	RequestName    string    `gorm:"column:request_name;size:120;not null;default:''"`
	CollectionName string    `gorm:"column:collection_name;size:120;not null;default:''"`
	UserID         *uint     `gorm:"column:user_id"`
	UserName       string    `gorm:"column:user_name;size:120;not null;default:''"`
	Method         string    `gorm:"column:method;size:12;not null"`
	TargetURL      string    `gorm:"column:target_url;type:text;not null;default:''"`
	ExecutionMode  string    `gorm:"column:execution_mode;size:24;not null;default:client"`
	StatusCode     int       `gorm:"column:status_code;not null;default:0"`
	DurationMS     int       `gorm:"column:duration_ms;not null;default:0"`
	ErrorMessage   string    `gorm:"column:error_message;type:text;not null;default:''"`
	CreatedAt      time.Time `gorm:"column:created_at"`
}

// TableName pins the table name.
func (APIRunLog) TableName() string { return "api_run_logs" }
