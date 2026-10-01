// Package hash provides password hashing and token helpers.
package hash

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"
)

const (
	argonTime    uint32 = 3
	argonMemory  uint32 = 64 * 1024
	argonThreads uint8  = 4
	argonKeyLen  uint32 = 32
	argonSaltLen int    = 16
)

// hashSlots is how many passwords are hashed or checked at the same time.
// Each takes 64 MB for a moment, so a whole office signing in at nine (or
// someone flooding the sign-in page) would otherwise need gigabytes at
// once and could push the server past its memory limit. The rest wait
// their turn, a few dozen milliseconds each.
const hashSlots = 4

var slots = make(chan struct{}, hashSlots)

// argonKey is argon2id; tests replace it to watch how many run at once.
var argonKey = argon2.IDKey

// derive runs argon2id inside one of the slots.
func derive(password, salt []byte, time, memory uint32, threads uint8, keyLen uint32) []byte {
	slots <- struct{}{}
	defer func() { <-slots }()
	return argonKey(password, salt, time, memory, threads, keyLen)
}

// Password hashes a plaintext password with argon2id and returns a PHC string.
func Password(plain string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := derive([]byte(plain), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	encoded := fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	)
	return encoded, nil
}

// ErrBusy means every password-check slot stayed busy for the whole wait.
var ErrBusy = errors.New("password checks are all busy")

// slotWait is the longest a sign-in waits for a password-check slot. A
// flood of sign-in attempts then turns into a short "busy, try again"
// for the people behind it instead of requests that hang until they time
// out.
const slotWait = 5 * time.Second

// Check is Compare for a sign-in: it waits for a slot at most slotWait,
// or until ctx ends, and then gives up with ErrBusy.
func Check(ctx context.Context, encoded, plain string) (bool, error) {
	salt, key, t, m, p, err := decode(encoded)
	if err != nil {
		return false, nil
	}
	ctx, cancel := context.WithTimeout(ctx, slotWait)
	defer cancel()
	select {
	case slots <- struct{}{}:
	case <-ctx.Done():
		return false, ErrBusy
	}
	defer func() { <-slots }()
	computed := argonKey([]byte(plain), salt, t, m, p, uint32(len(key)))
	return subtle.ConstantTimeCompare(key, computed) == 1, nil
}

// Compare reports whether plain matches the encoded argon2id hash in constant time.
func Compare(encoded, plain string) bool {
	salt, key, t, m, p, err := decode(encoded)
	if err != nil {
		return false
	}
	computed := derive([]byte(plain), salt, t, m, p, uint32(len(key)))
	return subtle.ConstantTimeCompare(key, computed) == 1
}

func decode(encoded string) (salt, key []byte, t, m uint32, p uint8, err error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return nil, nil, 0, 0, 0, errors.New("invalid hash format")
	}
	var version int
	if _, err = fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return nil, nil, 0, 0, 0, errors.New("invalid hash version")
	}
	if _, err = fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil {
		return nil, nil, 0, 0, 0, err
	}
	if salt, err = base64.RawStdEncoding.DecodeString(parts[4]); err != nil {
		return nil, nil, 0, 0, 0, err
	}
	if key, err = base64.RawStdEncoding.DecodeString(parts[5]); err != nil {
		return nil, nil, 0, 0, 0, err
	}
	return salt, key, t, m, p, nil
}

// Token returns size random bytes as a URL-safe base64 string.
func Token(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// SHA256 returns the hex-encoded SHA-256 digest of value.
func SHA256(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
