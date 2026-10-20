package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"strings"
)

// keySize is the AES-256 key length in bytes.
const keySize = 32

// GenerateKey creates a fresh 32-byte AES-256 key.
func GenerateKey() ([]byte, error) {
	key := make([]byte, keySize)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("failed to generate encryption key: %w", err)
	}
	return key, nil
}

// EncryptString encrypts plaintext with AES-256-GCM and returns a hex-encoded
// nonce+ciphertext payload.
func EncryptString(key []byte, plaintext string) (string, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return hex.EncodeToString(ciphertext), nil
}

// DecryptString reverses EncryptString.
func DecryptString(key []byte, payload string) (string, error) {
	ciphertext, err := hex.DecodeString(payload)
	if err != nil {
		return "", err
	}

	gcm, err := newGCM(key)
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return "", errors.New("ciphertext too short")
	}

	nonce, actualCiphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, actualCiphertext, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}

func newGCM(key []byte) (cipher.AEAD, error) {
	if len(key) != keySize {
		return nil, fmt.Errorf("invalid encryption key length: expected %d, got %d", keySize, len(key))
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	return cipher.NewGCM(block)
}

// SplitPEM is a small helper used by tests and tooling to validate that a
// string is a well-formed PEM block of the given type.
func SplitPEM(data, blockType string) bool {
	block, _ := pem.Decode([]byte(data))
	if block == nil {
		return false
	}
	return strings.EqualFold(block.Type, blockType)
}
