package user

import (
	"context"
	"testing"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
)

func withPerms(id uint, roleName string, keys ...enums.Permission) *models.User {
	perms := make([]models.Permission, 0, len(keys))
	for i, k := range keys {
		perms = append(perms, models.Permission{ID: uint(i + 1), Key: string(k)})
	}
	return &models.User{ID: id, Roles: []models.Role{{ID: id, Name: roleName, Permissions: perms}}}
}

func TestEnsureNotAbove(t *testing.T) {
	manager := withPerms(1, "manager", enums.UserView, enums.UserUpdate)
	agent := withPerms(2, "agent", enums.UserView)
	admin := withPerms(3, "admin", enums.UserView, enums.UserUpdate, enums.RoleAssign)
	owner := withPerms(4, string(enums.RoleInvisibleAdmin))

	tests := []struct {
		name    string
		actor   *models.User
		target  *models.User
		wantErr bool
	}{
		{"lower target", manager, agent, false},
		{"same rights", manager, withPerms(5, "manager", enums.UserView, enums.UserUpdate), false},
		{"higher target", manager, admin, true},
		{"invisible admin changes anyone", owner, admin, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ensureNotAbove(tt.actor, tt.target)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ensureNotAbove() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRolesForRefusesOwnRoles(t *testing.T) {
	s := &Service{}
	actor := withPerms(1, "admin", enums.RoleAssign)
	if _, err := s.rolesFor(context.Background(), actor, actor, []uint{1}); err == nil {
		t.Fatal("rolesFor() allowed a user to change their own roles")
	}
}
