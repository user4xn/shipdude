package github

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"shipdude/internal/db"
	"shipdude/internal/factory"
	"shipdude/pkg/client"
)

func newTestHandler(t *testing.T) *handler {
	t.Helper()
	database, err := db.Connect(filepath.Join(t.TempDir(), "shipdude.db"))
	if err != nil {
		t.Fatalf("failed to init database: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return NewHandler(factory.NewFactory(database))
}

func decodeEnvelope(t *testing.T, body []byte) map[string]interface{} {
	t.Helper()
	var env map[string]interface{}
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("failed to decode envelope: %v (body: %s)", err, body)
	}
	return env
}

func TestGetSettings_InitiallyUnconfigured(t *testing.T) {
	h := newTestHandler(t)

	w := httptest.NewRecorder()
	h.GetSettings(w, httptest.NewRequest(http.MethodGet, "/api/v1/settings/github", nil))

	env := decodeEnvelope(t, w.Body.Bytes())
	data := env["data"].(map[string]interface{})
	if data["configured"].(bool) != false {
		t.Errorf("expected configured=false, got %v", data["configured"])
	}
}

func TestPostSettings_ThenConfigured(t *testing.T) {
	h := newTestHandler(t)

	payload := `{"client_id":"my-client-id","client_secret":"my-client-secret"}`
	w := httptest.NewRecorder()
	h.PostSettings(w, httptest.NewRequest(http.MethodPost, "/api/v1/settings/github", bytes.NewBufferString(payload)))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", w.Code, w.Body.String())
	}

	wVerify := httptest.NewRecorder()
	h.GetSettings(wVerify, httptest.NewRequest(http.MethodGet, "/api/v1/settings/github", nil))

	env := decodeEnvelope(t, wVerify.Body.Bytes())
	data := env["data"].(map[string]interface{})
	if data["configured"].(bool) != true {
		t.Errorf("expected configured=true, got %v", data["configured"])
	}
	if data["client_id"].(string) != "my-client-id" {
		t.Errorf("expected client_id 'my-client-id', got %v", data["client_id"])
	}
}

func TestPostSettings_RequiresFields(t *testing.T) {
	h := newTestHandler(t)

	w := httptest.NewRecorder()
	h.PostSettings(w, httptest.NewRequest(http.MethodPost, "/api/v1/settings/github", bytes.NewBufferString(`{"client_id":"only"}`)))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestGetRepos_Unauthorized(t *testing.T) {
	h := newTestHandler(t)

	w := httptest.NewRecorder()
	h.GetRepos(w, httptest.NewRequest(http.MethodGet, "/api/v1/github/repos", nil))

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestGetUser_Success(t *testing.T) {
	gitServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/user" {
			json.NewEncoder(w).Encode(map[string]string{"login": "mock_user", "avatar_url": "https://example.com/a.png"})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer gitServer.Close()

	oldBaseURL := client.BaseURL
	client.BaseURL = gitServer.URL
	defer func() { client.BaseURL = oldBaseURL }()

	h := newTestHandler(t)
	_ = h.service.SaveOAuthToken(context.Background(), "gho_mock_user_token")

	w := httptest.NewRecorder()
	h.GetUser(w, httptest.NewRequest(http.MethodGet, "/api/v1/github/user", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", w.Code, w.Body.String())
	}

	env := decodeEnvelope(t, w.Body.Bytes())
	data := env["data"].(map[string]interface{})
	if data["login"].(string) != "mock_user" {
		t.Errorf("expected login 'mock_user', got %v", data["login"])
	}
}

func TestGetBranches_Success(t *testing.T) {
	gitServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/mock_owner/mock_repo/branches" {
			json.NewEncoder(w).Encode([]map[string]string{{"name": "main"}, {"name": "develop"}, {"name": "feature-xyz"}})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer gitServer.Close()

	oldBaseURL := client.BaseURL
	client.BaseURL = gitServer.URL
	defer func() { client.BaseURL = oldBaseURL }()

	h := newTestHandler(t)
	_ = h.service.SaveOAuthToken(context.Background(), "gho_mock_branches_token")

	w := httptest.NewRecorder()
	h.GetBranches(w, httptest.NewRequest(http.MethodGet, "/api/v1/github/branches?repo=mock_owner/mock_repo", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", w.Code, w.Body.String())
	}

	env := decodeEnvelope(t, w.Body.Bytes())
	branches := env["data"].([]interface{})
	if len(branches) != 3 {
		t.Fatalf("expected 3 branches, got %d", len(branches))
	}
}

func TestDetectStack_FallbackToken(t *testing.T) {
	gitServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "token gho_mock_token_123456" && auth != "Bearer gho_mock_token_123456" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		json.NewEncoder(w).Encode([]client.GitHubFile{{Name: "go.mod", Type: "file", Size: 100}})
	}))
	defer gitServer.Close()

	oldBaseURL := client.BaseURL
	client.BaseURL = gitServer.URL
	defer func() { client.BaseURL = oldBaseURL }()

	h := newTestHandler(t)
	_ = h.service.SaveOAuthToken(context.Background(), "gho_mock_token_123456")

	info, err := h.service.DetectStack(context.Background(), "user4xn/my-go-app", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Language != "go" {
		t.Errorf("expected language 'go', got %q", info.Language)
	}
}

func TestDisconnect_ClearsToken(t *testing.T) {
	h := newTestHandler(t)
	_ = h.service.SaveOAuthToken(context.Background(), "my-awesome-token")

	w := httptest.NewRecorder()
	h.Disconnect(w, httptest.NewRequest(http.MethodPost, "/api/v1/github/disconnect", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if token := h.service.OAuthToken(context.Background()); token != "" {
		t.Errorf("expected token cleared, got %q", token)
	}
}
