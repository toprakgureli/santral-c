package crypt

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// minDataKey is the shortest master data key accepted (openssl rand -hex 32
// gives 64 characters).
const minDataKey = 32

// ErrNotSealed means a value was not made by a Keyring, so it predates it
// and must be opened with the key it was made with.
var ErrNotSealed = errors.New("value was not sealed by the keyring")

// Keyring seals secrets kept in the database. Every purpose (a module such
// as "whatsapp") gets its own AES-256-GCM key, derived from the master data
// key with HKDF-SHA256 and the purpose as its label, so one module's key
// never opens another's. A sealed value starts with "k<id>:", naming the
// master key it was made with: the master key can be replaced, and values
// made with a previous one still open until they are sealed again.
type Keyring struct {
	current  masterKey
	previous []masterKey
}

type masterKey struct {
	id     string
	secret []byte
}

// NewKeyring builds a keyring that seals with current and still opens values
// made with any of previous.
func NewKeyring(current string, previous ...string) (*Keyring, error) {
	cur, err := newMasterKey(current)
	if err != nil {
		return nil, err
	}
	k := &Keyring{current: cur}
	for _, p := range previous {
		if strings.TrimSpace(p) == "" {
			continue
		}
		old, err := newMasterKey(p)
		if err != nil {
			return nil, fmt.Errorf("previous data key: %w", err)
		}
		k.previous = append(k.previous, old)
	}
	return k, nil
}

func newMasterKey(raw string) (masterKey, error) {
	raw = strings.TrimSpace(raw)
	if len(raw) < minDataKey {
		return masterKey{}, fmt.Errorf("data key must be at least %d characters (generate one with: openssl rand -hex 32)", minDataKey)
	}
	sum := sha256.Sum256([]byte("santral-c data key id:" + raw))
	return masterKey{id: hex.EncodeToString(sum[:4]), secret: []byte(raw)}, nil
}

// Seal encrypts plain for purpose with the current key. An empty value stays
// empty.
func (k *Keyring) Seal(purpose, plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	gcm, err := k.current.aead(purpose)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("nonce could not be generated: %w", err)
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plain), []byte(purpose))
	return "k" + k.current.id + ":" + base64.RawURLEncoding.EncodeToString(sealed), nil
}

// Open decrypts a value Seal made for purpose, with whichever known key made
// it. It returns ErrNotSealed for a value from before the keyring.
func (k *Keyring) Open(purpose, sealed string) (string, error) {
	if sealed == "" {
		return "", nil
	}
	id, body, ok := split(sealed)
	if !ok {
		return "", ErrNotSealed
	}
	key, ok := k.key(id)
	if !ok {
		return "", fmt.Errorf("value was sealed with an unknown data key %q", id)
	}
	gcm, err := key.aead(purpose)
	if err != nil {
		return "", err
	}
	raw, err := base64.RawURLEncoding.DecodeString(body)
	if err != nil {
		return "", fmt.Errorf("value could not be decoded: %w", err)
	}
	if len(raw) < gcm.NonceSize() {
		return "", errors.New("value is too short to decrypt")
	}
	plain, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], []byte(purpose))
	if err != nil {
		return "", fmt.Errorf("value could not be decrypted: %w", err)
	}
	return string(plain), nil
}

// Current reports whether sealed was made with the current key, so it needs
// no sealing again. An empty value counts as current.
func (k *Keyring) Current(sealed string) bool {
	if sealed == "" {
		return true
	}
	id, _, ok := split(sealed)
	return ok && id == k.current.id
}

// MACKeys returns the signing keys for purpose, the current one first and
// then those of the previous master keys, so a link signed before the data
// key was replaced still checks. Each purpose gets its own key, apart from
// the ones that seal secrets.
func (k *Keyring) MACKeys(purpose string) [][]byte {
	out := make([][]byte, 0, 1+len(k.previous))
	for _, m := range append([]masterKey{k.current}, k.previous...) {
		derived, err := hkdf.Key(sha256.New, m.secret, nil, "santral-c/mac/"+purpose, 32)
		if err != nil {
			continue
		}
		out = append(out, derived)
	}
	return out
}

func (k *Keyring) key(id string) (masterKey, bool) {
	if id == k.current.id {
		return k.current, true
	}
	for _, p := range k.previous {
		if p.id == id {
			return p, true
		}
	}
	return masterKey{}, false
}

func split(sealed string) (id, body string, ok bool) {
	if !strings.HasPrefix(sealed, "k") {
		return "", "", false
	}
	id, body, ok = strings.Cut(sealed[1:], ":")
	if !ok || len(id) != 8 {
		return "", "", false
	}
	return id, body, true
}

func (m masterKey) aead(purpose string) (cipher.AEAD, error) {
	derived, err := hkdf.Key(sha256.New, m.secret, nil, "santral-c/"+purpose, 32)
	if err != nil {
		return nil, fmt.Errorf("key could not be derived: %w", err)
	}
	block, err := aes.NewCipher(derived)
	if err != nil {
		return nil, fmt.Errorf("cipher could not be created: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("gcm could not be created: %w", err)
	}
	return gcm, nil
}
