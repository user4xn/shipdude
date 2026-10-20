package project

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"shipdude/internal/app/github"
	"shipdude/internal/factory"
	"shipdude/internal/model"
	"shipdude/internal/repository"
	"shipdude/pkg/client"
	"shipdude/pkg/config"
	"shipdude/pkg/consts"
	"shipdude/pkg/crypto"
	"shipdude/pkg/socket"
	"shipdude/pkg/util"
)

// Service is the business logic contract for projects.
type Service interface {
	FindAll(ctx context.Context) ([]model.Project, error)
	FindOne(ctx context.Context, id int64) (*model.Project, error)
	Deploy(req DeployRequest, serverURL string) (jobID string, err error)
	Trigger(repo, sha string) (jobID string, projectID int64, err error)
	Rollback(ctx context.Context, id int64, targetImage string) (jobID string, err error)
	Control(ctx context.Context, id int64, action consts.ContainerAction) (string, error)
	Delete(ctx context.Context, id int64) error
	StreamProjectJob(w socket.WriterFunc, r socket.ReadFunc, projectID int64) error
}

// DeployRequest is the normalized deployment setup input after defaults are
// applied by the handler.
type DeployRequest struct {
	IPAddress      string
	SSHPort        int
	SSHUser        string
	AuthMethod     consts.AuthMethod
	Password       string
	PrivateKey     string
	GitHubRepo     string
	GitHubBranch   string
	ProjectName    string
	DeploymentPath string
	StackType      string
	StackVersion   string
	AppPort        int
	GitHubToken    string
	Domain         string
}

type service struct {
	factory *factory.Factory
	project repository.Project
	server  repository.Server
	github  github.Service
	hub     *socket.Hub
}

// NewService builds the project service backed by the factory.
func NewService(f *factory.Factory) Service {
	return &service{
		factory: f,
		project: f.Project,
		server:  f.Server,
		github:  github.NewService(f),
		hub:     f.Hub,
	}
}

// FindAll returns every project for the dashboard.
func (s *service) FindAll(ctx context.Context) ([]model.Project, error) {
	list, err := s.project.FindAll(ctx)
	if err != nil {
		return nil, util.NewInternalServerError("Failed to find projects", err)
	}
	return list, nil
}

// FindOne returns a project or a typed not-found error.
func (s *service) FindOne(ctx context.Context, id int64) (*model.Project, error) {
	proj, err := s.project.FindOne(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, util.NewNotFoundError("Project not found", err)
		}
		return nil, util.NewInternalServerError("Failed to find project", err)
	}
	return proj, nil
}

// Deploy runs the one-click CI/CD setup pipeline in the background.
func (s *service) Deploy(req DeployRequest, serverURL string) (string, error) {
	// Validasi di depan: input buruk ditolak sebagai 400 dan tidak pernah
	// menyentuh SSH, GitHub, maupun perintah shell di VPS target.
	if err := ValidateDeployRequest(req); err != nil {
		return "", err
	}

	token := req.GitHubToken
	if token == "" {
		token = s.github.OAuthToken(context.Background())
	}
	if token == "" {
		return "", util.NewUnauthorizedError("GitHub OAuth login required to deploy projects", nil)
	}

	jobID := consts.JobID(consts.JobDeploy, req.ProjectName)
	job := s.hub.GetOrCreateJob(jobID)

	go s.runDeploy(job, req, token, serverURL)

	return jobID, nil
}

