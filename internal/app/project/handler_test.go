package project

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"

	"shipdude/internal/db"
	"shipdude/internal/factory"
	"shipdude/internal/model"
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

func withURLParam(r *http.Request, key, value string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, value)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

func waitForJob(t *testing.T, h *handler, jobID string) []string {
	t.Helper()
	var logs []string
	for i := 0; i < 100; i++ {
		time.Sleep(10 * time.Millisecond)
		job := h.factory.Hub.GetJob(jobID)
		if job == nil {
			continue
		}
		logs = job.GetLogs()
		for _, l := range logs {
			if strings.Contains(l, "Project setup completed successfully") || strings.Contains(l, "[Error]") {
				return logs
			}
		}
	}
	return logs
}

func mustSSHPort(t *testing.T, addr string) (string, int) {
	t.Helper()
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("failed to split addr: %v", err)
	}
	port, _ := strconv.Atoi(portStr)
	return host, port
}

func deployPayload(host string, port int, extra map[string]interface{}) []byte {
	payload := map[string]interface{}{
		"ip_address":      host,
		"ssh_port":        port,
		"ssh_user":        "deployer",
		"auth_method":     "password",
		"password":        "mypassword",
		"github_repo":     "user4xn/my-website",
		"github_branch":   "main",
		"project_name":    "my-website",
		"deployment_path": "/var/www/my-website",
		"stack_type":      "go",
		"stack_version":   "1.23",
		"app_port":        8080,
	}
	for k, v := range extra {
		payload[k] = v
	}
	body, _ := json.Marshal(payload)
	return body
}

func TestDeploy_FullFlow(t *testing.T) {
	hostKey := generateTestHostKey(t)
	sshAddr, sshServer := startMockSSHServer(t, hostKey, "deployer", "mypassword")
	defer sshServer.Close()
	host, port := mustSSHPort(t, sshAddr)

	gitServer := newMockGitHubServer(t, "")
	defer gitServer.Close()
	oldBaseURL := client.BaseURL
	client.BaseURL = gitServer.URL
	defer func() { client.BaseURL = oldBaseURL }()

	h := newTestHandler(t)
	_ = h.factory.Setting.Store(h.factory.InitDB().SQL(), "github_oauth_token", "gho_mock_token")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/deploy", bytes.NewReader(deployPayload(host, port, map[string]interface{}{})))
	w := httptest.NewRecorder()
	h.Deploy(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", w.Code, w.Body.String())
	}

	logs := waitForJob(t, h, "deploy-my-website")
	if !containsLine(logs, "Project setup completed successfully") {
		t.Fatalf("deployment did not finish successfully; last logs: %v", tail(logs))
	}

	projects, _ := h.factory.Project.FindAll(context.Background())
	if len(projects) != 1 || projects[0].ProjectName != "my-website" || projects[0].AppPort != 8080 {
		t.Errorf("project not persisted correctly: %+v", projects)
	}

	servers, _ := h.factory.Server.FindAll(context.Background())
	if len(servers) != 1 || !strings.Contains(servers[0].SSHPrivateKey, "BEGIN RSA PRIVATE KEY") {
		t.Errorf("server not persisted correctly: %+v", servers)
	}
}

func TestDeploy_ManualToken(t *testing.T) {
	hostKey := generateTestHostKey(t)
	sshAddr, sshServer := startMockSSHServer(t, hostKey, "deployer", "mypassword")
	defer sshServer.Close()
	host, port := mustSSHPort(t, sshAddr)

	gitServer := newMockGitHubServer(t, "gho_manual_token_xyz")
	defer gitServer.Close()
	oldBaseURL := client.BaseURL
	client.BaseURL = gitServer.URL
	defer func() { client.BaseURL = oldBaseURL }()

	h := newTestHandler(t)

	body := deployPayload(host, port, map[string]interface{}{"github_token": "gho_manual_token_xyz"})
	w := httptest.NewRecorder()
	h.Deploy(w, httptest.NewRequest(http.MethodPost, "/api/v1/projects/deploy", bytes.NewReader(body)))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", w.Code, w.Body.String())
	}

	if logs := waitForJob(t, h, "deploy-my-website"); !containsLine(logs, "Project setup completed successfully") {
		t.Fatalf("deployment did not finish with manual token; last logs: %v", tail(logs))
	}
}

