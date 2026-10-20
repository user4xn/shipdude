package client

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDetectStack_Go(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/username/go-app/contents" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]GitHubFile{
				{Name: "go.mod", Type: "file", Size: 100},
				{Name: "main.go", Type: "file", Size: 200},
			})
			return
		}
		if r.URL.Path == "/repos/username/go-app/contents/go.mod" {
			raw := "module go-app\n\ngo 1.25\n"
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(GitHubFile{
				Name:    "go.mod",
				Type:    "file",
				Content: base64.StdEncoding.EncodeToString([]byte(raw)),
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	oldBaseURL := BaseURL
	BaseURL = server.URL
	defer func() { BaseURL = oldBaseURL }()

	info, err := DetectStack("username/go-app", "")
	if err != nil {
		t.Fatalf("Unexpected error in DetectStack: %v", err)
	}
	if info.Language != "go" {
		t.Errorf("Expected language 'go', got '%s'", info.Language)
	}
	if info.Version != "1.25-alpine" {
		t.Errorf("Expected version '1.25-alpine', got '%s'", info.Version)
	}
	if info.DefaultPort != 8080 {
		t.Errorf("Expected default port 8080, got %d", info.DefaultPort)
	}
	if len(info.DetectedFiles) != 1 || info.DetectedFiles[0] != "go.mod" {
		t.Errorf("Expected detected files ['go.mod'], got %v", info.DetectedFiles)
	}
}

func TestDetectStack_Node(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/username/node-app/contents" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]GitHubFile{
				{Name: "package.json", Type: "file", Size: 100},
				{Name: "index.js", Type: "file", Size: 200},
			})
			return
		}
		if r.URL.Path == "/repos/username/node-app/contents/package.json" {
			raw := `{"name": "node-app", "engines": {"node": ">=18.0.0"}}`
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(GitHubFile{Name: "package.json", Type: "file", Content: base64.StdEncoding.EncodeToString([]byte(raw))})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	oldBaseURL := BaseURL
	BaseURL = server.URL
	defer func() { BaseURL = oldBaseURL }()

	info, err := DetectStack("username/node-app", "")
	if err != nil {
		t.Fatalf("Unexpected error in DetectStack: %v", err)
	}
	if info.Language != "node" {
		t.Errorf("Expected language 'node', got '%s'", info.Language)
	}
	if info.Version != "18-alpine" {
		t.Errorf("Expected version '18-alpine', got '%s'", info.Version)
	}
	if info.DefaultPort != 3000 {
		t.Errorf("Expected default port 3000, got %d", info.DefaultPort)
	}
}

func TestDetectStack_WithDockerfileExpose(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/username/exposed-app/contents" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]GitHubFile{
				{Name: "package.json", Type: "file", Size: 100},
				{Name: "Dockerfile", Type: "file", Size: 150},
			})
			return
		}
		if r.URL.Path == "/repos/username/exposed-app/contents/package.json" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(GitHubFile{Name: "package.json", Type: "file", Content: base64.StdEncoding.EncodeToString([]byte(`{"name": "exposed-app"}`))})
			return
		}
		if r.URL.Path == "/repos/username/exposed-app/contents/Dockerfile" {
			raw := "FROM node:20-alpine\nEXPOSE 5000\n"
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(GitHubFile{Name: "Dockerfile", Type: "file", Content: base64.StdEncoding.EncodeToString([]byte(raw))})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	oldBaseURL := BaseURL
	BaseURL = server.URL
	defer func() { BaseURL = oldBaseURL }()

	info, err := DetectStack("username/exposed-app", "")
	if err != nil {
		t.Fatalf("Unexpected error in DetectStack: %v", err)
	}
	if !info.HasDockerfile {
		t.Errorf("Expected HasDockerfile to be true")
	}
	if info.DefaultPort != 5000 {
		t.Errorf("Expected default port 5000 (from EXPOSE), got %d", info.DefaultPort)
	}
}
