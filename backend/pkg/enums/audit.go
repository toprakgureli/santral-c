package enums

// Audit action codes.
const (
	AuditUserCreated       string = "user.created"
	AuditUserActivated     string = "user.activated"
	AuditUserDeactivated   string = "user.deactivated"
	AuditUserPasswordReset string = "user.password_reset"
	AuditUserRolesUpdated  string = "user.roles_updated"
	AuditUserUpdated       string = "user.updated"
	AuditUserSIPUpdated    string = "user.sip_updated"
	AuditSIPSyncedAll      string = "pbx.sip_synced_all"

	AuditCallRecordingOpened string = "call.recording_opened"
	AuditCallRecordingDenied string = "call.recording_denied"
	AuditCallTransferredOut  string = "call.transferred_out"

	AuditRoleCreated string = "role.created"
	AuditRoleUpdated string = "role.updated"
	AuditRoleDeleted string = "role.deleted"

	AuditSettingsUpdated   string = "settings.updated"
	AuditDriveConnected    string = "drive.connected"
	AuditDriveDisconnected string = "drive.disconnected"
	AuditIPUnbanned        string = "security.ip_unbanned"

	AuditShiftStarted string = "shift.started"
	AuditShiftEnded   string = "shift.ended"

	AuditContactCreated     string = "contact.created"
	AuditContactUpdated     string = "contact.updated"
	AuditContactDeleted     string = "contact.deleted"
	AuditContactPhoneAdded  string = "contact.phone_added"
	AuditContactPhoneRemove string = "contact.phone_removed"

	AuditWAChannelDeactivated string = "whatsapp.channel_deactivated"
	AuditWAChannelDeleted     string = "whatsapp.channel_deleted"
	AuditWABotDeactivated     string = "whatsapp.bot_deactivated"
	AuditWABotDeleted         string = "whatsapp.bot_deleted"
)