func (s *service) runDeploy(job *socket.Job, req DeployRequest, token, serverURL string) {
	job.Reset()
	job.AddLog("[System] Starting project deployment setup...")

	job.AddLog("[System] 1. Generating SSH deployment key pair...")
	keyPair, err := crypto.GenerateSSHKeyPair()
	if err != nil {
		job.AddLog("[Error] Failed to generate deployment keys: " + err.Error())
		return
	}

	job.AddLog("[System] 2. Injecting public key to target VPS authorized_keys...")
	probeCreds := client.SSHCredentials{
		IPAddress:  req.IPAddress,
		Port:       req.SSHPort,
		User:       req.SSHUser,
		AuthMethod: req.AuthMethod,
		Password:   req.Password,
		PrivateKey: req.PrivateKey,
	}
	if err := client.AddAuthorizedKey(probeCreds, keyPair.PublicSSHKey); err != nil {
		job.AddLog("[Error] Failed to inject deployment public key: " + err.Error())
		return
	}

	job.AddLog("[System] 3. Saving server configuration to database...")
	serverID, err := s.server.Store(s.db(), model.Server{
		Name:          req.ProjectName + "-node",
		IPAddress:     req.IPAddress,
		SSHPort:       req.SSHPort,
		SSHUser:       req.SSHUser,
		SSHPrivateKey: keyPair.PrivateKeyPEM,
	})
	if err != nil {
		job.AddLog("[Error] Failed to save server metadata: " + err.Error())
		return
	}

	job.AddLog("[GitHub] 4. Uploading SSH private key as GitHub Actions secret...")
	owner, repo, err := client.ParseRepoOwner(req.GitHubRepo)
	if err != nil {
		job.AddLog("[Error] Failed to parse repo owner: " + err.Error())
		return
	}

	if err := s.github.UploadSecret(context.Background(), token, owner, repo, "SHIPDUDE_SSH_PRIVATE_KEY", keyPair.PrivateKeyPEM); err != nil {
		job.AddLog("[Error] Failed to configure GitHub Actions secrets: " + err.Error())
		return
	}

	job.AddLog("[GitHub] 4.5. Checking if Dockerfile exists in repository...")
	hasDockerfile, err := s.github.CheckFileExists(context.Background(), token, owner, repo, req.GitHubBranch, "Dockerfile")
	if err != nil {
		job.AddLog("[Error] Failed to verify if Dockerfile exists in repository: " + err.Error())
		return
	}

	if !hasDockerfile {
		logMissingDockerfile(job)
		return
	}

	job.AddLog("[GitHub] Existing Dockerfile verified successfully!")

	job.AddLog("[GitHub] 5. Committing CI/CD workflow template to repository...")
	if s.github.IsLocalhost(serverURL) {
		job.AddLog("[Warning] ShipDude is running on localhost (" + serverURL + "). The target VPS will not be able to trigger hot-swap deployments automatically. Expose ShipDude via ngrok/tunnel, or run it on a public IP.")
	}

	yaml := client.RenderWorkflowYAML(client.WorkflowSpec{
		Branch:      req.GitHubBranch,
		Host:        req.IPAddress,
		Port:        req.SSHPort,
		User:        req.SSHUser,
		ProjectName: req.ProjectName,
		ServerURL:   serverURL,
	})
	if err := s.github.CommitWorkflow(context.Background(), token, owner, repo, req.GitHubBranch, yaml); err != nil {
		job.AddLog("[Error] Failed to commit workflow template: " + err.Error())
		return
	}

	job.AddLog("[System] 6. Persisting project metadata in database...")
	projID, err := s.persistProject(serverID, req, job)
	if err != nil {
		return
	}

	job.AddLog(fmt.Sprintf("[System] Project setup completed successfully with Server ID: %d and Project ID: %d!", serverID, projID))
	logNextSteps(job)
}

func (s *service) persistProject(serverID int64, req DeployRequest, job *socket.Job) (int64, error) {
	project := model.Project{
		ServerID:       serverID,
		GitHubRepo:     req.GitHubRepo,
		GitHubBranch:   req.GitHubBranch,
		ProjectName:    req.ProjectName,
		DeploymentPath: req.DeploymentPath,
		StackType:      req.StackType,
		StackVersion:   req.StackVersion,
		AppPort:        req.AppPort,
		Domain:         req.Domain,
	}

	existing, err := s.project.FindOneByName(context.Background(), req.ProjectName)
	if err == nil {
		project.ID = existing.ID
		if err := s.project.UpdateOne(s.db(), project); err != nil {
			job.AddLog("[Error] Failed to update project metadata: " + err.Error())
			return 0, err
		}
		job.AddLog("[System] Existing project configuration updated in database.")
		return existing.ID, nil
	}

	projID, err := s.project.Store(s.db(), project)
	if err != nil {
		job.AddLog("[Error] Failed to persist project metadata: " + err.Error())
		return 0, err
	}
	job.AddLog("[System] New project configuration created in database.")
	return projID, nil
}

// Trigger reacts to a GitHub Actions completion by hot-swapping the image.
func (s *service) Trigger(repo, sha string) (string, int64, error) {
	if err := ValidateRepoRef(repo); err != nil {
		return "", 0, err
	}
	if sha == "" {
		sha = "latest"
	}
	if err := ValidateSHA(sha); err != nil {
		return "", 0, err
	}

	proj, err := s.project.FindOneByRepo(context.Background(), repo)
	if err != nil {
		return "", 0, util.NewNotFoundError("Project not found for repo: "+repo, err)
	}

	srv, err := s.server.FindOne(context.Background(), proj.ServerID)
	if err != nil {
		return "", 0, util.NewInternalServerError("Failed to load project server details", err)
	}

	jobID := consts.JobIDFromInt(consts.JobProject, proj.ID)
	job := s.hub.GetOrCreateJob(jobID)

	go func() {
		job.Reset()
		image := fmt.Sprintf("ghcr.io/%s:%s", strings.ToLower(proj.GitHubRepo), sha)
		if err := s.performHotSwap(proj, srv, image, job); err != nil {
			job.AddLog("[Error] Deployment failed: " + err.Error())
		}
	}()

	return jobID, proj.ID, nil
}

