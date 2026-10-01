package main

// Account security: the second sign-in step (setting it up, using it, a
// code used twice, too many wrong codes), the forced password change on
// first sign-in, changing one's own password, signing out everywhere,
// giving roles and editing roles within one's own rights, and switching a
// person off, which ends their sessions and their live stream at once.

import (
	"bytes"
	"fmt"
	"io"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"github.com/toprakgureli/santral-c/backend/configs"
	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/internal/sse"
	"github.com/toprakgureli/santral-c/backend/pkg/crypt"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
	"github.com/toprakgureli/santral-c/backend/pkg/totp"
)

// setMFAMode switches the second step on or off for the whole panel and
// puts the old value back when the test ends.
func setMFAMode(t *testing.T, db *gorm.DB, mode string) {
	t.Helper()
	var old []string
	db.Raw("SELECT value FROM system_settings WHERE key = 'mfa_mode'").Scan(&old)
	if err := db.Exec(`INSERT INTO system_settings (key, value, updated_at) VALUES ('mfa_mode', ?, now())
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`, mode).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if len(old) == 0 {
			db.Exec("DELETE FROM system_settings WHERE key = 'mfa_mode'")
			return
		}
		db.Exec("UPDATE system_settings SET value = ? WHERE key = 'mfa_mode'", old[0])
	})
}

