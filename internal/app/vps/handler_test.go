package vps

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"shipdude/internal/db"
	"shipdude/internal/factory"
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

func decodeData(t *testing.T, body []byte) map[string]interface{} {
	t.Helper()
	var env map[string]interface{}
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("failed to decode envelope: %v", err)
	}
	data, _ := env["data"].(map[string]interface{})
	return data
}

func TestProbe_Success(t *testing.T) {
	hostKey := generateTestHostKey(t)
	addr, server := startMockSSHServer(t, hostKey, "deployer", "mypassword")
	defer server.Close()

	host, portStr, _ := net.SplitHostPort(addr)
	var port int
	fmt.Sscanf(portStr, "%d", &port)

	h := newTestHandler(t)
	payload, _ := json.Marshal(map[string]interface{}{
		"ip_address":  host,
		"ssh_port":    port,
		"ssh_user":    "deployer",
		"auth_method": "password",
		"password":    "mypassword",
	})

	w := httptest.NewRecorder()
	h.Probe(w, httptest.NewRequest(http.MethodPost, "/api/v1/vps/probe", bytes.NewReader(payload)))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", w.Code, w.Body.String())
	}

	data := decodeData(t, w.Body.Bytes())
	if data["connected"].(bool) != true {
		t.Errorf("expected connected=true, got %v", data["connected"])
	}
	if data["bootstrapped"].(bool) != true {
		t.Errorf("expected bootstrapped=true, got %v", data["bootstrapped"])
	}
}

func TestProbe_RequiresIPAndUser(t *testing.T) {
	h := newTestHandler(t)

	w := httptest.NewRecorder()
	h.Probe(w, httptest.NewRequest(http.MethodPost, "/api/v1/vps/probe", bytes.NewBufferString(`{"ip_address":"1.2.3.4"}`)))

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestLastConnection_Roundtrip(t *testing.T) {
	h := newTestHandler(t)

	// Initially empty.
	wGet := httptest.NewRecorder()
	h.GetLastConnection(wGet, httptest.NewRequest(http.MethodGet, "/api/v1/vps/last-connection", nil))
	if data := decodeData(t, wGet.Body.Bytes()); data["ip_address"].(string) != "" {
		t.Errorf("expected empty IP initially, got %v", data["ip_address"])
	}

	// Save then read back.
	payload, _ := json.Marshal(map[string]interface{}{
		"ip_address":  "192.168.1.100",
		"ssh_port":    2222,
		"ssh_user":    "admin",
		"auth_method": "password",
		"password":    "secret123",
	})
	wPost := httptest.NewRecorder()
	h.PostLastConnection(wPost, httptest.NewRequest(http.MethodPost, "/api/v1/vps/last-connection", bytes.NewReader(payload)))
	if wPost.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", wPost.Code)
	}

	wGet2 := httptest.NewRecorder()
	h.GetLastConnection(wGet2, httptest.NewRequest(http.MethodGet, "/api/v1/vps/last-connection", nil))

	data := decodeData(t, wGet2.Body.Bytes())
	if data["ip_address"].(string) != "192.168.1.100" || data["ssh_port"].(float64) != 2222 || data["password"].(string) != "secret123" {
		t.Errorf("vps connection roundtrip failed, got %+v", data)
	}
}
