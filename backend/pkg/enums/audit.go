package enums

// Audit action codes.
const (
	AuditUserCreated       string = "user.created"
	AuditUserActivated     string = "user.activated"
	AuditUserDeactivated   string = "user.deactivated"
	AuditUserPasswordReset string = "user.password_reset"
	AuditUserRolesUpdated  string = "user.roles_updated"

	AuditContactCreated     string = "contact.created"
	AuditContactUpdated     string = "contact.updated"
	AuditContactDeleted     string = "contact.deleted"
	AuditContactPhoneAdded  string = "contact.phone_added"
	AuditContactPhoneRemove string = "contact.phone_removed"
)
