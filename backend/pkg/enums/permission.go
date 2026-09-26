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
	ModuleTeams       Module = "teams"
	ModuleGames       Module = "games"
	ModuleWhatsApp    Module = "whatsapp"
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
	// EscalationAuto: the wrap-up card is not shown; a record is written by
	// itself when a call ends (sales support given, or unreached).
	EscalationAuto Permission = "escalation.auto"

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

	TeamsView         Permission = "teams.view"
	TeamsGroupCreate  Permission = "teams.group_create"
	TeamsMemberInvite Permission = "teams.member_invite"
	TeamsMemberAdd    Permission = "teams.member_add"
	TeamsAdmin        Permission = "teams.admin"

	GamesPlay   Permission = "games.play"
	GamesManage Permission = "games.manage"

	// WhatsApp. Seeing: own tickets by default; team and all widen it.
	WAView            Permission = "whatsapp.view"
	WAViewTeam        Permission = "whatsapp.view_team"
	WAViewAll         Permission = "whatsapp.view_all"
	WAReply           Permission = "whatsapp.reply"
	WANote            Permission = "whatsapp.note"
	WAPool            Permission = "whatsapp.pool"
	WAWaiting         Permission = "whatsapp.waiting"
	WATake            Permission = "whatsapp.take"
	WAAssign          Permission = "whatsapp.assign"
	WAResolve         Permission = "whatsapp.resolve"
	WATemplateSend    Permission = "whatsapp.template_send"
	WATemplateManage  Permission = "whatsapp.template_manage"
	WAQuickReply      Permission = "whatsapp.quick_reply_manage"
	WAAutomation      Permission = "whatsapp.automation_manage"
	WABotManage       Permission = "whatsapp.bot_manage"
	WABotPublish      Permission = "whatsapp.bot_publish"
	WAChannelManage   Permission = "whatsapp.channel_manage"
	WASetReadReceipts Permission = "whatsapp.setting_read_receipts"
	WASetGreeting     Permission = "whatsapp.setting_greeting"
	WASetDistribution Permission = "whatsapp.setting_distribution"
	WASetGeneral      Permission = "whatsapp.setting_general"
	WATeamManage      Permission = "whatsapp.team_manage"
	WAContactManage   Permission = "whatsapp.contact_manage"
	WACallbacks       Permission = "whatsapp.callbacks"
	WAReports         Permission = "whatsapp.reports"
	WAExport          Permission = "whatsapp.export"
	WAAISuggest       Permission = "whatsapp.ai_suggest"
	WAAIManage        Permission = "whatsapp.ai_manage"
	WACallSurvey      Permission = "whatsapp.call_survey_manage"

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
	{EscalationAuto, "Çağrı bitince eskalasyon kendiliğinden yazılır (görüşme: satış desteği verildi, açmadıysa: ulaşılamadı); eskalasyon kartı sorulmaz"},
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
	{TeamsView, "Teams sekmesini görür, mesajlaşır"},
	{TeamsGroupCreate, "Teams'te grup oluşturur"},
	{TeamsMemberInvite, "Teams gruplarına davet gönderir"},
	{TeamsMemberAdd, "Teams gruplarına doğrudan üye ekler"},
	{TeamsAdmin, "Tüm Teams gruplarını yönetir"},
	{GamesPlay, "Teams'teki mini oyunlara katılır"},
	{GamesManage, "Mini oyun içeriklerini ve ayarlarını yönetir"},
	{WAView, "WhatsApp gelen kutusunu açar, kendisine atanan sohbetleri görür"},
	{WAViewTeam, "Ekibindeki sohbetleri de görür"},
	{WAViewAll, "Tüm cihazlardaki bütün sohbetleri görür"},
	{WAReply, "Müşteriye mesaj yazar"},
	{WANote, "Sohbete müşterinin görmediği iç not yazar"},
	{WAPool, "Havuzdaki sohbetleri görür ve üstlenir"},
	{WAWaiting, "Cevap Bekleyenler listesini görür ve yardıma katılır"},
	{WATake, "Başkasına atanmış sohbeti kendi üstüne alır"},
	{WAAssign, "Sohbeti başka bir kişiye ya da ekibe aktarır"},
	{WAResolve, "Sohbeti çözüldü olarak kapatır"},
	{WATemplateSend, "Şablonla mesaj gönderir"},
	{WATemplateManage, "Şablon oluşturur, düzenler, siler"},
	{WAQuickReply, "Hazır yanıtları düzenler"},
	{WAAutomation, "Otomatik mesaj kurallarını düzenler"},
	{WABotManage, "Chatbot akışlarını düzenler"},
	{WABotPublish, "Chatbot akışlarını yayına alır"},
	{WAChannelManage, "WhatsApp cihazı ekler, kimlik bilgilerini girer"},
	{WASetReadReceipts, "Müşteriye mavi tik gidip gitmeyeceğini ayarlar"},
	{WASetGreeting, "Karşılama mesajını açar, kapatır ve düzenler"},
	{WASetDistribution, "Otomatik dağıtımı açar, kapatır ve ayarlar"},
	{WASetGeneral, "Mesai saatleri, bekleme süresi ve anket gibi cihaz ayarlarını düzenler"},
	{WATeamManage, "WhatsApp ekiplerini ve cihaz üyelerini düzenler"},
	{WAContactManage, "Müşteri bilgilerini ve etiketlerini düzenler"},
	{WACallbacks, "Geri arama taleplerini görür ve kapatır"},
	{WAReports, "WhatsApp raporlarını görür"},
	{WAExport, "WhatsApp yazışmalarını dışa aktarır"},
	{WAAISuggest, "Yazarken yapay zekâdan cevap önerisi alır"},
	{WAAIManage, "Yapay zekâ ayarlarını ve anahtarını düzenler"},
	{WACallSurvey, "Telefon görüşmesinden sonra WhatsApp'tan giden anketi ayarlar"},
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
	ModuleTeams:       "Teams",
	ModuleGames:       "Mini Oyunlar",
	ModuleWhatsApp:    "WhatsApp",
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
