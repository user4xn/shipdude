package project

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"shipdude/internal/db"
	"shipdude/internal/factory"
	"shipdude/internal/model"
	"shipdude/pkg/socket"
)

func newTestService(t *testing.T) *service {
	t.Helper()
	database, err := db.Connect(filepath.Join(t.TempDir(), "shipdude.db"))
	if err != nil {
		t.Fatalf("failed to init database: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return NewService(factory.NewFactory(database)).(*service)
}

type cmdRecorder struct {
	mu   sync.Mutex
	cmds []string
}

func (r *cmdRecorder) record(cmd string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cmds = append(r.cmds, cmd)
}

func (r *cmdRecorder) contains(sub string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, cmd := range r.cmds {
		if strings.Contains(cmd, sub) {
			return true
		}
	}
	return false
}

// withMockHook memasang hook perintah SSH untuk durasi satu test.
func withMockHook(t *testing.T, fn func(cmd string) (string, uint32, bool)) {
	t.Helper()
	old := mockCommandHook
	mockCommandHook = fn
	t.Cleanup(func() { mockCommandHook = old })
}

// setupHotSwap menyiapkan service + DB + mock SSH, dengan skenario health check
// dan reload Caddy yang bisa diatur test.
func setupHotSwap(t *testing.T, healthCode string, reloadFails bool) (*service, *model.Project, *model.Server, *cmdRecorder) {
	t.Helper()

	svc := newTestService(t)

	hostKey := generateTestHostKey(t)
	addr, closer := startMockSSHServer(t, hostKey, "deployer", "pw")
	t.Cleanup(func() { closer.Close() })
	host, port := mustSSHPort(t, addr)

	srv := model.Server{
		Name:          "test-node",
		IPAddress:     host,
		SSHPort:       port,
		SSHUser:       "deployer",
		SSHPrivateKey: mustKeyPair(t),
	}
	serverID, err := svc.server.Store(svc.db(), srv)
	if err != nil {
		t.Fatalf("failed to store server: %v", err)
	}
	srv.ID = serverID

	proj := model.Project{
		ServerID:     serverID,
		GitHubRepo:   "user4xn/my-website",
		GitHubBranch: "main",
		ProjectName:  "my-website",
		AppPort:      8080,
		ActivePort:   8080, // container lama di 8080 → target baru di 8081
	}
	projectID, err := svc.project.Store(svc.db(), proj)
	if err != nil {
		t.Fatalf("failed to store project: %v", err)
	}
	proj.ID = projectID

	rec := &cmdRecorder{}
	withMockHook(t, func(cmd string) (string, uint32, bool) {
		rec.record(cmd)
		switch {
		case strings.Contains(cmd, "sudo cat /var/lib/shipdude/caddy/Caddyfile"):
			return "IyBvbGQgY2FkZHlmaWxlCg==", 0, true // base64("# old caddyfile\n")
		case strings.Contains(cmd, "curl -s -o /dev/null -w"):
			return healthCode, 0, true
		case strings.Contains(cmd, "caddy reload"):
			if reloadFails {
				return "caddy: reload failed\n", 1, true
			}
			return "", 0, true
		default:
			return "", 0, true
		}
	})

	oldInterval, oldAttempts := healthCheckInterval, healthCheckAttempts
	healthCheckInterval, healthCheckAttempts = time.Millisecond, 3
	t.Cleanup(func() {
		healthCheckInterval, healthCheckAttempts = oldInterval, oldAttempts
	})

	return svc, &proj, &srv, rec
}

func TestIsHealthyStatus(t *testing.T) {
	healthy := []string{"200", "204", "301", " 200 "}
	for _, code := range healthy {
		if !isHealthyStatus(code) {
			t.Errorf("expected %q healthy", code)
		}
	}

	unhealthy := []string{"", "000", "20", "abc", "404", "500", "502", "199"}
	for _, code := range unhealthy {
		if isHealthyStatus(code) {
			t.Errorf("expected %q NOT healthy", code)
		}
	}
}

// TestHotSwap_CaddyReloadFailureKeepsOldContainer adalah inti perbaikan outage
// senyap: reload gagal harus menggagalkan deploy dan TIDAK menghapus container
// lama, karena Caddy masih menunjuk ke sana.
func TestHotSwap_CaddyReloadFailureKeepsOldContainer(t *testing.T) {
	svc, proj, srv, rec := setupHotSwap(t, "200", true)
	job := socket.NewJob("test-reload-fail")

	err := svc.performHotSwap(proj, srv, "ghcr.io/user4xn/my-website:abcdef1", job)
	if err == nil {
		t.Fatal("expected deploy to fail when caddy reload fails")
	}
	if !strings.Contains(err.Error(), "caddy reload") {
		t.Errorf("expected caddy reload error, got: %v", err)
	}

	if rec.contains("rm -f 'shipdude-my-website-8080'") {
		t.Error("container lama TIDAK boleh dihapus saat caddy reload gagal")
	}
	if !rec.contains("caddy reload") {
		t.Error("reload caddy harus tetap dicoba")
	}
	if !rec.contains("rm -f 'shipdude-my-website-8081'") {
		t.Error("container baru harus dibuang saat reload gagal (belum pernah menerima trafik)")
	}
	if !rec.contains("base64 -d | sudo tee /var/lib/shipdude/caddy/Caddyfile") {
		t.Error("Caddyfile harus dipulihkan ke isi sebelumnya saat reload gagal")
	}
}

// TestHotSwap_HealthCheck500Fails memastikan aplikasi yang membalas 500 tidak
// pernah dipromosikan ke produksi.
func TestHotSwap_HealthCheck500Fails(t *testing.T) {
	svc, proj, srv, rec := setupHotSwap(t, "500", false)
	job := socket.NewJob("test-health-500")

	err := svc.performHotSwap(proj, srv, "ghcr.io/user4xn/my-website:abcdef1", job)
	if err == nil {
		t.Fatal("expected deploy to fail on 500 health check")
	}
	if !strings.Contains(err.Error(), "health check") {
		t.Errorf("expected health check error, got: %v", err)
	}
	if rec.contains("caddy reload") {
		t.Error("caddy tidak boleh direload kalau health check gagal")
	}
	if rec.contains("rm -f 'shipdude-my-website-8080'") {
		t.Error("container lama harus tetap hidup saat health check gagal")
	}
	if !rec.contains("rm -f 'shipdude-my-website-8081'") {
		t.Error("container baru yang gagal sehat harus dibuang")
	}
}

// TestHotSwap_HealthyPromotesAndPrunes adalah jalur bahagia: 200 → Caddy
// direload, container lama baru dipangkas setelah reload sukses.
func TestHotSwap_HealthyPromotesAndPrunes(t *testing.T) {
	svc, proj, srv, rec := setupHotSwap(t, "200", false)
	job := socket.NewJob("test-health-200")

	if err := svc.performHotSwap(proj, srv, "ghcr.io/user4xn/my-website:abcdef1", job); err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if !rec.contains("caddy reload") {
		t.Error("caddy harus direload setelah health check lolos")
	}
	if !rec.contains("rm -f 'shipdude-my-website-8080'") {
		t.Error("container lama harus dipangkas setelah reload sukses")
	}

	updated, err := svc.project.FindOne(context.Background(), proj.ID)
	if err != nil {
		t.Fatalf("failed to reload project: %v", err)
	}
	if updated.ActivePort != 8081 {
		t.Errorf("expected active port 8081 after hot-swap, got %d", updated.ActivePort)
	}
}

// TestHotSwap_QuotesShellArguments memastikan referensi image masuk sebagai satu
// argumen yang di-quote, bukan interpolasi mentah.
func TestHotSwap_QuotesShellArguments(t *testing.T) {
	svc, proj, srv, rec := setupHotSwap(t, "200", false)
	job := socket.NewJob("test-quoting")

	image := "ghcr.io/user4xn/my-website:abcdef1"
	if err := svc.performHotSwap(proj, srv, image, job); err != nil {
		t.Fatalf("expected success, got: %v", err)
	}

	if !rec.contains("docker pull '" + image + "'") {
		t.Error("docker pull harus memakai argumen single-quote")
	}
	if !rec.contains("--name 'shipdude-my-website-8081'") {
		t.Error("docker run harus meng-quote nama container")
	}
}

// TestHotSwap_ImageBerbahayaTetapSatuArgumen memastikan quoting tidak bergantung
// pada validator: image dengan "$(id)" harus tetap masuk sebagai satu argumen
// literal, bukan perintah yang dieksekusi.
func TestHotSwap_ImageBerbahayaTetapSatuArgumen(t *testing.T) {
	svc, proj, srv, rec := setupHotSwap(t, "200", false)
	job := socket.NewJob("test-quoting-danger")

	if err := svc.performHotSwap(proj, srv, "evil:1$(id)", job); err != nil {
		t.Fatalf("expected success, got: %v", err)
	}

	if !rec.contains("docker pull 'evil:1$(id)'") {
		t.Error("image berbahaya harus di-single-quote supaya tidak dieksekusi shell")
	}
	if rec.contains(`docker pull "evil:1$(id)"`) {
		t.Error("double quote tidak menghentikan command substitution")
	}
}

// TestHotSwap_AbortsWhenCaddyfileCaptureFails menutup celah yang ditemukan QA:
// kalau isi Caddyfile lama tidak bisa dibaca, jangan menulis yang baru sama
// sekali, karena kegagalan reload tidak akan bisa dipulihkan.
func TestHotSwap_AbortsWhenCaddyfileCaptureFails(t *testing.T) {
	svc, proj, srv, rec := setupHotSwap(t, "200", false)

	mockCommandHook = func(cmd string) (string, uint32, bool) {
		rec.record(cmd)
		switch {
		case strings.Contains(cmd, "sudo cat /var/lib/shipdude/caddy/Caddyfile"):
			return "permission denied\n", 1, true
		case strings.Contains(cmd, "curl -s -o /dev/null -w"):
			return "200", 0, true
		default:
			return "", 0, true
		}
	}

	job := socket.NewJob("test-capture-fail")
	err := svc.performHotSwap(proj, srv, "ghcr.io/user4xn/my-website:abcdef1", job)
	if err == nil {
		t.Fatal("expected deploy to abort when Caddyfile backup cannot be read")
	}
	if !strings.Contains(err.Error(), "Caddyfile") {
		t.Errorf("expected Caddyfile error, got: %v", err)
	}
	if rec.contains("base64 -d | sudo tee /var/lib/shipdude/caddy/Caddyfile") {
		t.Error("Caddyfile tidak boleh ditulis kalau cadangan gagal diambil")
	}
}