// Rollback redeploys a previously pushed image in the background.
func (s *service) Rollback(ctx context.Context, id int64, targetImage string) (string, error) {
	if err := ValidateImageRef(targetImage); err != nil {
		return "", err
	}

	proj, err := s.FindOne(ctx, id)
	if err != nil {
		return "", err
	}

	srv, err := s.server.FindOne(ctx, proj.ServerID)
	if err != nil {
		return "", util.NewInternalServerError("Failed to load project server details", err)
	}

	jobID := consts.JobIDFromInt(consts.JobProject, proj.ID)
	job := s.hub.GetOrCreateJob(jobID)

	go func() {
		job.Reset()
		if err := s.performHotSwap(proj, srv, targetImage, job); err != nil {
			job.AddLog("[Error] Rollback failed: " + err.Error())
		}
	}()

	return jobID, nil
}

// Control starts, stops or restarts a project's container.
func (s *service) Control(ctx context.Context, id int64, action consts.ContainerAction) (string, error) {
	if !action.IsValid() {
		return "", util.NewBadRequestError("Invalid action; must be start, stop, or restart", nil)
	}

	proj, err := s.FindOne(ctx, id)
	if err != nil {
		return "", err
	}

	if proj.ActivePort == 0 {
		return "", util.NewBadRequestError("Project has no active deployment to control", nil)
	}

	srv, err := s.server.FindOne(ctx, proj.ServerID)
	if err != nil {
		return "", util.NewInternalServerError("Failed to load project server details", err)
	}

	sshClient, err := client.NewSSHClient(toSSHCreds(srv))
	if err != nil {
		return "", util.NewInternalServerError("Failed to connect to VPS", err)
	}
	defer sshClient.Close()

	containerName := client.ContainerName(proj.ProjectName, proj.ActivePort)
	cmd := fmt.Sprintf("docker %s %s", action, containerName)

	var output []string
	if err := client.RunCommandStream(sshClient, cmd, func(line string) {
		output = append(output, line)
	}); err != nil {
		return "", util.NewInternalServerError(fmt.Sprintf("Failed to %s container", action), err)
	}

	return strings.Join(output, "\n"), nil
}

// Delete removes a project configuration after verifying it exists.
func (s *service) Delete(ctx context.Context, id int64) error {
	if _, err := s.FindOne(ctx, id); err != nil {
		return err
	}

	if err := s.project.DeleteOne(s.db(), id); err != nil {
		return util.NewInternalServerError("Failed to delete project", err)
	}
	return nil
}

// StreamProjectJob replays and tails a project job's logs to a WebSocket.
func (s *service) StreamProjectJob(w socket.WriterFunc, r socket.ReadFunc, projectID int64) error {
	jobID := consts.JobIDFromInt(consts.JobProject, projectID)
	job := s.hub.GetOrCreateJob(jobID)

	for _, line := range job.GetLogs() {
		if err := w(line); err != nil {
			return err
		}
	}

	return socket.Tail(job, w, r)
}

// resolveDeployDefaults applies the same defaults the legacy handler used.
func resolveDeployDefaults(req *DeployRequest) {
	if req.GitHubBranch == "" {
		req.GitHubBranch = "main"
	}
	if req.SSHPort == 0 {
		req.SSHPort = config.DefaultSSHPort()
	}
	if req.SSHUser == "" {
		req.SSHUser = config.DefaultSSHUser()
	}
}

func logMissingDockerfile(job *socket.Job) {
	job.AddLog("[Error] Dockerfile not found in repository root. A valid Dockerfile is required for deployment.")
	job.AddLog("[System] === REPOSITORY ELIGIBILITY REQUIREMENTS ===")
	job.AddLog("[System] 1. A valid 'Dockerfile' must exist in the root of the repository.")
	job.AddLog("[System] 2. The Dockerfile must build successfully and run a server/service.")
	job.AddLog("[System] 3. The service must listen and serve on the configured project port (e.g., PORT 3000) so that Caddy can proxy the traffic.")
	job.AddLog("[System] ============================================")
	job.AddLog("[Error] Project setup terminated due to missing requirements.")
}

func logNextSteps(job *socket.Job) {
	job.AddLog("")
	job.AddLog("[Next Steps] ===============================================")
	job.AddLog("[Next Steps] 1. Go to your GitHub repository -> Actions tab.")
	job.AddLog("[Next Steps] 2. Monitor the 'ShipDude CD' workflow run.")
	job.AddLog("[Next Steps] 3. Once it finishes, it will trigger the hot-swap deploy on your VPS.")
	job.AddLog("[Next Steps] 4. Your container status will automatically update to 'Active' here!")
	job.AddLog("[Next Steps] ===============================================")
}
