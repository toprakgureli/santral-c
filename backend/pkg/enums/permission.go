package enums

import "strings"

// Permission is a fine-grained capability key in module.action form.
type Permission string

// Module groups related permissions.
type Module string

// Permission modules.
const (
	ModuleCall        Module = "call"
	ModuleCDR         Module = "cdr"
	ModuleAgent       Module = "agent"
	ModuleQueue       Module = "queue"
	ModuleContact     Module = "contact"
	ModuleEscalation  Module = "escalation"
	ModuleUser        Module = "user"
	ModuleRole        Module = "role"
	ModuleQuality     Module = "quality"
	ModulePerformance Module = "performance"
	ModuleSystem      Module = "system"
)

// Permission keys.
const (
	CallViewOwn      Permission = "call.view_own"
	CallViewAll      Permission = "call.view_all"
	CallOriginate    Permission = "call.originate"
	CallTransfer     Permission = "call.transfer"
	CallHangup       Permission = "call.hangup"
	CallRecordAccess Permission = "call.record_access"

	CDRViewOwn Permission = "cdr.view_own"
	CDRViewAll Permission = "cdr.view_all"
	CDRExport  Permission = "cdr.export"

	AgentView            Permission = "agent.view"
	AgentManage          Permission = "agent.manage"
	AgentPresenceViewAll Permission = "agent.presence_view_all"
	AgentBreakLimit      Permission = "agent.break_limit"

	QueueView   Permission = "queue.view"
	QueueManage Permission = "queue.manage"

	ContactView   Permission = "contact.view"
	ContactManage Permission = "contact.manage"

	EscalationView    Permission = "escalation.view"
	EscalationSearch  Permission = "escalation.search"
	EscalationManage  Permission = "escalation.manage"
	EscalationListOwn Permission = "escalation.list_own"
	EscalationListAll Permission = "escalation.list_all"

	UserView       Permission = "user.view"
	UserCreate     Permission = "user.create"
	UserUpdate     Permission = "user.update"
	UserDeactivate Permission = "user.deactivate"

	RoleView   Permission = "role.view"
	RoleManage Permission = "role.manage"
	RoleAssign Permission = "role.assign"

	QualityView Permission = "quality.view"

	PerformanceViewRole Permission = "performance.view_role"
	PerformanceViewAll  Permission = "performance.view_all"

	SystemSettings  Permission = "system.settings"
	SystemLogs      Permission = "system.logs"
	SystemAuditView Permission = "system.audit_view"
)

// PermissionInfo describes a permission for seeding.
type PermissionInfo struct {
	Key         Permission
	Description string
}

var permissions = []PermissionInfo{
	{CallViewOwn, "Kendi çağrılarını görüntüler"},
	{CallViewAll, "Tüm çağrıları görüntüler"},
	{CallOriginate, "Giden çağrı başlatır"},
	{CallTransfer, "Çağrı aktarır"},
	{CallHangup, "Çağrı sonlandırır"},
	{CallRecordAccess, "Çağrı kayıtlarına erişir"},
	{CDRViewOwn, "Kendi çağrı kayıtlarını (CDR) görür"},
	{CDRViewAll, "Tüm CDR kayıtlarını görür"},
	{CDRExport, "CDR dışa aktarır"},
	{AgentView, "Temsilci listesini görür"},
	{AgentManage, "Temsilcileri yönetir"},
	{AgentPresenceViewAll, "Tüm temsilcilerin canlı durumunu görür"},
	{AgentBreakLimit, "Günlük mola sınırını belirler"},
	{QueueView, "Kuyrukları görür"},
	{QueueManage, "Kuyrukları yönetir"},
	{ContactView, "Kişileri görür"},
	{ContactManage, "Kişileri yönetir"},
	{EscalationView, "Eskalasyon kayıtlarını görür ve oluşturur"},
	{EscalationSearch, "Müşteriye göre eskalasyon geçmişini arar"},
	{EscalationManage, "Eskalasyon durum kataloğunu yönetir"},
	{EscalationListOwn, "Kendi eskalasyon kayıtlarını listeler"},
	{EscalationListAll, "Tüm eskalasyon kayıtlarını listeler"},
	{UserView, "Kullanıcıları görür"},
	{UserCreate, "Kullanıcı oluşturur"},
	{UserUpdate, "Kullanıcı günceller"},
	{UserDeactivate, "Kullanıcı pasifleştirir"},
	{RoleView, "Rolleri görür"},
	{RoleManage, "Rolleri yönetir"},
	{RoleAssign, "Rol ve izin atar"},
	{QualityView, "Çağrı kalite metriklerini görür"},
	{PerformanceViewRole, "Ekip performansında kendi rolündekileri görür"},
	{PerformanceViewAll, "Ekip performansında herkesi görür"},
	{SystemSettings, "Sistem ayarlarını değiştirir"},
	{SystemLogs, "Sistem loglarını görür"},
	{SystemAuditView, "Denetim (audit) kayıtlarını görür"},
}

// Permissions returns a copy of the full permission catalog.
func Permissions() []PermissionInfo {
	out := make([]PermissionInfo, len(permissions))
	copy(out, permissions)
	return out
}

// moduleLabels are the human-facing group titles for the role editor.
var moduleLabels = map[Module]string{
	ModuleCall:        "Çağrı",
	ModuleCDR:         "Çağrı Kayıtları",
	ModuleAgent:       "Temsilci",
	ModuleQueue:       "Kuyruk",
	ModuleContact:     "Kişiler",
	ModuleEscalation:  "Eskalasyon",
	ModuleUser:        "Kullanıcı",
	ModuleRole:        "Rol",
	ModuleQuality:     "Kalite",
	ModulePerformance: "Ekip Performansı",
	ModuleSystem:      "Sistem",
}

// ModuleLabel returns the human-facing title for a module.
func ModuleLabel(m Module) string {
	if label, ok := moduleLabels[m]; ok {
		return label
	}
	return string(m)
}

// Module returns the module a permission belongs to.
func (p Permission) Module() Module {
	if i := strings.IndexByte(string(p), '.'); i > 0 {
		return Module(string(p)[:i])
	}
	return ""
}
