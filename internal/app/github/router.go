package github

import (
	"github.com/go-chi/chi/v5"

	"shipdude/internal/factory"
)

// Register mounts the GitHub routes under the API root.
func Register(r chi.Router, f *factory.Factory) {
	h := NewHandler(f)

	r.Get("/settings/github", h.GetSettings)
	r.Post("/settings/github", h.PostSettings)

	r.Get("/auth/github", h.Login)
	r.Get("/auth/github/callback", h.Callback)

	r.Get("/github/repos", h.GetRepos)
	r.Get("/github/user", h.GetUser)
	r.Get("/github/branches", h.GetBranches)
	r.Post("/github/disconnect", h.Disconnect)
}
