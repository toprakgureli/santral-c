package enums

// Role is a named bundle of permissions.
type Role string

// System roles.
const (
	RoleInvisibleAdmin Role = "invisible_admin"
	RoleManager        Role = "manager"
	RoleTechnicalTeam  Role = "technical_team"
	RoleSalesTeam      Role = "sales_team"
)

// RoleInfo describes a role for seeding.
type RoleInfo struct {
	Name        Role
	DisplayName string
	Description string
}

var roles = []RoleInfo{
	{RoleInvisibleAdmin, "Görünmez Yönetici", "Tüm izinlere sahip, listelerde gizli sahip hesabı"},
	{RoleManager, "Yönetici", "Kullanıcı, kuyruk ve temsilci yönetimi ile tam çağrı görünürlüğü"},
	{RoleTechnicalTeam, "Teknik Ekip", "Teknik çağrılar, kalite ve log görünürlüğü"},
	{RoleSalesTeam, "Satış Ekibi", "Satış çağrıları, kendi CDR'ı ve kişiler"},
}

// Roles returns a copy of the system role catalog.
func Roles() []RoleInfo {
	out := make([]RoleInfo, len(roles))
	copy(out, roles)
	return out
}

// RolePermissions returns the permissions granted by a role.
func RolePermissions(r Role) []Permission {
	switch r {
	case RoleInvisibleAdmin:
		return allPermissions()
	case RoleManager:
		return managerPermissions()
	case RoleTechnicalTeam:
		return technicalTeamPermissions()
	case RoleSalesTeam:
		return salesTeamPermissions()
	}
	return nil
}

func allPermissions() []Permission {
	out := make([]Permission, 0, len(permissions))
	for _, p := range permissions {
		out = append(out, p.Key)
	}
	return out
}

func managerPermissions() []Permission {
	return []Permission{
		CallViewAll, CallOriginate, CallTransfer, CallHangup, CallRecordAccess,
		CDRViewAll, CDRExport, QualityView,
		AgentView, AgentManage, AgentPresenceViewAll,
		QueueView, QueueManage,
		ContactView, ContactManage,
		EscalationView, EscalationSearch, EscalationManage,
		UserView, UserCreate, UserUpdate, UserDeactivate,
		RoleView, RoleManage, RoleAssign,
		SystemLogs, SystemAuditView,
	}
}

func technicalTeamPermissions() []Permission {
	return []Permission{
		CallViewOwn, CallViewAll, CallTransfer, CallHangup,
		CDRViewOwn, CDRViewAll, QualityView,
		ContactView, EscalationView, EscalationSearch, AgentView, AgentPresenceViewAll, SystemLogs,
	}
}

func salesTeamPermissions() []Permission {
	return []Permission{
		CallViewOwn, CallOriginate, CallTransfer, CallHangup,
		CDRViewOwn, ContactView, ContactManage, EscalationView, AgentView,
	}
}
