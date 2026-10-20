package github

import (
	"context"
	"database/sql"
	"sync"

	"shipdude/internal/factory"
	"shipdude/internal/repository"
	"shipdude/pkg/client"
	"shipdude/pkg/config"
	"shipdude/pkg/consts"
	"shipdude/pkg/crypto"
	"shipdude/pkg/util"
)

// Service is the business logic contract for GitHub integration.
type Service interface {
	GetSettings(ctx context.Context) (clientID string, configured bool, err error)
	SaveSettings(ctx context.Context, clientID, clientSecret string) error
	ClientID(ctx context.Context) string
	ClientSecret(ctx context.Context) string
	OAuthToken(ctx context.Context) string
	SaveOAuthToken(ctx context.Context, token string) error
	RegisterOAuthState(state string)
	ConsumeOAuthState(state string) bool
	Disconnect(ctx context.Context) error
	GetUserProfile(ctx context.Context) (*client.UserProfile, error)
	ListRepositories(ctx context.Context) ([]client.Repository, error)
	ListBranches(ctx context.Context, repo, token string) ([]client.Branch, error)
	DetectStack(ctx context.Context, repo, token string) (*client.StackInfo, error)
	UploadSecret(ctx context.Context, token, owner, repo, name, value string) error
	CheckFileExists(ctx context.Context, token, owner, repo, branch, path string) (bool, error)
	CommitWorkflow(ctx context.Context, token, owner, repo, branch, yaml string) error
	IsLocalhost(url string) bool
}

type service struct {
	factory *factory.Factory
	setting repository.Setting
	states  sync.Map
}

// NewService builds the GitHub service backed by the factory.
func NewService(f *factory.Factory) Service {
	return &service{
		factory: f,
		setting: f.Setting,
	}
}

func (s *service) GetSettings(ctx context.Context) (string, bool, error) {
	clientID, err := s.setting.FindValue(ctx, consts.SettingGitHubClientID.String())
	if err != nil {
		return "", false, util.NewInternalServerError("Failed to load GitHub settings", err)
	}

	clientID = util.FirstNonEmpty(clientID, config.GitHubClientID())
	return clientID, clientID != "", nil
}

func (s *service) SaveSettings(ctx context.Context, clientID, clientSecret string) error {
	if clientID == "" || clientSecret == "" {
		return util.NewBadRequestError("Client ID and Client Secret are required", nil)
	}

	if err := s.setting.Store(s.db(), consts.SettingGitHubClientID.String(), clientID); err != nil {
		return util.NewInternalServerError("Failed to save GitHub client ID", err)
	}

	if err := s.setting.Store(s.db(), consts.SettingGitHubClientSecret.String(), clientSecret); err != nil {
		return util.NewInternalServerError("Failed to save GitHub client secret", err)
	}

	return nil
}

func (s *service) ClientID(ctx context.Context) string {
	value, _ := s.setting.FindValue(ctx, consts.SettingGitHubClientID.String())
	return util.FirstNonEmpty(value, config.GitHubClientID())
}

func (s *service) ClientSecret(ctx context.Context) string {
	value, _ := s.setting.FindValue(ctx, consts.SettingGitHubClientSecret.String())
	return util.FirstNonEmpty(value, config.GitHubClientSecret())
}

func (s *service) OAuthToken(ctx context.Context) string {
	value, _ := s.setting.FindValue(ctx, consts.SettingGitHubOAuthToken.String())
	return util.FirstNonEmpty(value, config.GitHubOAuthToken())
}

func (s *service) SaveOAuthToken(ctx context.Context, token string) error {
	if err := s.setting.Store(s.db(), consts.SettingGitHubOAuthToken.String(), token); err != nil {
		return util.NewInternalServerError("Failed to save GitHub OAuth token", err)
	}
	return nil
}

func (s *service) Disconnect(ctx context.Context) error {
	if err := s.setting.Store(s.db(), consts.SettingGitHubOAuthToken.String(), ""); err != nil {
		return util.NewInternalServerError("Failed to disconnect from GitHub", err)
	}
	return nil
}

