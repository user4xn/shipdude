package repository

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"shipdude/internal/db"
	"shipdude/internal/model"
)

func newTestDB(t *testing.T) *db.DB {
	t.Helper()
	database, err := db.Connect(filepath.Join(t.TempDir(), "shipdude.db"))
	if err != nil {
		t.Fatalf("failed to init database: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func TestServerRepository_EncryptsAtRest(t *testing.T) {
	database := newTestDB(t)
	repo := NewServerRepository(database)
	ctx := context.Background()

	const privateKey = "-----BEGIN OPENSSH PRIVATE KEY-----\nMOCKKEYCONTENT\n-----END OPENSSH PRIVATE KEY-----"

	id, err := repo.Store(database.SQL(), model.Server{
		Name:          "Production VPS",
		IPAddress:     "103.187.22.44",
		SSHPort:       22,
		SSHUser:       "deployer",
		SSHPrivateKey: privateKey,
	})
	if err != nil {
		t.Fatalf("failed to store server: %v", err)
	}

	// Raw column must be encrypted, not plaintext.
	var stored string
	if err := database.SQL().QueryRow(`SELECT ssh_private_key FROM servers WHERE id = ?`, id).Scan(&stored); err != nil {
		t.Fatalf("failed to read raw key: %v", err)
	}
	if stored == privateKey {
		t.Errorf("expected ssh key to be encrypted at rest")
	}

	srv, err := repo.FindOne(ctx, id)
	if err != nil {
		t.Fatalf("failed to find server: %v", err)
	}
	if srv.SSHPrivateKey != privateKey {
		t.Errorf("expected decrypted key to round-trip")
	}

	list, err := repo.FindAll(ctx)
	if err != nil {
		t.Fatalf("failed to list servers: %v", err)
	}
	if len(list) != 1 || list[0].SSHPrivateKey != privateKey {
		t.Errorf("expected one server with decrypted key, got %+v", list)
	}
}

func TestProjectRepository_JoinsServerIP(t *testing.T) {
	database := newTestDB(t)
	servers := NewServerRepository(database)
	projects := NewProjectRepository(database)
	ctx := context.Background()

	serverID, _ := servers.Store(database.SQL(), model.Server{Name: "node", IPAddress: "127.0.0.1", SSHPort: 22, SSHUser: "root"})

	projectID, err := projects.Store(database.SQL(), model.Project{
		ServerID:     serverID,
		GitHubRepo:   "user4xn/my-website",
		GitHubBranch: "main",
		ProjectName:  "my-website",
		AppPort:      8080,
		Domain:       "my-website.com",
	})
	if err != nil {
		t.Fatalf("failed to store project: %v", err)
	}

	proj, err := projects.FindOne(ctx, projectID)
	if err != nil {
		t.Fatalf("failed to find project: %v", err)
	}
	if proj.ServerIP != "127.0.0.1" {
		t.Errorf("expected joined server IP 127.0.0.1, got %q", proj.ServerIP)
	}

	if err := projects.UpdateActiveState(database.SQL(), projectID, 3001, "ghcr.io/user4xn/my-website:abc"); err != nil {
		t.Fatalf("failed to update active state: %v", err)
	}
	proj, _ = projects.FindOne(ctx, projectID)
	if proj.ActivePort != 3001 || proj.ActiveImage == "" {
		t.Errorf("expected active state to be updated, got %+v", proj)
	}

	byName, err := projects.FindOneByName(ctx, "my-website")
	if err != nil || byName.ID != projectID {
		t.Errorf("expected FindOneByName to return project %d, got %+v (err %v)", projectID, byName, err)
	}

	if err := projects.DeleteOne(database.SQL(), projectID); err != nil {
		t.Fatalf("failed to delete project: %v", err)
	}
	if _, err := projects.FindOne(ctx, projectID); err != sql.ErrNoRows {
		t.Errorf("expected sql.ErrNoRows after delete, got %v", err)
	}
}

func TestSettingRepository_EncryptsValues(t *testing.T) {
	database := newTestDB(t)
	repo := NewSettingRepository(database)
	ctx := context.Background()

	if err := repo.Store(database.SQL(), "github_oauth_token", "github_pat_1234567"); err != nil {
		t.Fatalf("failed to store setting: %v", err)
	}

	var stored string
	if err := database.SQL().QueryRow(`SELECT value FROM settings WHERE key = ?`, "github_oauth_token").Scan(&stored); err != nil {
		t.Fatalf("failed to read raw setting: %v", err)
	}
	if stored == "github_pat_1234567" {
		t.Errorf("expected setting value to be encrypted at rest")
	}

	value, err := repo.FindValue(ctx, "github_oauth_token")
	if err != nil {
		t.Fatalf("failed to read setting: %v", err)
	}
	if value != "github_pat_1234567" {
		t.Errorf("expected decrypted value, got %q", value)
	}

	missing, err := repo.FindValue(ctx, "non_existent")
	if err != nil {
		t.Fatalf("unexpected error for missing key: %v", err)
	}
	if missing != "" {
		t.Errorf("expected empty string for missing key, got %q", missing)
	}
}
