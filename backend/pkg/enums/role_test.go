package enums

import "testing"

func TestInvisibleAdminHasEveryPermission(t *testing.T) {
	if got, want := len(RolePermissions(RoleInvisibleAdmin)), len(Permissions()); got != want {
		t.Fatalf("invisible_admin has %d permissions, want all %d", got, want)
	}
}

func TestRolePermissionsStayWithinCatalog(t *testing.T) {
	catalog := make(map[Permission]bool)
	for _, p := range Permissions() {
		catalog[p.Key] = true
	}
	for _, r := range []Role{RoleManager, RoleTechnicalTeam, RoleSalesTeam} {
		perms := RolePermissions(r)
		if len(perms) == 0 {
			t.Fatalf("role %q has no permissions", r)
		}
		for _, p := range perms {
			if !catalog[p] {
				t.Fatalf("role %q references unknown permission %q", r, p)
			}
		}
	}
}

func TestUnknownRoleHasNoPermissions(t *testing.T) {
	if RolePermissions(Role("does_not_exist")) != nil {
		t.Fatal("unknown role must return nil permissions")
	}
}

func TestPermissionModule(t *testing.T) {
	cases := map[Permission]Module{
		CallViewOwn:    ModuleCall,
		CDRViewAll:     ModuleCDR,
		SystemSettings: ModuleSystem,
	}
	for p, want := range cases {
		if got := p.Module(); got != want {
			t.Fatalf("%q module = %q, want %q", p, got, want)
		}
	}
}
