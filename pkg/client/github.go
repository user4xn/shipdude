package client

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// BaseURL is the GitHub REST API endpoint. It is a package variable so tests
// can point it at an httptest server.
var BaseURL = "https://api.github.com"

// OAuthURL is the GitHub OAuth host. It is a package variable for the same
// reason as BaseURL.
var OAuthURL = "https://github.com"

// TokenResponse is the GitHub OAuth token exchange payload.
type TokenResponse struct {
	AccessToken string `json:"access_token"`
	Scope       string `json:"scope"`
	TokenType   string `json:"token_type"`
	Error       string `json:"error"`
	ErrorDesc   string `json:"error_description"`
}

// Repository is a repository entry returned by the GitHub API.
type Repository struct {
	FullName      string `json:"full_name"`
	HTMLURL       string `json:"html_url"`
	DefaultBranch string `json:"default_branch"`
	Permissions   struct {
		Admin bool `json:"admin"`
		Push  bool `json:"push"`
		Pull  bool `json:"pull"`
	} `json:"permissions"`
}

// PublicKeyResponse is the Actions secrets public key for a repository.
type PublicKeyResponse struct {
	KeyID string `json:"key_id"`
	Key   string `json:"key"`
}

// Branch is a Git branch returned by the GitHub API.
type Branch struct {
	Name string `json:"name"`
}

// UserProfile is the authenticated GitHub user profile.
type UserProfile struct {
	Login     string `json:"login"`
	AvatarURL string `json:"avatar_url"`
}

// StackInfo describes the application profile detected in a repository.
type StackInfo struct {
	Language      string   `json:"language"`
	Version       string   `json:"version"`
	DefaultPort   int      `json:"default_port"`
	DetectedFiles []string `json:"detected_files"`
	HasDockerfile bool     `json:"has_dockerfile"`
}

// GitHubFile is a file object from the GitHub Contents API.
type GitHubFile struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Size    int    `json:"size"`
	Content string `json:"content,omitempty"`
}

func doJSON(method, url, token string, body io.Reader, out any) (*http.Response, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/vnd.github+json")
	if token != "" {
		req.Header.Set("Authorization", "token "+token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}

	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return resp, err
		}
	}

	return resp, nil
}

// ExchangeCodeForToken exchanges an OAuth code for a GitHub access token.
func ExchangeCodeForToken(clientID, clientSecret, code, redirectURI string) (string, error) {
	payload, err := json.Marshal(map[string]string{
		"client_id":     clientID,
		"client_secret": clientSecret,
		"code":          code,
		"redirect_uri":  redirectURI,
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", OAuthURL+"/login/oauth/access_token", strings.NewReader(string(payload)))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github oauth returned status %d: %s", resp.StatusCode, string(respBytes))
	}

	var token TokenResponse
	if err := json.Unmarshal(respBytes, &token); err != nil {
		return "", err
	}

	if token.Error != "" {
		return "", fmt.Errorf("oauth error: %s - %s", token.Error, token.ErrorDesc)
	}

	return token.AccessToken, nil
}

// ListUserRepositories lists repositories where the user has admin or push
// permissions.
func ListUserRepositories(token string) ([]Repository, error) {
	var repos []Repository
	resp, err := doJSON("GET", BaseURL+"/user/repos?per_page=100&sort=updated", token, nil, &repos)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get repos (status %d): %s", resp.StatusCode, string(body))
	}

	filtered := make([]Repository, 0, len(repos))
	for _, r := range repos {
		if r.Permissions.Admin || r.Permissions.Push {
			filtered = append(filtered, r)
		}
	}

	return filtered, nil
}

// GetRepoPublicKey retrieves the Actions secrets public key for a repository.
func GetRepoPublicKey(token, owner, repo string) (string, string, error) {
	var pk PublicKeyResponse
	url := fmt.Sprintf("%s/repos/%s/%s/actions/secrets/public-key", BaseURL, owner, repo)
	resp, err := doJSON("GET", url, token, nil, &pk)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", "", fmt.Errorf("failed to get public key (status %d): %s", resp.StatusCode, string(body))
	}

	return pk.KeyID, pk.Key, nil
}

// UploadRepoSecret uploads an already-encrypted Actions secret to a repository.
func UploadRepoSecret(token, owner, repo, secretName, encryptedValue, keyID string) error {
	payload, err := json.Marshal(map[string]string{
		"encrypted_value": encryptedValue,
		"key_id":          keyID,
	})
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/repos/%s/%s/actions/secrets/%s", BaseURL, owner, repo, secretName)
	resp, err := doJSON("PUT", url, token, strings.NewReader(string(payload)), nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to upload secret (status %d): %s", resp.StatusCode, string(body))
	}

	return nil
}

