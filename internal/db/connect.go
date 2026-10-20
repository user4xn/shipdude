package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"shipdude/pkg/crypto"

	_ "modernc.org/sqlite"
)

// DB wraps the SQLite handle together with the AES key used to encrypt
// secrets at rest and exposes transactional helpers.
type DB struct {
	sql    *sql.DB
	aesKey []byte
}

// Connect opens (or creates) the SQLite database at path, loads or generates
// the AES-256 key stored beside it and applies the schema.
func Connect(path string) (*DB, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create config directory: %w", err)
	}

	aesKey, err := loadOrCreateKey(dir)
	if err != nil {
		return nil, err
	}

	handle, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	database := &DB{sql: handle, aesKey: aesKey}
	if err := database.migrate(); err != nil {
		_ = handle.Close()
		return nil, fmt.Errorf("failed to init schema: %w", err)
	}

	return database, nil
}

// Close releases the underlying database handle.
func (d *DB) Close() error {
	return d.sql.Close()
}

// SQL exposes the raw handle to repositories that need custom statements.
func (d *DB) SQL() *sql.DB {
	return d.sql
}

// Encrypt encrypts a plaintext secret with the database AES key.
func (d *DB) Encrypt(plaintext string) (string, error) {
	return crypto.EncryptString(d.aesKey, plaintext)
}

// Decrypt decrypts a secret produced by Encrypt.
func (d *DB) Decrypt(payload string) (string, error) {
	return crypto.DecryptString(d.aesKey, payload)
}

// DecryptIntoField decrypts a stored secret in place, keeping the wrapped
// cause reachable while attaching a human-readable description.
func (d *DB) DecryptIntoField(field *string) error {
	plaintext, err := crypto.DecryptString(d.aesKey, *field)
	if err != nil {
		return fmt.Errorf("failed to decrypt ssh key: %w", err)
	}
	*field = plaintext
	return nil
}

func loadOrCreateKey(dir string) ([]byte, error) {
	keyPath := filepath.Join(dir, "encryption.key")

	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		key, err := crypto.GenerateKey()
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(keyPath, key, 0o600); err != nil {
			return nil, fmt.Errorf("failed to write encryption key: %w", err)
		}
		return key, nil
	}

	key, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read encryption key: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("invalid encryption key length: expected 32, got %d", len(key))
	}

	return key, nil
}

func (d *DB) migrate() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS servers (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			ip_address TEXT NOT NULL,
			ssh_port INTEGER DEFAULT 22,
			ssh_user TEXT DEFAULT 'root',
			ssh_private_key TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS projects (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			server_id INTEGER NOT NULL,
			github_repo TEXT NOT NULL,
			github_branch TEXT DEFAULT 'main',
			project_name TEXT NOT NULL,
			deployment_path TEXT NOT NULL,
			stack_type TEXT NOT NULL,
			stack_version TEXT NOT NULL,
			app_port INTEGER NOT NULL,
			FOREIGN KEY(server_id) REFERENCES servers(id)
		);`,
		`CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);`,
	}

	for _, q := range queries {
		if _, err := d.sql.Exec(q); err != nil {
			return err
		}
	}

	// Back-fill columns introduced after the first release. These ALTER
	// statements are intentionally best-effort and idempotent.
	for _, alter := range []string{
		`ALTER TABLE projects ADD COLUMN domain TEXT DEFAULT ''`,
		`ALTER TABLE projects ADD COLUMN active_port INTEGER DEFAULT 0`,
		`ALTER TABLE projects ADD COLUMN active_image TEXT DEFAULT ''`,
	} {
		_, _ = d.sql.Exec(alter)
	}

	return nil
}
