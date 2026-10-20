package repository

import (
	"context"
	"database/sql"
	"errors"

	"shipdude/internal/db"
)

// Setting describes the encrypted key/value configuration contract.
type Setting interface {
	FindValue(ctx context.Context, key string) (string, error)
	Store(db *sql.DB, key string, value string) error
}

type setting struct {
	Db *db.DB
}

// NewSettingRepository wires a Setting repository over the encrypted database.
func NewSettingRepository(database *db.DB) Setting {
	return &setting{Db: database}
}

// FindValue returns the decrypted value for a key, or "" when it is missing.
func (r *setting) FindValue(ctx context.Context, key string) (string, error) {
	var encrypted string
	err := r.Db.SQL().QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&encrypted)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", err
	}

	if encrypted == "" {
		return "", nil
	}

	return r.Db.Decrypt(encrypted)
}

// Store upserts and encrypts a setting value.
func (r *setting) Store(database *sql.DB, key string, value string) error {
	encrypted, err := r.Db.Encrypt(value)
	if err != nil {
		return err
	}

	_, err = database.Exec(
		`INSERT OR REPLACE INTO settings (key, value) VALUES (?, ?)`,
		key, encrypted,
	)
	return err
}