// codeAt is the authenticator's code for secret, offset steps from now.
func codeAt(t *testing.T, secret string, offset time.Duration) string {
	t.Helper()
	c, err := totp.Code(secret, time.Now().Add(offset))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// wrongCode is a code that no step around now accepts.
func wrongCode(t *testing.T, secret string) string {
	t.Helper()
	valid := map[string]bool{}
	for _, d := range []time.Duration{-time.Minute, -30 * time.Second, 0, 30 * time.Second, time.Minute} {
		valid[codeAt(t, secret, d)] = true
	}
	for n := 0; ; n++ {
		if c := fmt.Sprintf("%06d", n*7919%1_000_000); !valid[c] {
			return c
		}
	}
}

// withMFA turns the second step on for u with a fresh secret, as if they
// had set it up, and returns the secret.
func withMFA(t *testing.T, db *gorm.DB, u models.User) string {
	t.Helper()
	key, err := totp.Generate("santral-test", u.Email)
	if err != nil {
		t.Fatal(err)
	}
	enc, err := crypt.Encrypt(configs.Cnf.Security.MFAKey, key.Secret)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("UPDATE users SET mfa_secret = ?, mfa_enabled = true, mfa_exempt = false WHERE id = ?", enc, u.ID).Error; err != nil {
		t.Fatal(err)
	}
	return key.Secret
}

type challenge struct {
	MFARequired            bool   `json:"mfaRequired"`
	MFASetupRequired       bool   `json:"mfaSetupRequired"`
	MFAToken               string `json:"mfaToken"`
	PasswordChangeRequired bool   `json:"passwordChangeRequired"`
	PasswordToken          string `json:"passwordToken"`
}

// signedIn reports whether the browser's session opens the panel.
func signedIn(b *browser) bool {
	return b.do(fiber.MethodGet, "/api/v1/auth/me", nil).status == fiber.StatusOK
}

func TestMFASetupAndSignIn(t *testing.T) {
	srv, db := testServer(t, officeSecurity)
	u := seedPeople(t, db, 1, enums.RoleSalesTeam, false)[0]
	b := newBrowser(t, srv.app, nil, u)
	b.mustSignIn()

	if a := b.do(fiber.MethodPost, "/api/v1/auth/mfa/enable", map[string]string{"code": "123456"}); a.status != fiber.StatusBadRequest {
		t.Errorf("turning the second step on before setting it up answered %d, want 400", a.status)
	}
	a := b.do(fiber.MethodPost, "/api/v1/auth/mfa/setup", nil)
	if a.status != fiber.StatusOK {
		t.Fatalf("setup answered %d %s", a.status, a.body)
	}
	var setup struct {
		Secret string `json:"secret"`
		URL    string `json:"url"`
		QR     string `json:"qr"`
	}
	a.json(t, &setup)
	if setup.Secret == "" || !strings.HasPrefix(setup.URL, "otpauth://") || !strings.HasPrefix(setup.QR, "data:image/png;base64,") {
		t.Fatalf("setup returned %+v", setup)
	}
	var stored models.User
	db.First(&stored, u.ID)
	if stored.MFAEnabled || stored.MFASecret == nil || *stored.MFASecret == setup.Secret {
		t.Fatalf("after setup: enabled=%v, secret stored in the clear=%v", stored.MFAEnabled, stored.MFASecret != nil && *stored.MFASecret == setup.Secret)
	}
	if a := b.do(fiber.MethodPost, "/api/v1/auth/mfa/enable", map[string]string{"code": wrongCode(t, setup.Secret)}); a.status != fiber.StatusBadRequest {
		t.Errorf("a wrong code turned the second step on: %d", a.status)
	}
	if a := b.do(fiber.MethodPost, "/api/v1/auth/mfa/enable", map[string]string{"code": "12ab56"}); a.status != fiber.StatusBadRequest {
		t.Errorf("a code with letters answered %d, want 400", a.status)
	}
	enableCode := codeAt(t, setup.Secret, 0)
	if a := b.do(fiber.MethodPost, "/api/v1/auth/mfa/enable", map[string]string{"code": enableCode}); a.status != fiber.StatusNoContent {
		t.Fatalf("the right code answered %d %s", a.status, a.body)
	}
	db.First(&stored, u.ID)
	if !stored.MFAEnabled {
		t.Fatal("the second step is not on after the right code")
	}
	if a := b.do(fiber.MethodPost, "/api/v1/auth/mfa/setup", nil); a.status != fiber.StatusConflict {
		t.Errorf("setting it up again answered %d, want 409", a.status)
	}

	// With the second step asked for, a password alone opens nothing.
	setMFAMode(t, db, "on")
	login := func() challenge {
		t.Helper()
		nb := newBrowser(t, srv.app, nil, u)
		a := nb.login(loadPassword)
		if a.status != fiber.StatusOK {
			t.Fatalf("sign in answered %d %s", a.status, a.body)
		}
		var c challenge
		a.json(t, &c)
		if !c.MFARequired || c.MFAToken == "" {
			t.Fatalf("sign in did not ask for the code: %s", a.body)
		}
		if signedIn(nb) {
			t.Fatal("the password alone opened a session")
		}
		return c
	}
	verify := func(nb *browser, token, code string) answer {
		return nb.do(fiber.MethodPost, "/api/v1/auth/mfa/verify", map[string]string{"token": token, "code": code})
	}

	// The code that turned the step on cannot be used again.
	c := login()
	nb := newBrowser(t, srv.app, nil, u)
	if a := verify(nb, c.MFAToken, enableCode); a.status != fiber.StatusUnauthorized || !strings.Contains(string(a.body), "az önce kullanıldı") {
		t.Errorf("a code used a moment ago answered %d %s, want 401 saying it was used", a.status, a.body)
	}
	next := codeAt(t, setup.Secret, 30*time.Second)
	if a := verify(nb, c.MFAToken, next); a.status != fiber.StatusOK {
		t.Fatalf("a fresh code answered %d %s", a.status, a.body)
	}
	if !signedIn(nb) {
		t.Fatal("the right code did not open a session")
	}
	// The step's token is single use, and so is the code.
	if a := verify(newBrowser(t, srv.app, nil, u), c.MFAToken, codeAt(t, setup.Secret, -30*time.Second)); a.status != fiber.StatusUnauthorized {
		t.Errorf("the same step token worked twice: %d", a.status)
	}
	c2 := login()
	if a := verify(newBrowser(t, srv.app, nil, u), c2.MFAToken, next); a.status != fiber.StatusUnauthorized {
		t.Errorf("a code used for one sign-in opened another: %d", a.status)
	}
	// Another step's token does not stand in for this one.
	if a := verify(newBrowser(t, srv.app, nil, u), "not-a-token", codeAt(t, setup.Secret, -30*time.Second)); a.status != fiber.StatusUnauthorized {
		t.Errorf("a made-up step token answered %d, want 401", a.status)
	}
}

func TestMFAWrongCodeLimit(t *testing.T) {
	srv, db := testServer(t, officeSecurity)
	setMFAMode(t, db, "on")
	u := seedPeople(t, db, 1, enums.RoleSalesTeam, false)[0]
	secret := withMFA(t, db, u)

	b := newBrowser(t, srv.app, nil, u)
	var c challenge
	b.login(loadPassword).json(t, &c)
	if c.MFAToken == "" {
		t.Fatal("no step token")
	}
	bad := wrongCode(t, secret)
	for i := 1; i <= 5; i++ {
		a := b.do(fiber.MethodPost, "/api/v1/auth/mfa/verify", map[string]string{"token": c.MFAToken, "code": bad})
		if a.status != fiber.StatusUnauthorized {
			t.Fatalf("wrong code %d answered %d", i, a.status)
		}
		if i == 5 && !strings.Contains(string(a.body), "yeniden giriş yap") {
			t.Errorf("the fifth wrong code answered %s, want the start-over message", a.body)
		}
	}
	// The step is over: even the right code does not open it now.
	if a := b.do(fiber.MethodPost, "/api/v1/auth/mfa/verify", map[string]string{"token": c.MFAToken, "code": codeAt(t, secret, 0)}); a.status != fiber.StatusUnauthorized {
		t.Errorf("after five wrong codes the right one answered %d, want 401", a.status)
	}
	if signedIn(b) {
		t.Fatal("signed in after five wrong codes")
	}
	// Starting over with the password works, and the account is not locked.
	var again challenge
	b.login(loadPassword).json(t, &again)
	if again.MFAToken == "" || again.MFAToken == c.MFAToken {
		t.Fatalf("starting over gave no new step: %+v", again)
	}
	if a := b.do(fiber.MethodPost, "/api/v1/auth/mfa/verify", map[string]string{"token": again.MFAToken, "code": codeAt(t, secret, 0)}); a.status != fiber.StatusOK {
		t.Fatalf("the right code after starting over answered %d %s", a.status, a.body)
	}
	if !signedIn(b) {
		t.Fatal("not signed in after the right code")
	}
}

func TestMFAForcedEnrollment(t *testing.T) {
	srv, db := testServer(t, officeSecurity)
	setMFAMode(t, db, "on")
	u := seedPeople(t, db, 1, enums.RoleSalesTeam, false)[0]
	db.Exec("UPDATE users SET mfa_exempt = false WHERE id = ?", u.ID)

	b := newBrowser(t, srv.app, nil, u)
	var c challenge
	b.login(loadPassword).json(t, &c)
	if !c.MFASetupRequired || c.MFAToken == "" {
		t.Fatalf("sign in without the second step set up did not ask to set it up: %+v", c)
	}
	if signedIn(b) {
		t.Fatal("the password alone opened a session")
	}
	// The setup token is not a sign-in step token.
	if a := b.do(fiber.MethodPost, "/api/v1/auth/mfa/verify", map[string]string{"token": c.MFAToken, "code": "123456"}); a.status != fiber.StatusUnauthorized {
		t.Errorf("the setup token passed as a sign-in step: %d", a.status)
	}
	a := b.do(fiber.MethodPost, "/api/v1/auth/mfa/enroll", map[string]string{"token": c.MFAToken})
	if a.status != fiber.StatusOK {
		t.Fatalf("enroll answered %d %s", a.status, a.body)
	}
	var setup struct {
		Secret string `json:"secret"`
	}
	a.json(t, &setup)
	if a := b.do(fiber.MethodPost, "/api/v1/auth/mfa/enroll/verify", map[string]string{"token": c.MFAToken, "code": wrongCode(t, setup.Secret)}); a.status != fiber.StatusUnauthorized {
		t.Errorf("a wrong code finished the setup: %d", a.status)
	}
	if a := b.do(fiber.MethodPost, "/api/v1/auth/mfa/enroll/verify", map[string]string{"token": c.MFAToken, "code": codeAt(t, setup.Secret, 0)}); a.status != fiber.StatusOK {
		t.Fatalf("the right code answered %d %s", a.status, a.body)
	}
	if !signedIn(b) {
		t.Fatal("finishing the setup did not sign in")
	}
	var stored models.User
	db.First(&stored, u.ID)
	if !stored.MFAEnabled {
		t.Error("the second step is not on after the forced setup")
	}
	// The setup token is spent.
	if a := b.do(fiber.MethodPost, "/api/v1/auth/mfa/enroll", map[string]string{"token": c.MFAToken}); a.status != fiber.StatusUnauthorized && a.status != fiber.StatusForbidden {
		t.Errorf("the setup token worked again: %d", a.status)
	}
	// From now on the sign-in asks for the code.
	var next challenge
	newBrowser(t, srv.app, nil, u).login(loadPassword).json(t, &next)
	if !next.MFARequired {
		t.Errorf("the next sign-in did not ask for the code: %+v", next)
	}
}

func TestForcedPasswordChange(t *testing.T) {
	srv, db := testServer(t, officeSecurity)
	u := seedPeople(t, db, 1, enums.RoleSalesTeam, false)[0]
	db.Exec("UPDATE users SET must_change_password = true WHERE id = ?", u.ID)

	b := newBrowser(t, srv.app, nil, u)
	var c challenge
	b.login(loadPassword).json(t, &c)
	if !c.PasswordChangeRequired || c.PasswordToken == "" {
		t.Fatalf("the first sign-in did not ask for a new password: %+v", c)
	}
	if signedIn(b) {
		t.Fatal("the first sign-in opened a session before the password was changed")
	}
	change := func(token, pw string) answer {
		return b.do(fiber.MethodPost, "/api/v1/auth/password/change", map[string]string{"token": token, "password": pw})
	}
	if a := change(c.PasswordToken, loadPassword); a.status != fiber.StatusBadRequest {
		t.Errorf("keeping the old password answered %d, want 400", a.status)
	}
	if a := change(c.PasswordToken, "weakpassword"); a.status != fiber.StatusBadRequest {
		t.Errorf("a password breaking the rules answered %d, want 400", a.status)
	}
	if a := change("not-a-token", "New-Pass-2026!"); a.status != fiber.StatusUnauthorized {
		t.Errorf("a made-up token answered %d, want 401", a.status)
	}
	if a := change(c.PasswordToken, "New-Pass-2026!"); a.status != fiber.StatusOK {
		t.Fatalf("the new password answered %d %s", a.status, a.body)
	}
	if !signedIn(b) {
		t.Fatal("changing the password did not sign in")
	}
	if a := change(c.PasswordToken, "Other-Pass-2026!"); a.status != fiber.StatusUnauthorized {
		t.Errorf("the password token worked twice: %d", a.status)
	}
	var stored models.User
	db.First(&stored, u.ID)
	if stored.MustChangePassword {
		t.Error("the account still has to change its password")
	}
	if a := newBrowser(t, srv.app, nil, u).login(loadPassword); a.status != fiber.StatusUnauthorized {
		t.Errorf("the old password still signs in: %d", a.status)
	}
	nb := newBrowser(t, srv.app, nil, u)
	if a := nb.login("New-Pass-2026!"); a.status != fiber.StatusOK || !signedIn(nb) {
		t.Errorf("the new password does not sign in: %d %s", a.status, a.body)
	}
}

func TestChangeOwnPassword(t *testing.T) {
	srv, db := testServer(t, officeSecurity)
	u := seedPeople(t, db, 1, enums.RoleSalesTeam, false)[0]
	here := newBrowser(t, srv.app, nil, u)
	here.mustSignIn()
	other := newBrowser(t, srv.app, nil, u)
	other.mustSignIn()

	change := func(current, next string) answer {
		return here.do(fiber.MethodPost, "/api/v1/auth/password", map[string]string{"current": current, "password": next})
	}
	if a := change("Wrong-Pass-1!", "New-Pass-2026!"); a.status != fiber.StatusBadRequest {
		t.Errorf("a wrong current password answered %d, want 400", a.status)
	}
	if a := change(loadPassword, loadPassword); a.status != fiber.StatusBadRequest {
		t.Errorf("the same password again answered %d, want 400", a.status)
	}
	if a := change(loadPassword, "short"); a.status != fiber.StatusBadRequest {
		t.Errorf("a password breaking the rules answered %d, want 400", a.status)
	}
	if !signedIn(here) || !signedIn(other) {
		t.Fatal("a refused change ended a session")
	}
	if a := change(loadPassword, "New-Pass-2026!"); a.status != fiber.StatusOK {
		t.Fatalf("the change answered %d %s", a.status, a.body)
	}
	// This browser goes on with a fresh session; the other is signed out
	// and cannot renew.
	if !signedIn(here) {
		t.Error("the browser that changed the password was signed out")
	}
	if signedIn(other) {
		t.Error("another browser is still signed in after the password changed")
	}
	if a := other.do(fiber.MethodPost, "/api/v1/auth/refresh", nil); a.status != fiber.StatusUnauthorized {
		t.Errorf("the other browser renewed its session: %d", a.status)
	}
	if a := newBrowser(t, srv.app, nil, u).login(loadPassword); a.status != fiber.StatusUnauthorized {
		t.Errorf("the old password still signs in: %d", a.status)
	}
	if a := newBrowser(t, srv.app, nil, u).login("New-Pass-2026!"); a.status != fiber.StatusOK {
		t.Errorf("the new password does not sign in: %d", a.status)
	}
}

func TestLogoutEverywhere(t *testing.T) {
	srv, db := testServer(t, officeSecurity)
	u := seedPeople(t, db, 1, enums.RoleSalesTeam, false)[0]
	colleague := seedPeople(t, db, 1, enums.RoleSalesTeam, false)[0]
	browsers := []*browser{newBrowser(t, srv.app, nil, u), newBrowser(t, srv.app, nil, u), newBrowser(t, srv.app, nil, u)}
	for _, b := range browsers {
		b.mustSignIn()
	}
	cb := newBrowser(t, srv.app, nil, colleague)
	cb.mustSignIn()

	if a := browsers[0].do(fiber.MethodPost, "/api/v1/auth/logout/everywhere", nil); a.status != fiber.StatusNoContent {
		t.Fatalf("sign out everywhere answered %d %s", a.status, a.body)
	}
	for i, b := range browsers {
		if signedIn(b) {
			t.Errorf("browser %d is still signed in", i)
		}
		if a := b.do(fiber.MethodPost, "/api/v1/auth/refresh", nil); a.status == fiber.StatusOK {
			t.Errorf("browser %d renewed its session after signing out everywhere", i)
		}
	}
	if !signedIn(cb) {
		t.Error("signing out everywhere also signed out a colleague")
	}
	// Signing in again works at once.
	time.Sleep(time.Until(time.Now().Truncate(time.Second).Add(time.Second + 10*time.Millisecond)))
	nb := newBrowser(t, srv.app, nil, u)
	nb.mustSignIn()
	if !signedIn(nb) {
		t.Error("signing in after signing out everywhere did not work")
	}
}

// customRole adds a role with the given permissions, removed when the test
// ends together with its assignments.
func customRole(t *testing.T, db *gorm.DB, perms ...enums.Permission) models.Role {
	t.Helper()
	var ps []models.Permission
	keys := make([]string, len(perms))
	for i, p := range perms {
		keys[i] = string(p)
	}
	if err := db.Where("key IN ?", keys).Find(&ps).Error; err != nil || len(ps) != len(perms) {
		t.Fatalf("permissions %v: %v", perms, err)
	}
	role := models.Role{Name: fmt.Sprintf("test_%d_%d", time.Now().UnixNano(), roleRun.Add(1)), DisplayName: "Test rolü", Permissions: ps}
	if err := db.Create(&role).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Exec("DELETE FROM user_roles WHERE role_id = ?", role.ID)
		db.Exec("DELETE FROM role_permissions WHERE role_id = ?", role.ID)
		db.Exec("DELETE FROM roles WHERE id = ?", role.ID)
	})
	return role
}

