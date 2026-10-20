package vps

import (
	"github.com/go-chi/chi/v5"

	"shipdude/internal/factory"
)

// Register mounts the VPS routes under the API root.
func Register(r chi.Router, f *factory.Factory) {
	h := NewHandler(f)

	r.Post("/vps/probe", h.Probe)
	r.Get("/vps/last-connection", h.GetLastConnection)
	r.Post("/vps/last-connection", h.PostLastConnection)
	r.Post("/vps/provision", h.Provision)
	r.Get("/vps/provision/logs", h.ProvisionLogs)
}
