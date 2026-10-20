package factory

import (
	"shipdude/internal/db"
	"shipdude/internal/repository"
	"shipdude/pkg/socket"
)

// Factory wires repositories and shared runtime state for the thin handler
// constructors. Services pull only the repository they need.
type Factory struct {
	DB      *db.DB
	Hub     *socket.Hub
	Server  repository.Server
	Project repository.Project
	Setting repository.Setting
}

// NewFactory builds a Factory around an open database connection.
func NewFactory(database *db.DB) *Factory {
	return &Factory{
		DB:      database,
		Hub:     socket.NewHub(),
		Server:  repository.NewServerRepository(database),
		Project: repository.NewProjectRepository(database),
		Setting: repository.NewSettingRepository(database),
	}
}

// InitDB exposes the raw database so services can open transactions for
// multi-statement writes.
func (f *Factory) InitDB() *db.DB {
	return f.DB
}