func TestList_ReturnsProjects(t *testing.T) {
	h := newTestHandler(t)

	serverID, _ := h.factory.Server.Store(h.factory.InitDB().SQL(), model.Server{Name: "127.0.0.1", IPAddress: "127.0.0.1", SSHPort: 22, SSHUser: "root"})
	_, _ = h.factory.Project.Store(h.factory.InitDB().SQL(), model.Project{
		ServerID: serverID, GitHubRepo: "user4xn/my-website", GitHubBranch: "main",
		ProjectName: "my-website", DeploymentPath: "/var/www/my-website", StackType: "go",
		StackVersion: "1.23", AppPort: 8080, Domain: "my-website.com",
	})

	w := httptest.NewRecorder()
	h.List(w, httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var env map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &env)
	projects := env["data"].([]interface{})
	if len(projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(projects))
	}

	first := projects[0].(map[string]interface{})
	if first["project_name"].(string) != "my-website" || first["server_ip"].(string) != "127.0.0.1" {
		t.Errorf("unexpected project payload: %+v", first)
	}
}

func TestDelete_RemovesProject(t *testing.T) {
	h := newTestHandler(t)

	serverID, _ := h.factory.Server.Store(h.factory.InitDB().SQL(), model.Server{Name: "127.0.0.1", IPAddress: "127.0.0.1", SSHPort: 22, SSHUser: "root"})
	projID, _ := h.factory.Project.Store(h.factory.InitDB().SQL(), model.Project{
		ServerID: serverID, GitHubRepo: "user4xn/my-website", GitHubBranch: "main",
		ProjectName: "my-website", DeploymentPath: "/var/www", StackType: "go", StackVersion: "1.23", AppPort: 8080,
	})

	req := withURLParam(httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/projects/%d", projID), nil), "id", strconv.FormatInt(projID, 10))
	w := httptest.NewRecorder()
	h.Delete(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", w.Code, w.Body.String())
	}

	if _, err := h.factory.Project.FindOne(context.Background(), projID); err == nil {
		t.Fatal("expected project to be deleted")
	}
}

func TestRollback_TriggersJob(t *testing.T) {
	h := newTestHandler(t)

	serverID, _ := h.factory.Server.Store(h.factory.InitDB().SQL(), model.Server{Name: "127.0.0.1", IPAddress: "127.0.0.1", SSHPort: 22, SSHUser: "root"})
	projID, _ := h.factory.Project.Store(h.factory.InitDB().SQL(), model.Project{
		ServerID: serverID, GitHubRepo: "user4xn/my-website", GitHubBranch: "main",
		ProjectName: "my-website", DeploymentPath: "/var/www", StackType: "go", StackVersion: "1.23", AppPort: 8080,
	})

	body := `{"target_image":"ghcr.io/user4xn/my-website:sha-abcdef"}`
	req := withURLParam(httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/projects/%d/rollback", projID), bytes.NewBufferString(body)), "id", strconv.FormatInt(projID, 10))
	w := httptest.NewRecorder()
	h.Rollback(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", w.Code, w.Body.String())
	}

	var env map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &env)
	data := env["data"].(map[string]interface{})
	if data["status"].(string) != "triggered" {
		t.Errorf("expected status 'triggered', got %v", data["status"])
	}
}

func TestControl_Restart(t *testing.T) {
	hostKey := generateTestHostKey(t)
	sshAddr, sshServer := startMockSSHServer(t, hostKey, "deployer", "mypassword")
	defer sshServer.Close()
	host, port := mustSSHPort(t, sshAddr)

	h := newTestHandler(t)

	keyPair := mustKeyPair(t)
	serverID, _ := h.factory.Server.Store(h.factory.InitDB().SQL(), model.Server{
		Name: "127.0.0.1", IPAddress: host, SSHPort: port, SSHUser: "deployer", SSHPrivateKey: keyPair,
	})
	projID, _ := h.factory.Project.Store(h.factory.InitDB().SQL(), model.Project{
		ServerID: serverID, ProjectName: "my-website", ActivePort: 8080,
	})

	body := `{"action":"restart"}`
	req := withURLParam(httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/projects/%d/control", projID), bytes.NewBufferString(body)), "id", strconv.FormatInt(projID, 10))
	w := httptest.NewRecorder()
	h.Control(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", w.Code, w.Body.String())
	}

	var env map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &env)
	data := env["data"].(map[string]interface{})
	if data["status"].(string) != "success" || data["action"].(string) != "restart" {
		t.Errorf("expected success restart, got %+v", data)
	}
}

func TestTelemetry_StreamsStats(t *testing.T) {
	hostKey := generateTestHostKey(t)
	sshAddr, sshServer := startMockSSHServer(t, hostKey, "deployer", "mypassword")
	defer sshServer.Close()
	host, port := mustSSHPort(t, sshAddr)

	h := newTestHandler(t)
	keyPair := mustKeyPair(t)
	serverID, _ := h.factory.Server.Store(h.factory.InitDB().SQL(), model.Server{
		Name: "127.0.0.1", IPAddress: host, SSHPort: port, SSHUser: "deployer", SSHPrivateKey: keyPair,
	})
	projID, _ := h.factory.Project.Store(h.factory.InitDB().SQL(), model.Project{
		ServerID: serverID, ProjectName: "my-website", ActivePort: 8080,
	})

	r := chi.NewRouter()
	r.Get("/api/v1/projects/{id}/telemetry", h.Telemetry)
	ts := httptest.NewServer(r)
	defer ts.Close()

	ws, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(ts.URL, "http")+fmt.Sprintf("/api/v1/projects/%d/telemetry", projID), nil)
	if err != nil {
		t.Fatalf("failed to dial websocket: %v", err)
	}
	defer ws.Close()

	var msg map[string]interface{}
	if err := ws.ReadJSON(&msg); err != nil {
		t.Fatalf("failed to read telemetry: %v", err)
	}
	if msg["online"] != true || msg["cpu_perc"] != "12.5" {
		t.Errorf("unexpected telemetry payload: %v", msg)
	}
}

