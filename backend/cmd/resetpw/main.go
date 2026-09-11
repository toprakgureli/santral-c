// Command resetpw resets (or creates) a user's password directly against the
// database, for recovering a forgotten owner login. It clears MFA and unlocks
// the account so the user can sign back in and re-enroll.
//
// Usage:
//
//	go run ./cmd/resetpw -config config.yml -email owner@example.com -password 'NewPass123!'
//
// If the email does not exist it is created as an invisible-admin (owner).
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/toprakgureli/santral-c/backend/configs"
	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
	"github.com/toprakgureli/santral-c/backend/pkg/hash"
	"github.com/toprakgureli/santral-c/backend/pkg/postgresql"
)

func main() {
	cfgPath := flag.String("config", "config.yml", "path to the configuration file")
	email := flag.String("email", "", "user email")
	password := flag.String("password", "", "new password (min 8 chars)")
	name := flag.String("name", "Owner", "display name when creating a new user")
	flag.Parse()

	if *email == "" || len(*password) < 8 {
		fmt.Println("usage: resetpw -email <email> -password <min 8 chars> [-config config.yml] [-name Name]")
		os.Exit(1)
	}
	if err := configs.Load(*cfgPath); err != nil {
		fmt.Println("config could not be loaded:", err)
		os.Exit(1)
	}
	if err := postgresql.Connect(configs.Cnf.Database); err != nil {
		fmt.Println("database could not be reached:", err)
		os.Exit(1)
	}
	db := postgresql.Get()

	hashed, err := hash.Password(*password)
	if err != nil {
		fmt.Println("password could not be hashed:", err)
		os.Exit(1)
	}

	res := db.Model(&models.User{}).Where("email = ?", *email).Updates(map[string]any{
		"password":             hashed,
		"must_change_password": false,
		"mfa_enabled":          false,
		"mfa_secret":           nil,
		"failed_count":         0,
		"locked_until":         nil,
		"active":               true,
	})
	if res.Error != nil {
		fmt.Println("update failed:", res.Error)
		os.Exit(1)
	}
	if res.RowsAffected > 0 {
		fmt.Printf("Password reset for %s (MFA cleared, account unlocked).\n", *email)
		return
	}

	// No such user: create an invisible-admin (owner).
	var role models.Role
	if err := db.Where("name = ?", string(enums.RoleInvisibleAdmin)).First(&role).Error; err != nil {
		fmt.Println("invisible-admin role not found (run the server once to seed):", err)
		os.Exit(1)
	}
	u := &models.User{Name: *name, Email: *email, Password: hashed, Active: true, Roles: []models.Role{role}}
	if err := db.Omit("Roles.*").Create(u).Error; err != nil {
		fmt.Println("user could not be created:", err)
		os.Exit(1)
	}
	fmt.Printf("Created owner %s.\n", *email)
}
