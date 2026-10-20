package project

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	cryptossh "golang.org/x/crypto/ssh"

	app "shipdude/internal/app"
	"shipdude/internal/dto"
	"shipdude/internal/factory"
	"shipdude/internal/model"
	"shipdude/pkg/client"
	"shipdude/pkg/consts"
	"shipdude/pkg/util"
)

type handler struct {
	service Service
	factory *factory.Factory
}

// NewHandler builds the project HTTP handler.
func NewHandler(f *factory.Factory) *handler {
	return &handler{service: NewService(f), factory: f}
}

// List returns every configured project.
func (h *handler) List(w http.ResponseWriter, r *http.Request) {
	projects, err := h.service.FindAll(r.Context())
	if err != nil {
		util.RespondError(w, err)
		return
	}

	util.RespondJSON(w, http.StatusOK, "Successfully retrieved projects", toResponses(projects))
}

// Deploy starts the one-click CI/CD setup pipeline.
func (h *handler) Deploy(w http.ResponseWriter, r *http.Request) {
	var body dto.PayloadProjectDeploy
	if err := util.DecodeJSON(r, &body); err != nil {
		util.RespondError(w, util.NewBadRequestError("Invalid request payload", err))
		return
	}

	req := DeployRequest{
		IPAddress:      body.IPAddress,
		SSHPort:        body.SSHPort,
		SSHUser:        body.SSHUser,
		AuthMethod:     consts.AuthMethod(body.AuthMethod),
		Password:       body.Password,
		PrivateKey:     body.PrivateKey,
		GitHubRepo:     body.GitHubRepo,
		GitHubBranch:   body.GitHubBranch,
		ProjectName:    body.ProjectName,
		DeploymentPath: body.DeploymentPath,
		StackType:      body.StackType,
		StackVersion:   body.StackVersion,
		AppPort:        body.AppPort,
		GitHubToken:    body.GitHubToken,
		Domain:         body.Domain,
	}
	resolveDeployDefaults(&req)

	var missing []string
	if req.IPAddress == "" {
		missing = append(missing, "host IP")
	}
	if req.GitHubRepo == "" {
		missing = append(missing, "repository")
	}
	if req.ProjectName == "" {
		missing = append(missing, "project name")
	}
	if len(missing) > 0 {
		util.RespondError(w, util.NewBadRequestError("Missing required fields: "+strings.Join(missing, ", "), nil))
		return
	}

	jobID, err := h.service.Deploy(req, util.ServerURL(r))
	if err != nil {
		util.RespondError(w, err)
		return
	}

	util.RespondJSON(w, http.StatusOK, "Successfully started deployment", &dto.ResponseJobAck{
		Status: string(consts.StatusStarted),
		JobID:  jobID,
	})
}

// DeployLogs streams the setup job logs for a project name.
func (h *handler) DeployLogs(w http.ResponseWriter, r *http.Request) {
	projectName := r.URL.Query().Get("project_name")
	jobID := consts.JobID(consts.JobDeploy, projectName)
	app.StreamJobLogs(w, r, h.factory.Hub, jobID)
}

// Trigger handles the GitHub Actions callback that starts a hot-swap deploy.
func (h *handler) Trigger(w http.ResponseWriter, r *http.Request) {
	repo := r.URL.Query().Get("repo")

	var body dto.PayloadProjectTrigger
	if err := util.DecodeJSON(r, &body); err != nil && r.ContentLength != 0 {
		util.RespondError(w, util.NewBadRequestError("Invalid request payload", err))
		return
	}

	jobID, projectID, err := h.service.Trigger(repo, body.SHA)
	if err != nil {
		util.RespondError(w, err)
		return
	}

	util.RespondJSON(w, http.StatusOK, "Successfully triggered deployment", &dto.ResponseJobAck{
		Status:    string(consts.StatusTriggered),
		JobID:     jobID,
		ProjectID: projectID,
	})
}

// Rollback redeploys a previously pushed image.
func (h *handler) Rollback(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		util.RespondError(w, err)
		return
	}

	var body dto.PayloadProjectRollback
	if err := util.DecodeJSON(r, &body); err != nil {
		util.RespondError(w, util.NewBadRequestError("Invalid request payload", err))
		return
	}

	jobID, err := h.service.Rollback(r.Context(), id, body.TargetImage)
	if err != nil {
		util.RespondError(w, err)
		return
	}

	util.RespondJSON(w, http.StatusOK, "Successfully triggered rollback", &dto.ResponseJobAck{
		Status:    string(consts.StatusTriggered),
		JobID:     jobID,
		ProjectID: id,
	})
}

// Delete removes a project configuration.
func (h *handler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		util.RespondError(w, err)
		return
	}

	if err := h.service.Delete(r.Context(), id); err != nil {
		util.RespondError(w, err)
		return
	}

	util.RespondJSON(w, http.StatusOK, "Successfully deleted project", &dto.ResponseProjectDelete{
		Status: string(consts.StatusDeleted),
	})
}

// Control starts, stops or restarts a project container.
func (h *handler) Control(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		util.RespondError(w, err)
		return
	}

	var body dto.PayloadProjectControl
	if err := util.DecodeJSON(r, &body); err != nil {
		util.RespondError(w, util.NewBadRequestError("Invalid request payload", err))
		return
	}

	action := consts.ContainerAction(body.Action)
	output, err := h.service.Control(r.Context(), id, action)
	if err != nil {
		util.RespondError(w, err)
		return
	}

	util.RespondJSON(w, http.StatusOK, "Successfully controlled container", &dto.ResponseProjectControl{
		Status: string(consts.StatusSuccess),
		Action: body.Action,
		Output: output,
	})
}

