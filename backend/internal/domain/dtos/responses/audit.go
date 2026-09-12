package responses

import (
	"encoding/json"
	"time"
)

// AuditItem is one recorded privileged action with its actor resolved.
type AuditItem struct {
	ID         uint            `json:"id"`
	ActorID    *uint           `json:"actorId,omitempty"`
	ActorName  string          `json:"actorName"`
	ActorEmail string          `json:"actorEmail"`
	Action     string          `json:"action"`
	TargetType string          `json:"targetType"`
	TargetID   string          `json:"targetId"`
	IP         string          `json:"ip"`
	Detail     json.RawMessage `json:"detail"`
	CreatedAt  time.Time       `json:"createdAt"`
}

// AuditList is a page of audit entries.
type AuditList struct {
	Items   []AuditItem `json:"items"`
	Total   int64       `json:"total"`
	Page    int         `json:"page"`
	PerPage int         `json:"perPage"`
}