// CommitFile creates or updates a file in the repository via the Contents API.
func CommitFile(token, owner, repo, branch, path, content, commitMessage string) error {
	url := fmt.Sprintf("%s/repos/%s/%s/contents/%s", BaseURL, owner, repo, path)

	currentSHA := ""
	if check, err := doJSON("GET", url+"?ref="+branch, token, nil, nil); err == nil {
		if check.StatusCode == http.StatusOK {
			var info struct {
				SHA string `json:"sha"`
			}
			if json.NewDecoder(check.Body).Decode(&info) == nil {
				currentSHA = info.SHA
			}
		}
		check.Body.Close()
	}

	payload := map[string]any{
		"message": commitMessage,
		"content": base64.StdEncoding.EncodeToString([]byte(content)),
		"branch":  branch,
		"committer": map[string]string{
			"name":  "ShipDude Agent",
			"email": "agent@shipdude.dev",
		},
	}
	if currentSHA != "" {
		payload["sha"] = currentSHA
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	resp, err := doJSON("PUT", url, token, strings.NewReader(string(bodyBytes)), nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to commit file %s (status %d): %s", path, resp.StatusCode, string(body))
	}

	return nil
}

// CommitWorkflowFile commits the ShipDude deployment workflow YAML.
func CommitWorkflowFile(token, owner, repo, branch, yamlContent string) error {
	return CommitFile(token, owner, repo, branch, ".github/workflows/shipdude.yml", yamlContent, "ci: inject ShipDude auto deployment workflow")
}

// CheckFileExists reports whether a file exists on the given branch.
func CheckFileExists(token, owner, repo, branch, path string) (bool, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/contents/%s?ref=%s", BaseURL, owner, repo, path, branch)
	resp, err := doJSON("GET", url, token, nil, nil)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	default:
		body, _ := io.ReadAll(resp.Body)
		return false, fmt.Errorf("failed to check file existence (status %d): %s", resp.StatusCode, string(body))
	}
}

// ParseRepoOwner splits "owner/repo" into its two components.
func ParseRepoOwner(fullName string) (string, string, error) {
	parts := strings.Split(fullName, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid repository format, expected 'owner/repo', got '%s'", fullName)
	}
	return parts[0], parts[1], nil
}

// GetUserProfile retrieves the authenticated user's profile.
func GetUserProfile(token string) (*UserProfile, error) {
	var profile UserProfile
	resp, err := doJSON("GET", BaseURL+"/user", token, nil, &profile)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get user profile (status %d): %s", resp.StatusCode, string(body))
	}

	return &profile, nil
}

// ListRepoBranches lists all branches in a repository.
func ListRepoBranches(token, owner, repo string) ([]Branch, error) {
	var branches []Branch
	url := fmt.Sprintf("%s/repos/%s/%s/branches?per_page=100", BaseURL, owner, repo)
	resp, err := doJSON("GET", url, token, nil, &branches)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get branches (status %d): %s", resp.StatusCode, string(body))
	}

	return branches, nil
}

// DetectStack inspects a repository's root files to identify its tech stack
// and, when present, the port exposed by its Dockerfile.
func DetectStack(repo string, token string) (*StackInfo, error) {
	repo = strings.TrimPrefix(repo, "https://github.com/")
	repo = strings.TrimSuffix(repo, "/")

	var files []GitHubFile
	resp, err := doJSON("GET", fmt.Sprintf("%s/repos/%s/contents", BaseURL, repo), token, nil, &files)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch repo contents: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github api returned status %d", resp.StatusCode)
	}

	var hasPackageJSON, hasGoMod, hasRequirements, hasDockerfile bool
	var detected []string

	for _, f := range files {
		if f.Type != "file" {
			continue
		}
		switch f.Name {
		case "Dockerfile":
			hasDockerfile = true
			detected = append(detected, "Dockerfile")
		case "package.json":
			hasPackageJSON = true
			detected = append(detected, "package.json")
		case "go.mod":
			hasGoMod = true
			detected = append(detected, "go.mod")
		case "requirements.txt", "pyproject.toml", "Pipfile":
			hasRequirements = true
			detected = append(detected, f.Name)
		}
	}

	info := detectLanguage(repo, token, hasGoMod, hasPackageJSON, hasRequirements)
	info.DetectedFiles = detected
	info.HasDockerfile = hasDockerfile

	if hasDockerfile {
		if content, err := fetchFileContent(repo, "Dockerfile", token); err == nil {
			if matches := regexp.MustCompile(`(?m)^EXPOSE\s+(\d+)`).FindStringSubmatch(content); len(matches) > 1 {
				var port int
				if _, err := fmt.Sscanf(matches[1], "%d", &port); err == nil && port > 0 {
					info.DefaultPort = port
				}
			}
		}
	}

	return info, nil
}

func detectLanguage(repo, token string, hasGoMod, hasPackageJSON, hasRequirements bool) *StackInfo {
	switch {
	case hasGoMod:
		version := "1.23-alpine"
		if content, err := fetchFileContent(repo, "go.mod", token); err == nil {
			if m := regexp.MustCompile(`(?m)^go\s+(\d+\.\d+)`).FindStringSubmatch(content); len(m) > 1 {
				version = m[1] + "-alpine"
			}
		}
		return &StackInfo{Language: "go", Version: version, DefaultPort: 8080}

	case hasPackageJSON:
		version := "20-alpine"
		if content, err := fetchFileContent(repo, "package.json", token); err == nil {
			var pkg map[string]any
			if json.Unmarshal([]byte(content), &pkg) == nil {
				if engines, ok := pkg["engines"].(map[string]any); ok {
					if nodeVer, ok := engines["node"].(string); ok {
						if major := regexp.MustCompile(`\d+`).FindString(nodeVer); major != "" {
							version = major + "-alpine"
						}
					}
				}
			}
		}
		return &StackInfo{Language: "node", Version: version, DefaultPort: 3000}

	case hasRequirements:
		return &StackInfo{Language: "python", Version: "3.11-slim", DefaultPort: 8000}

	default:
		return &StackInfo{Language: "unknown", Version: "latest", DefaultPort: 80}
	}
}

func fetchFileContent(repo string, path string, token string) (string, error) {
	var file GitHubFile
	url := fmt.Sprintf("%s/repos/%s/contents/%s", BaseURL, repo, path)
	resp, err := doJSON("GET", url, token, nil, &file)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status %d", resp.StatusCode)
	}

	raw := strings.NewReplacer("\n", "", "\r", "").Replace(file.Content)
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return "", err
	}

	return string(decoded), nil
}