// RegisterOAuthState remembers a one-time CSRF state parameter.
func (s *service) RegisterOAuthState(state string) {
	s.states.Store(state, true)
}

// ConsumeOAuthState validates and deletes a state parameter, preventing replay.
func (s *service) ConsumeOAuthState(state string) bool {
	_, found := s.states.LoadAndDelete(state)
	return found
}

func (s *service) GetUserProfile(ctx context.Context) (*client.UserProfile, error) {
	token := s.OAuthToken(ctx)
	if token == "" {
		return nil, util.NewUnauthorizedError("GitHub OAuth login is required", nil)
	}

	profile, err := client.GetUserProfile(token)
	if err != nil {
		return nil, util.NewInternalServerError("Failed to fetch GitHub profile", err)
	}
	return profile, nil
}

func (s *service) ListRepositories(ctx context.Context) ([]client.Repository, error) {
	token := s.OAuthToken(ctx)
	if token == "" {
		return nil, util.NewUnauthorizedError("GitHub OAuth login is required", nil)
	}

	repos, err := client.ListUserRepositories(token)
	if err != nil {
		return nil, util.NewInternalServerError("Failed to retrieve repositories", err)
	}
	return repos, nil
}

func (s *service) ListBranches(ctx context.Context, repo, token string) ([]client.Branch, error) {
	if repo == "" {
		return nil, util.NewBadRequestError("Repository parameter 'repo' is required", nil)
	}

	owner, repoName, err := client.ParseRepoOwner(repo)
	if err != nil {
		return nil, util.NewBadRequestError("Invalid repository format, expected 'owner/repo'", err)
	}

	token = util.FirstNonEmpty(token, s.OAuthToken(ctx))
	if token == "" {
		return nil, util.NewUnauthorizedError("GitHub authorization token required", nil)
	}

	branches, err := client.ListRepoBranches(token, owner, repoName)
	if err != nil {
		return nil, util.NewInternalServerError("Failed to retrieve branches", err)
	}
	return branches, nil
}

func (s *service) DetectStack(ctx context.Context, repo, token string) (*client.StackInfo, error) {
	if repo == "" {
		return nil, util.NewBadRequestError("Repository URL or name is required", nil)
	}

	token = util.FirstNonEmpty(token, s.OAuthToken(ctx))

	info, err := client.DetectStack(repo, token)
	if err != nil {
		return nil, util.NewInternalServerError("Failed to detect repository stack", err)
	}
	return info, nil
}

func (s *service) UploadSecret(ctx context.Context, token, owner, repo, name, value string) error {
	keyID, publicKey, err := client.GetRepoPublicKey(token, owner, repo)
	if err != nil {
		return util.NewInternalServerError("Failed to fetch repository public key", err)
	}

	encrypted, err := crypto.EncryptSecret(publicKey, value)
	if err != nil {
		return util.NewInternalServerError("Failed to encrypt repository secret", err)
	}

	if err := client.UploadRepoSecret(token, owner, repo, name, encrypted, keyID); err != nil {
		return util.NewInternalServerError("Failed to upload repository secret", err)
	}
	return nil
}

func (s *service) CheckFileExists(ctx context.Context, token, owner, repo, branch, path string) (bool, error) {
	exists, err := client.CheckFileExists(token, owner, repo, branch, path)
	if err != nil {
		return false, util.NewInternalServerError("Failed to check repository file", err)
	}
	return exists, nil
}

func (s *service) CommitWorkflow(ctx context.Context, token, owner, repo, branch, yaml string) error {
	if err := client.CommitWorkflowFile(token, owner, repo, branch, yaml); err != nil {
		return util.NewInternalServerError("Failed to commit workflow template", err)
	}
	return nil
}

func (s *service) IsLocalhost(url string) bool {
	return util.IsLocalhostURL(url)
}

func (s *service) db() *sql.DB {
	return s.factory.InitDB().SQL()
}
