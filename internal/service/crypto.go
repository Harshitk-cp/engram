package service

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"strings"
)

// cryptoShredPrefix marks content that has been crypto-shredded (AES-GCM
// ciphertext under a destroyed key — permanently unrecoverable).
const cryptoShredPrefix = "enc:v1:"

// cryptoShred encrypts plaintext with a fresh random AES-256 key that is then
// discarded (never stored), so the result is cryptographically unrecoverable.
// This is the erasure primitive for per-subject "right to be forgotten" that
// preserves the row + the immutable audit chain (unlike a hard delete).
func cryptoShred(plaintext string) (string, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	ct := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	// key and plaintext go out of scope here; the key is never persisted.
	return cryptoShredPrefix + base64.StdEncoding.EncodeToString(ct), nil
}

// IsShredded reports whether content has been crypto-shredded.
func IsShredded(content string) bool { return strings.HasPrefix(content, cryptoShredPrefix) }
