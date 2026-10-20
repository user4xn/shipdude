package crypto

import (
	"crypto/rand"
	"encoding/base64"
	"testing"

	"golang.org/x/crypto/nacl/box"
)

func TestEncryptSecret_Roundtrip(t *testing.T) {
	pubKey, privKey, err := box.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate box key pair: %v", err)
	}

	pubKeyB64 := base64.StdEncoding.EncodeToString(pubKey[:])
	secretText := "my-secret-ssh-key-data-12345"

	encryptedB64, err := EncryptSecret(pubKeyB64, secretText)
	if err != nil {
		t.Fatalf("failed to encrypt secret: %v", err)
	}

	encryptedBytes, err := base64.StdEncoding.DecodeString(encryptedB64)
	if err != nil {
		t.Fatalf("failed to decode encrypted base64: %v", err)
	}

	decrypted, ok := box.OpenAnonymous(nil, encryptedBytes, pubKey, privKey)
	if !ok {
		t.Fatalf("failed to open sealed box")
	}
	if string(decrypted) != secretText {
		t.Errorf("expected %q, got %q", secretText, string(decrypted))
	}
}

func TestEncryptDecryptString(t *testing.T) {
	key, err := GenerateKey()
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	ciphertext, err := EncryptString(key, "top secret")
	if err != nil {
		t.Fatalf("failed to encrypt: %v", err)
	}
	if ciphertext == "top secret" {
		t.Fatalf("ciphertext must not equal plaintext")
	}

	plaintext, err := DecryptString(key, ciphertext)
	if err != nil {
		t.Fatalf("failed to decrypt: %v", err)
	}
	if plaintext != "top secret" {
		t.Errorf("expected 'top secret', got %q", plaintext)
	}
}

func TestGenerateSSHKeyPair(t *testing.T) {
	pair, err := GenerateSSHKeyPair()
	if err != nil {
		t.Fatalf("failed to generate ssh key pair: %v", err)
	}
	if !SplitPEM(pair.PrivateKeyPEM, "RSA PRIVATE KEY") {
		t.Errorf("expected an RSA PRIVATE KEY PEM block")
	}
	if pair.PublicSSHKey == "" {
		t.Errorf("expected a non-empty authorized_keys entry")
	}
}
