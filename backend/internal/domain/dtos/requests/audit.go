package requests

// AuditFilter narrows the audit-trail listing.
type AuditFilter struct {
	Action  string // exact action code or a "module." prefix
	Query   string // matches actor name, actor email or target id
	Page    int
	PerPage int
}
