// Package jwt issues and parses the gateway's signed tokens.
package jwt

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/toprakgureli/santral-c/backend/configs"
	"github.com/toprakgureli/santral-c/backend/pkg/hash"
)

// Token purposes.
const (
	PurposeAccess   string = "access"
	PurposeMFA      string = "mfa"
	PurposeEnroll   string = "mfa_enroll"
	PurposePassword string = "password_change"
)

const mfaTTL time.Duration = 5 * time.Minute

// Claims are the santral JWT claims.
type Claims struct {
	UserID  uint   `json:"uid"`
	Purpose string `json:"pur,omitempty"`
	jwt.RegisteredClaims
}

// Token is a signed token with its id and expiry.
type Token struct {
	Value     string
	ID        string
	ExpiresAt time.Time
}

// Generate issues an access token.
func Generate(c configs.Auth, userID uint) (*Token, error) {
	return generate(c, userID, PurposeAccess, c.AccessTTL)
}

// GenerateMFA issues a short-lived second-factor token.
func GenerateMFA(c configs.Auth, userID uint) (*Token, error) {
	return generate(c, userID, PurposeMFA, mfaTTL)
}

// GenerateEnroll issues a short-lived MFA enrollment token.
func GenerateEnroll(c configs.Auth, userID uint) (*Token, error) {
	return generate(c, userID, PurposeEnroll, mfaTTL)
}

// GeneratePassword issues a short-lived password-change token.
func GeneratePassword(c configs.Auth, userID uint) (*Token, error) {
	return generate(c, userID, PurposePassword, mfaTTL)
}

func generate(c configs.Auth, userID uint, purpose string, ttl time.Duration) (*Token, error) {
	now := time.Now()
	expiresAt := now.Add(ttl)
	id, err := hash.Token(16)
	if err != nil {
		return nil, err
	}
	claims := Claims{
		UserID:  userID,
		Purpose: purpose,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        id,
			Issuer:    c.Issuer,
			Subject:   fmt.Sprint(userID),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(c.Secret))
	if err != nil {
		return nil, fmt.Errorf("token could not be signed: %w", err)
	}
	return &Token{Value: signed, ID: id, ExpiresAt: expiresAt}, nil
}

// Parse validates a token and returns its claims.
func Parse(c configs.Auth, token string) (*Claims, error) {
	claims := &Claims{}
	parsed, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(c.Secret), nil
	}, jwt.WithIssuer(c.Issuer), jwt.WithExpirationRequired())
	if err != nil {
		return nil, fmt.Errorf("token could not be parsed: %w", err)
	}
	if !parsed.Valid {
		return nil, fmt.Errorf("token is not valid")
	}
	return claims, nil
}