var roleRun atomic.Int64

// giveRoles replaces a user's roles directly in the database.
func giveRoles(t *testing.T, db *gorm.DB, u models.User, roles ...models.Role) {
	t.Helper()
	db.Exec("DELETE FROM user_roles WHERE user_id = ?", u.ID)
	for _, r := range roles {
		if err := db.Exec("INSERT INTO user_roles (user_id, role_id) VALUES (?, ?)", u.ID, r.ID).Error; err != nil {
			t.Fatal(err)
		}
	}
}

func systemRole(t *testing.T, db *gorm.DB, name enums.Role) models.Role {
	t.Helper()
	var r models.Role
	if err := db.Where("name = ?", string(name)).First(&r).Error; err != nil {
		t.Fatal(err)
	}
	return r
}

func TestSetRolesWithinOwnRights(t *testing.T) {
	srv, db := testServer(t, officeSecurity)
	assigner := customRole(t, db, enums.RoleAssign, enums.RoleView, enums.UserView, enums.ContactView)
	small := customRole(t, db, enums.ContactView)
	big := customRole(t, db, enums.ContactView, enums.ContactManage)

	people := seedPeople(t, db, 4, enums.RoleSalesTeam, false)
	actor, lower, above, admin := people[0], people[1], people[2], people[3]
	giveRoles(t, db, actor, assigner)
	giveRoles(t, db, lower, small)
	giveRoles(t, db, above, big)
	giveRoles(t, db, admin, systemRole(t, db, enums.RoleInvisibleAdmin))
	ab := newBrowser(t, srv.app, nil, actor)
	ab.mustSignIn()
	set := func(b *browser, target models.User, roles ...models.Role) answer {
		ids := make([]uint, len(roles))
		for i, r := range roles {
			ids[i] = r.ID
		}
		return b.do(fiber.MethodPatch, fmt.Sprintf("/api/v1/users/%d/roles", target.ID), map[string]any{"roleIds": ids})
	}

	// The person the change is for feels it at once.
	lb := newBrowser(t, srv.app, nil, lower)
	lb.mustSignIn()
	if a := lb.do(fiber.MethodGet, "/api/v1/users", nil); a.status != fiber.StatusForbidden {
		t.Fatalf("before the change the user list answered %d, want 403", a.status)
	}
	if a := set(ab, lower, small, assigner); a.status != fiber.StatusOK {
		t.Fatalf("giving a role within one's own rights answered %d %s", a.status, a.body)
	}
	if a := lb.do(fiber.MethodGet, "/api/v1/users", nil); a.status != fiber.StatusOK {
		t.Errorf("right after the change the user list answered %d, want 200", a.status)
	}
	if a := set(ab, lower, small); a.status != fiber.StatusOK {
		t.Fatalf("taking the role back answered %d %s", a.status, a.body)
	}

	for _, tc := range []struct {
		name   string
		target models.User
		roles  []models.Role
		want   int
	}{
		{"a role holding a permission the giver lacks", lower, []models.Role{big}, fiber.StatusForbidden},
		{"the invisible admin role", lower, []models.Role{systemRole(t, db, enums.RoleInvisibleAdmin)}, fiber.StatusForbidden},
		{"someone holding a permission the giver lacks", above, []models.Role{small}, fiber.StatusForbidden},
		{"one's own roles", actor, []models.Role{assigner, small}, fiber.StatusForbidden},
		{"a role that does not exist", lower, []models.Role{{ID: 2_000_000_000}}, fiber.StatusBadRequest},
		{"no role at all", lower, nil, fiber.StatusBadRequest},
		{"the invisible admin, hidden from others", admin, []models.Role{small}, fiber.StatusNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if a := set(ab, tc.target, tc.roles...); a.status != tc.want {
				t.Errorf("answered %d, want %d: %s", a.status, tc.want, a.body)
			}
		})
	}
	var held []uint
	db.Raw("SELECT role_id FROM user_roles WHERE user_id = ?", above.ID).Scan(&held)
	if len(held) != 1 || held[0] != big.ID {
		t.Errorf("a refused change touched the roles anyway: %v", held)
	}

	// The invisible admin may change anyone.
	adm := newBrowser(t, srv.app, nil, admin)
	adm.mustSignIn()
	if a := set(adm, above, small); a.status != fiber.StatusOK {
		t.Errorf("the invisible admin answered %d %s", a.status, a.body)
	}
	// Without the right to give roles, nothing.
	if a := set(lb, above, small); a.status != fiber.StatusForbidden {
		t.Errorf("someone without role.assign answered %d, want 403", a.status)
	}
}

