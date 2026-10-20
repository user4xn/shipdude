package project

import (
	"github.com/go-chi/chi/v5"

	"shipdude/internal/factory"
)

// Register mounts the project routes under the API root.
func Register(r chi.Router, f *factory.Factory) {
	h := NewHandler(f)

	r.Get("/projects", h.List)
	r.Post("/projects/deploy", h.Deploy)
	r.Get("/projects/deploy/logs", h.DeployLogs)
	r.Post("/projects/deploy-trigger", h.Trigger)
	r.Get("/projects/{id}/logs", h.Logs)
	r.Post("/projects/{id}/rollback", h.Rollback)
	r.Delete("/projects/{id}", h.Delete)
	r.Post("/projects/{id}/control", h.Control)
	r.Get("/projects/{id}/telemetry", h.Telemetry)
	r.Get("/projects/{id}/container-logs", h.ContainerLogs)
}
