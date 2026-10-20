package repository

import (
	"context"
	"database/sql"
	"fmt"

	"shipdude/internal/db"
	"shipdude/internal/model"
	"shipdude/pkg/dbutil"
)

// Server describes the data access contract for VPS nodes.
type Server interface {
	FindAll(ctx context.Context, opts ...dbutil.Scope) ([]model.Server, error)
	FindOne(ctx context.Context, id int64) (*model.Server, error)
	Store(db *sql.DB, server model.Server) (int64, error)
}

type server struct {
	Db *db.DB
}

// NewServerRepository wires a Server repository over the encrypted database.
func NewServerRepository(database *db.DB) Server {
	return &server{Db: database}
}

const serverColumns = "id, name, ip_address, ssh_port, ssh_user, ssh_private_key"

// FindAll returns every server, decrypting each private key in place.
func (r *server) FindAll(ctx context.Context, opts ...dbutil.Scope) ([]model.Server, error) {
	q := dbutil.ApplyScopes(opts...)
	fields := serverColumns
	if q.Fields != "" {
		fields = q.Fields
	}

	query := "SELECT " + fields + " FROM servers" + buildTail(q)

	rows, err := r.Db.SQL().QueryContext(ctx, query, q.Args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []model.Server
	for rows.Next() {
		var srv model.Server
		if err := scanServer(rows, &srv); err != nil {
			return nil, err
		}
		if err := r.Db.DecryptIntoField(&srv.SSHPrivateKey); err != nil {
			return nil, err
		}
		list = append(list, srv)
	}

	return list, rows.Err()
}

// FindOne returns a single server by ID with its private key decrypted.
func (r *server) FindOne(ctx context.Context, id int64) (*model.Server, error) {
	var srv model.Server
	row := r.Db.SQL().QueryRowContext(ctx, "SELECT "+serverColumns+" FROM servers WHERE id = ?", id)
	if err := scanServer(row, &srv); err != nil {
		return nil, err
	}
	if err := r.Db.DecryptIntoField(&srv.SSHPrivateKey); err != nil {
		return nil, err
	}
	return &srv, nil
}

// Store encrypts the SSH key and inserts a new server row.
func (r *server) Store(database *sql.DB, srv model.Server) (int64, error) {
	encryptedKey, err := r.Db.Encrypt(srv.SSHPrivateKey)
	if err != nil {
		return 0, fmt.Errorf("failed to encrypt ssh key: %w", err)
	}

	res, err := database.Exec(
		`INSERT INTO servers (name, ip_address, ssh_port, ssh_user, ssh_private_key) VALUES (?, ?, ?, ?, ?)`,
		srv.Name, srv.IPAddress, srv.SSHPort, srv.SSHUser, encryptedKey,
	)
	if err != nil {
		return 0, err
	}

	return res.LastInsertId()
}

type rowScanner interface {
	Scan(dest ...any) error
}

// buildTail renders the optional WHERE / ORDER BY / LIMIT clauses of a query.
func buildTail(q *dbutil.Query) string {
	tail := ""
	if q.Where != "" {
		tail += " WHERE " + q.Where
	}
	if q.OrderBy != "" {
		tail += " ORDER BY " + q.OrderBy
	}
	if q.Limit > 0 {
		tail += fmt.Sprintf(" LIMIT %d", q.Limit)
	}
	if q.Offset > 0 {
		tail += fmt.Sprintf(" OFFSET %d", q.Offset)
	}
	return tail
}

func scanServer(row rowScanner, srv *model.Server) error {
	return row.Scan(&srv.ID, &srv.Name, &srv.IPAddress, &srv.SSHPort, &srv.SSHUser, &srv.SSHPrivateKey)
}