func TestRoleEditing(t *testing.T) {
	srv, db := testServer(t, officeSecurity)
	people := seedPeople(t, db, 3, enums.RoleSalesTeam, false)
	admin, editor, holder := people[0], people[1], people[2]
	giveRoles(t, db, admin, systemRole(t, db, enums.RoleInvisibleAdmin))
	editorRole := customRole(t, db, enums.RoleManage, enums.RoleView, enums.ContactView)
	giveRoles(t, db, editor, editorRole)
	ab := newBrowser(t, srv.app, nil, admin)
	ab.mustSignIn()
	eb := newBrowser(t, srv.app, nil, editor)
	eb.mustSignIn()

	permID := func(p enums.Permission) uint {
		var id uint
		db.Raw("SELECT id FROM permissions WHERE key = ?", string(p)).Scan(&id)
		return id
	}
	name := fmt.Sprintf("Kalite Ekibi %d", time.Now().UnixNano())
	a := ab.do(fiber.MethodPost, "/api/v1/roles", map[string]any{"name": name, "displayName": "Kalite", "permissionIds": []uint{permID(enums.ContactView)}})
	if a.status != fiber.StatusCreated {
		t.Fatalf("create answered %d %s", a.status, a.body)
	}
	var created struct {
		ID   uint   `json:"id"`
		Name string `json:"name"`
	}
	a.json(t, &created)
	t.Cleanup(func() {
		db.Exec("DELETE FROM user_roles WHERE role_id = ?", created.ID)
		db.Exec("DELETE FROM role_permissions WHERE role_id = ?", created.ID)
		db.Exec("DELETE FROM roles WHERE id = ?", created.ID)
	})
	if !strings.HasPrefix(created.Name, "kalite_ekibi_") {
		t.Errorf("the technical name was kept as %q, want lower case with underscores", created.Name)
	}
	if a := ab.do(fiber.MethodPost, "/api/v1/roles", map[string]any{"name": created.Name, "displayName": "Kalite", "permissionIds": []uint{permID(enums.ContactView)}}); a.status != fiber.StatusConflict {
		t.Errorf("a second role with the same name answered %d, want 409", a.status)
	}
	if a := ab.do(fiber.MethodPost, "/api/v1/roles", map[string]any{"name": "a-", "displayName": "Kısa", "permissionIds": []uint{permID(enums.ContactView)}}); a.status != fiber.StatusBadRequest {
		t.Errorf("a too short name answered %d, want 400", a.status)
	}
	if a := ab.do(fiber.MethodPost, "/api/v1/roles", map[string]any{"name": "bos_rol_deneme", "displayName": "Boş", "permissionIds": []uint{2_000_000_000}}); a.status != fiber.StatusBadRequest {
		t.Errorf("a permission that does not exist answered %d, want 400", a.status)
	}

	// Someone holding the role gets its new permissions at once.
	giveRoles(t, db, holder, models.Role{ID: created.ID})
	hb := newBrowser(t, srv.app, nil, holder)
	hb.mustSignIn()
	if a := hb.do(fiber.MethodGet, "/api/v1/users", nil); a.status != fiber.StatusForbidden {
		t.Fatalf("before the edit the user list answered %d, want 403", a.status)
	}
	if a := ab.do(fiber.MethodPut, fmt.Sprintf("/api/v1/roles/%d", created.ID), map[string]any{"displayName": "Kalite ve kullanıcılar",
		"permissionIds": []uint{permID(enums.ContactView), permID(enums.UserView)}}); a.status != fiber.StatusOK {
		t.Fatalf("update answered %d %s", a.status, a.body)
	}
	if a := hb.do(fiber.MethodGet, "/api/v1/users", nil); a.status != fiber.StatusOK {
		t.Errorf("right after the edit the user list answered %d, want 200", a.status)
	}

	// An editor cannot hand out what they do not hold, cannot touch a role
	// held by someone with more rights, and cannot touch the invisible admin.
	if a := eb.do(fiber.MethodPost, "/api/v1/roles", map[string]any{"name": fmt.Sprintf("genis_%d", time.Now().UnixNano()), "displayName": "Geniş",
		"permissionIds": []uint{permID(enums.ContactView), permID(enums.UserView)}}); a.status != fiber.StatusForbidden {
		t.Errorf("creating a role with a permission the editor lacks answered %d, want 403", a.status)
	}
	if a := eb.do(fiber.MethodPut, fmt.Sprintf("/api/v1/roles/%d", created.ID), map[string]any{"displayName": "Daraltılmış",
		"permissionIds": []uint{permID(enums.ContactView)}}); a.status != fiber.StatusForbidden {
		t.Errorf("editing a role held by someone with more rights answered %d, want 403", a.status)
	}
	invisible := systemRole(t, db, enums.RoleInvisibleAdmin)
	if a := eb.do(fiber.MethodPut, fmt.Sprintf("/api/v1/roles/%d", invisible.ID), map[string]any{"displayName": "Değiştirilmiş",
		"permissionIds": []uint{permID(enums.ContactView)}}); a.status != fiber.StatusForbidden {
		t.Errorf("editing the invisible admin role as an editor answered %d, want 403", a.status)
	}
	// Not even the invisible admin can take role management away from it.
	if a := ab.do(fiber.MethodPut, fmt.Sprintf("/api/v1/roles/%d", invisible.ID), map[string]any{"displayName": invisible.DisplayName,
		"permissionIds": []uint{permID(enums.ContactView)}}); a.status != fiber.StatusBadRequest {
		t.Errorf("taking role management from the invisible admin role answered %d, want 400", a.status)
	}

	// System roles stay; a role still held stays until nobody holds it.
	sales := systemRole(t, db, enums.RoleSalesTeam)
	if a := ab.do(fiber.MethodDelete, fmt.Sprintf("/api/v1/roles/%d", sales.ID), nil); a.status != fiber.StatusBadRequest {
		t.Errorf("deleting a system role answered %d, want 400", a.status)
	}
	if a := ab.do(fiber.MethodDelete, fmt.Sprintf("/api/v1/roles/%d", created.ID), nil); a.status != fiber.StatusConflict {
		t.Errorf("deleting a role someone holds answered %d, want 409", a.status)
	}
	giveRoles(t, db, holder, sales)
	if a := ab.do(fiber.MethodDelete, fmt.Sprintf("/api/v1/roles/%d", created.ID), nil); a.status != fiber.StatusNoContent {
		t.Errorf("deleting a role nobody holds answered %d %s", a.status, a.body)
	}
	var left int64
	db.Model(&models.Role{}).Where("id = ?", created.ID).Count(&left)
	if left != 0 {
		t.Error("the deleted role is still there")
	}
	if a := ab.do(fiber.MethodDelete, fmt.Sprintf("/api/v1/roles/%d", created.ID), nil); a.status != fiber.StatusNotFound {
		t.Errorf("deleting it again answered %d, want 404", a.status)
	}
}