func TestContainerLogs_StreamsLogs(t *testing.T) {
	hostKey := generateTestHostKey(t)
	sshAddr, sshServer := startMockSSHServer(t, hostKey, "deployer", "mypassword")
	defer sshServer.Close()
	host, port := mustSSHPort(t, sshAddr)

	h := newTestHandler(t)
	keyPair := mustKeyPair(t)
	serverID, _ := h.factory.Server.Store(h.factory.InitDB().SQL(), model.Server{
		Name: "127.0.0.1", IPAddress: host, SSHPort: port, SSHUser: "deployer", SSHPrivateKey: keyPair,
	})
	projID, _ := h.factory.Project.Store(h.factory.InitDB().SQL(), model.Project{
		ServerID: serverID, ProjectName: "my-website", ActivePort: 8080,
	})

	r := chi.NewRouter()
	r.Get("/api/v1/projects/{id}/container-logs", h.ContainerLogs)
	ts := httptest.NewServer(r)
	defer ts.Close()

	ws, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(ts.URL, "http")+fmt.Sprintf("/api/v1/projects/%d/container-logs", projID), nil)
	if err != nil {
		t.Fatalf("failed to dial websocket: %v", err)
	}
	defer ws.Close()

	_, message, err := ws.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read log message: %v", err)
	}
	if !strings.Contains(string(message), "Listening for HTTP requests") {
		t.Errorf("unexpected log line: %q", string(message))
	}
}

func containsLine(lines []string, sub string) bool {
	for _, l := range lines {
		if strings.Contains(l, sub) {
			return true
		}
	}
	return false
}

func tail(lines []string) []string {
	if len(lines) <= 3 {
		return lines
	}
	return lines[len(lines)-3:]
}
