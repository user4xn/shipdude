package middleware

import (
	"net/http"

	"github.com/go-chi/chi/v5/middleware"
)

// Base returns the stack of middleware shared by every route: request
// logging, panic recovery and permissive CORS for the local portal UI.
func Base() []func(http.Handler) http.Handler {
	return []func(http.Handler) http.Handler{
		middleware.Logger,
		middleware.Recoverer,
		AllowAllCORS,
	}
}

// AllowAllCORS adds permissive CORS headers. ShipDude is a single-operator
// tool meant to be reachable only by its owner, so any origin is accepted.
func AllowAllCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