// TestDeactivationEndsSessionAndStream: switching a person off signs them
// out on every browser and closes their live stream within one heartbeat.
func TestDeactivationEndsSessionAndStream(t *testing.T) {
	defer sse.SetHeartbeat(100 * time.Millisecond)()
	srv, db := testServer(t, officeSecurity)
	manager := seedPeople(t, db, 1, enums.RoleManager, false)[0]
	agent := seedPeople(t, db, 1, enums.RoleManager, false)[0]
	mb := newBrowser(t, srv.app, nil, manager)
	mb.mustSignIn()
	ab := newBrowser(t, srv.app, nil, agent)
	ab.mustSignIn()
	second := newBrowser(t, srv.app, nil, agent)
	second.mustSignIn()

	// The agent's live stream.
	req := httptest.NewRequest(fiber.MethodGet, "/api/v1/teams/stream", nil)
	req.Header.Set(fiber.HeaderXForwardedFor, officeIP)
	for _, c := range ab.cookies() {
		req.AddCookie(c)
	}
	before := sse.Open()
	type result struct {
		status int
		body   string
		err    error
	}
	done := make(chan result, 1)
	go func() {
		res, err := srv.app.Test(req, -1)
		if err != nil {
			done <- result{err: err}
			return
		}
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, res.Body)
		_ = res.Body.Close()
		done <- result{status: res.StatusCode, body: buf.String()}
	}()
	deadline := time.Now().Add(5 * time.Second)
	for sse.Open() <= before && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if sse.Open() <= before {
		t.Fatal("the stream never opened")
	}
	// A few heartbeats pass while the agent is still allowed.
	time.Sleep(350 * time.Millisecond)
	select {
	case r := <-done:
		t.Fatalf("the stream closed while the agent was still active: %+v", r)
	default:
	}

	if a := mb.do(fiber.MethodPatch, fmt.Sprintf("/api/v1/users/%d/active", agent.ID), map[string]bool{"active": false}); a.status != fiber.StatusNoContent {
		t.Fatalf("switching the agent off answered %d %s", a.status, a.body)
	}
	select {
	case r := <-done:
		if r.err != nil {
			t.Fatal(r.err)
		}
		if r.status != fiber.StatusOK || !strings.Contains(r.body, `"type":"hello"`) {
			t.Errorf("the stream answered %d %q", r.status, r.body)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the stream of a switched-off person is still open")
	}
	for i, b := range []*browser{ab, second} {
		if signedIn(b) {
			t.Errorf("browser %d of the switched-off person is still signed in", i)
		}
		if a := b.do(fiber.MethodPost, "/api/v1/auth/refresh", nil); a.status == fiber.StatusOK {
			t.Errorf("browser %d of the switched-off person renewed its session", i)
		}
	}
	if a := newBrowser(t, srv.app, nil, agent).login(loadPassword); a.status != fiber.StatusForbidden {
		t.Errorf("a switched-off person signing in answered %d, want 403", a.status)
	}
	// Nobody switches themselves off.
	if a := mb.do(fiber.MethodPatch, fmt.Sprintf("/api/v1/users/%d/active", manager.ID), map[string]bool{"active": false}); a.status != fiber.StatusBadRequest {
		t.Errorf("switching oneself off answered %d, want 400", a.status)
	}
	// Switched on again, the person signs in as before.
	if a := mb.do(fiber.MethodPatch, fmt.Sprintf("/api/v1/users/%d/active", agent.ID), map[string]bool{"active": true}); a.status != fiber.StatusNoContent {
		t.Fatalf("switching the agent on answered %d", a.status)
	}
	time.Sleep(time.Until(time.Now().Truncate(time.Second).Add(time.Second + 10*time.Millisecond)))
	nb := newBrowser(t, srv.app, nil, agent)
	nb.mustSignIn()
	if !signedIn(nb) {
		t.Error("switched on again, the agent cannot use the panel")
	}
}
