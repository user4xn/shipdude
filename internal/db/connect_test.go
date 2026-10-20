package db

import (
	"os"
	"path/filepath"
	"testing"
)

func newTestDB(t *testing.T) *DB {
	t.Helper()

	tmpDir := t.TempDir()
	database, err := Connect(filepath.Join(tmpDir, "shipdude.db"))
	if err != nil {
		t.Fatalf("failed to init database: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func TestConnect_CreatesKeyAndSchema(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "shipdude.db")

	database, err := Connect(path)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer database.Close()

	if _, err := os.Stat(filepath.Join(tmpDir, "encryption.key")); err != nil {
		t.Errorf("expected encryption key to be created: %v", err)
	}

	for _, table := range []string{"servers", "projects", "settings"} {
		var name string
		err := database.SQL().QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&name)
		if err != nil {
			t.Errorf("expected table %q to exist: %v", table, err)
		}
	}
}

func TestEncryptDecryptRoundtrip(t *testing.T) {
	database := newTestDB(t)

	ciphertext, err := database.Encrypt("github_pat_1234567")
	if err != nil {
		t.Fatalf("failed to encrypt: %v", err)
	}

	plaintext, err := database.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("failed to decrypt: %v", err)
	}
	if plaintext != "github_pat_1234567" {
		t.Errorf("expected 'github_pat_1234567', got %q", plaintext)
	}
}
