package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"shipdude/internal/app/github"
	"shipdude/internal/app/project"
	"shipdude/internal/app/vps"
	"shipdude/internal/factory"
	"shipdude/internal/middleware"
)

// NewRouter builds the HTTP router, registering every module router under the
// API prefix and serving the embedded SPA for everything else.
func NewRouter(f *factory.Factory, staticHandler http.Handler) http.Handler {
	r := chi.NewRouter()

	for _, mw := range middleware.Base() {
		r.Use(mw)
	}

	r.Route("/api/v1", func(r chi.Router) {
		github.Register(r, f)
		vps.Register(r, f)
		project.Register(r, f)
	})

	r.Handle("/*", staticHandler)

	return r
}
