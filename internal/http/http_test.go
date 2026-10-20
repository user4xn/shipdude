package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"shipdude/internal/db"
	"shipdude/internal/factory"
)

func newTestRouter(t *testing.T) http.Handler {
	t.Helper()

	database, err := db.Connect(filepath.Join(t.TempDir(), "shipdude.db"))
	if err != nil {
		t.Fatalf("failed to init database: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	static := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	return NewRouter(factory.NewFactory(database), static)
}

func TestRouter_RegistersModuleRoutes(t *testing.T) {
	router := newTestRouter(t)

	cases := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/settings/github"},
		{http.MethodGet, "/api/v1/github/user"},
		{http.MethodGet, "/api/v1/vps/last-connection"},
		{http.MethodGet, "/api/v1/projects"},
	}

	for _, tc := range cases {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
		if w.Code == http.StatusNotFound {
			t.Errorf("%s %s should be registered, got 404", tc.method, tc.path)
		}
	}
}

func TestRouter_ServesSPAForUnknownPaths(t *testing.T) {
	router := newTestRouter(t)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/some/deep/spa/route", nil))

	if w.Code != http.StatusOK {
		t.Errorf("expected SPA fallback to serve 200, got %d", w.Code)
	}
}

func TestRouter_ReturnsUniformEnvelope(t *testing.T) {
	router := newTestRouter(t)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil))

	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected JSON content type, got %q", ct)
	}

	var env map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("failed to decode envelope: %v", err)
	}
	for _, key := range []string{"message", "status"} {
		if _, ok := env[key]; !ok {
			t.Errorf("expected envelope to include %q, got %v", key, env)
		}
	}
}

func TestRouter_OptionsCORS(t *testing.T) {
	router := newTestRouter(t)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodOptions, "/api/v1/projects", nil))

	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("expected CORS header on preflight")
	}
}
