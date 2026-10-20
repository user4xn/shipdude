package project

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newMockGitHubServer starts a fake GitHub API that accepts a specific token,
// serves the Actions public key, records secrets/workflow writes and reports a
// Dockerfile as present.
func newMockGitHubServer(t *testing.T, expectedToken string) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if expectedToken != "" {
			auth := r.Header.Get("Authorization")
			if auth != "token "+expectedToken && auth != "Bearer "+expectedToken {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
		}

		w.Header().Set("Content-Type", "application/json")

		switch {
		case strings.Contains(r.URL.Path, "public-key"):
			json.NewEncoder(w).Encode(map[string]string{
				"key_id": "key-123",
				"key":    base64.StdEncoding.EncodeToString(make([]byte, 32)),
			})
		case r.Method == http.MethodPut:
			w.WriteHeader(http.StatusCreated)
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "contents"):
			if strings.Contains(r.URL.Path, "Dockerfile") {
				w.Write([]byte(`{"type":"file","name":"Dockerfile"}`))
				return
			}
			w.WriteHeader(http.StatusNotFound)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}
