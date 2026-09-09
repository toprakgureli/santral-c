// Package crypt provides AES-GCM encryption for small secrets at rest.
package crypt

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
)

func newGCM(key string) (cipher.AEAD, error) {
	sum := sha256.Sum256([]byte(key))
	block, err := aes.NewCipher(sum[:])
	if err != nil {
		return nil, fmt.Errorf("cipher could not be created: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("gcm could not be created: %w", err)
	}
	return gcm, nil
}

// Encrypt seals plain with a key derived from key and returns base64url text.
func Encrypt(key, plain string) (string, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("nonce could not be generated: %w", err)
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plain), nil)
	return base64.RawURLEncoding.EncodeToString(sealed), nil
}

// Decrypt reverses Encrypt for the same key.
func Decrypt(key, encoded string) (string, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return "", err
	}
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("value could not be decoded: %w", err)
	}
	if len(raw) < gcm.NonceSize() {
		return "", fmt.Errorf("value is too short to decrypt")
	}
	nonce, body := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, body, nil)
	if err != nil {
		return "", fmt.Errorf("value could not be decrypted: %w", err)
	}
	return string(plain), nil
}