// Logs streams a project job's logs over a WebSocket connection.
func (h *handler) Logs(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		util.RespondError(w, err)
		return
	}

	conn, err := app.Upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	write := func(line string) error {
		return conn.WriteMessage(websocket.TextMessage, []byte(line))
	}
	read := func() error {
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return err
			}
		}
	}

	if err := h.service.StreamProjectJob(write, read, id); err != nil {
		return
	}
}

// Telemetry streams CPU/memory usage of a project's container.
func (h *handler) Telemetry(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		util.RespondError(w, err)
		return
	}

	conn, err := app.Upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	proj, srv, err := h.resolveProjectServer(r.Context(), id)
	if err != nil {
		_ = conn.WriteJSON(map[string]string{"error": util.GetErrorMessage(err)})
		return
	}
	if proj.ActivePort == 0 {
		_ = conn.WriteJSON(map[string]interface{}{"online": false, "error": "No active deployment"})
		return
	}

	sshClient, err := client.NewSSHClient(toSSHCreds(srv))
	if err != nil {
		_ = conn.WriteJSON(map[string]string{"error": "Failed to connect to VPS via SSH"})
		return
	}
	defer sshClient.Close()

	containerName := client.ContainerName(proj.ProjectName, proj.ActivePort)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			payload := collectStats(sshClient, containerName)
			if err := conn.WriteJSON(payload); err != nil {
				return
			}
		}
	}
}

// ContainerLogs streams the docker logs of a project's container.
func (h *handler) ContainerLogs(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		util.RespondError(w, err)
		return
	}

	conn, err := app.Upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	proj, srv, err := h.resolveProjectServer(r.Context(), id)
	if err != nil || proj.ActivePort == 0 {
		return
	}

	sshClient, err := client.NewSSHClient(toSSHCreds(srv))
	if err != nil {
		return
	}
	defer sshClient.Close()

	cancelChan := make(chan struct{})
	go func() {
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				close(cancelChan)
				return
			}
		}
	}()
	go func() {
		<-cancelChan
		sshClient.Close()
	}()

	containerName := client.ContainerName(proj.ProjectName, proj.ActivePort)
	_ = client.RunCommandStream(sshClient, "docker logs -f --tail 100 "+containerName, func(line string) {
		_ = conn.WriteMessage(websocket.TextMessage, []byte(line))
	})
}

func (h *handler) resolveProjectServer(ctx context.Context, id int64) (*model.Project, *model.Server, error) {
	proj, err := h.service.FindOne(ctx, id)
	if err != nil {
		return nil, nil, err
	}

	srv, err := h.factory.Server.FindOne(ctx, proj.ServerID)
	if err != nil {
		return nil, nil, util.NewInternalServerError("Failed to load project server details", err)
	}

	return proj, srv, nil
}

// collectStats probes whether the container is running and, if so, returns its
// current docker stats snapshot.
func collectStats(sshClient *cryptossh.Client, containerName string) map[string]interface{} {
	var running bool
	checkCmd := fmt.Sprintf("docker ps --filter 'name=%s' --filter 'status=running' --format '{{.Names}}'", containerName)
	_ = client.RunCommandStream(sshClient, checkCmd, func(line string) {
		if strings.TrimSpace(line) == containerName {
			running = true
		}
	})

	if !running {
		return map[string]interface{}{"online": false}
	}

	var statsLine string
	statsCmd := fmt.Sprintf("docker stats --no-stream --format '{{.CPUPerc}};{{.MemUsage}};{{.MemPerc}}' %s", containerName)
	_ = client.RunCommandStream(sshClient, statsCmd, func(line string) {
		statsLine = strings.TrimSpace(line)
	})

	parts := strings.Split(statsLine, ";")
	if len(parts) < 3 {
		return map[string]interface{}{"online": false}
	}

	return map[string]interface{}{
		"online":    true,
		"cpu_perc":  strings.TrimSuffix(parts[0], "%"),
		"mem_usage": parts[1],
		"mem_perc":  strings.TrimSuffix(parts[2], "%"),
	}
}

func toResponses(projects []model.Project) []dto.ResponseProject {
	out := make([]dto.ResponseProject, 0, len(projects))
	for _, p := range projects {
		out = append(out, dto.ResponseProject{
			ID:             p.ID,
			ServerID:       p.ServerID,
			GitHubRepo:     p.GitHubRepo,
			GitHubBranch:   p.GitHubBranch,
			ProjectName:    p.ProjectName,
			DeploymentPath: p.DeploymentPath,
			StackType:      p.StackType,
			StackVersion:   p.StackVersion,
			AppPort:        p.AppPort,
			Domain:         p.Domain,
			ActivePort:     p.ActivePort,
			ActiveImage:    p.ActiveImage,
			ServerIP:       p.ServerIP,
		})
	}
	return out
}

func parseID(r *http.Request) (int64, error) {
	id, err := util.ParseInt64(util.URLParam(r, "id"))
	if err != nil {
		return 0, util.NewBadRequestError("Invalid project ID", err)
	}
	return id, nil
}
