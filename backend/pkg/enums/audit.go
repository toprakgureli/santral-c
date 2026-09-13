package enums

// Audit action codes.
const (
	AuditUserCreated       string = "user.created"
	AuditUserActivated     string = "user.activated"
	AuditUserDeactivated   string = "user.deactivated"
	AuditUserPasswordReset string = "user.password_reset"
	AuditUserRolesUpdated  string = "user.roles_updated"
	AuditUserUpdated       string = "user.updated"

	AuditRoleCreated string = "role.created"
	AuditRoleUpdated string = "role.updated"
	AuditRoleDeleted string = "role.deleted"

	AuditSettingsUpdated string = "settings.updated"
	AuditIPUnbanned      string = "security.ip_unbanned"

	AuditShiftStarted string = "shift.started"
	AuditShiftEnded   string = "shift.ended"

	AuditContactCreated     string = "contact.created"
	AuditContactUpdated     string = "contact.updated"
	AuditContactDeleted     string = "contact.deleted"
	AuditContactPhoneAdded  string = "contact.phone_added"
	AuditContactPhoneRemove string = "contact.phone_removed"
)
