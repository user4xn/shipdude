package project

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"shipdude/internal/model"
	"shipdude/pkg/client"
	"shipdude/pkg/consts"
	"shipdude/pkg/socket"
)

// performHotSwap executes the zero-downtime deployment sequence on the host.
func (s *service) performHotSwap(proj *model.Project, srv *model.Server, image string, job *socket.Job) error {
	job.AddLog(fmt.Sprintf("[System] Establishing SSH connection to target VPS (%s)...", srv.IPAddress))
	sshClient, err := client.NewSSHClient(toSSHCreds(srv))
	if err != nil {
		return fmt.Errorf("failed to connect to VPS: %w", err)
	}
	defer sshClient.Close()

	portA, portB := proj.AppPort, proj.AppPort+1
	if portA <= 0 {
		portA, portB = 3000, 3001
	}

	targetPort := portA
	if proj.ActivePort == portA {
		targetPort = portB
	}

	s.authenticateRegistry(sshClient, proj, job)

	job.AddLog(fmt.Sprintf("[Docker] Pulling image: %s...", image))
	if err := client.RunCommandStream(sshClient, "docker pull "+image, func(line string) {
		job.AddLog("[Docker] " + line)
	}); err != nil {
		return fmt.Errorf("docker pull failed: %w", err)
	}

	newContainer := client.ContainerName(proj.ProjectName, targetPort)
	oldContainer := ""
	if proj.ActivePort > 0 {
		oldContainer = client.ContainerName(proj.ProjectName, proj.ActivePort)
	}

	job.AddLog(fmt.Sprintf("[Docker] Removing pre-existing container %s if present...", newContainer))
	_ = client.RunCommandStream(sshClient, "docker rm -f "+newContainer, func(string) {})

	job.AddLog(fmt.Sprintf("[Docker] Launching new container %s on port %d...", newContainer, targetPort))
	runCmd := fmt.Sprintf("docker run -d --name %s --restart unless-stopped -p %d:%d -e PORT=%d %s",
		newContainer, targetPort, proj.AppPort, proj.AppPort, image)
	if err := client.RunCommandStream(sshClient, runCmd, func(line string) {
		job.AddLog("[Docker] " + line)
	}); err != nil {
		return fmt.Errorf("docker run failed: %w", err)
	}

	if !s.waitHealthy(sshClient, targetPort, job) {
		_ = client.RunCommandStream(sshClient, "docker rm -f "+newContainer, func(string) {})
		return errors.New("container health check failed")
	}

	job.AddLog("[Caddy] Regenerating Caddyfile configurations...")
	if err := s.regenerateCaddyfile(sshClient, srv, proj, targetPort); err != nil {
		return fmt.Errorf("caddyfile regeneration failed: %w", err)
	}

	job.AddLog("[Caddy] Reloading Caddy Ingress Gateway...")
	if err := client.RunCommandStream(sshClient, "docker exec shipdude-caddy caddy reload --config /etc/caddy/Caddyfile", func(line string) {
		job.AddLog("[Caddy] " + line)
	}); err != nil {
		job.AddLog(fmt.Sprintf("[Warning] Caddy reload returned error: %s. Continuing...", err.Error()))
	}

	if oldContainer != "" && oldContainer != newContainer {
		job.AddLog(fmt.Sprintf("[Docker] Pruning old container %s...", oldContainer))
		_ = client.RunCommandStream(sshClient, "docker rm -f "+oldContainer, func(string) {})
	}

	if err := s.project.UpdateActiveState(s.db(), proj.ID, targetPort, image); err != nil {
		job.AddLog("[Warning] Failed to update project state in database: " + err.Error())
	}

	job.AddLog("[System] Zero-Downtime Hot-Swap complete!")
	return nil
}

func (s *service) authenticateRegistry(sshClient *ssh.Client, proj *model.Project, job *socket.Job) {
	owner := strings.Split(proj.GitHubRepo, "/")[0]
	token := s.github.OAuthToken(context.Background())
	if token == "" {
		return
	}

	job.AddLog("[Docker] Authenticating with GitHub Container Registry (ghcr.io)...")
	loginCmd := fmt.Sprintf("echo %q | docker login ghcr.io -u %q --password-stdin", token, owner)
	if err := client.RunCommandStream(sshClient, loginCmd, func(line string) {
		if !strings.Contains(line, token) {
			job.AddLog("[Docker] " + line)
		}
	}); err != nil {
		job.AddLog("[Warning] GHCR authentication failed: " + err.Error() + ". Attempting pull anyway...")
	}
}

func (s *service) waitHealthy(sshClient *ssh.Client, port int, job *socket.Job) bool {
	job.AddLog("[System] Waiting for application to accept HTTP connections...")

	for i := 0; i < 30; i++ {
		time.Sleep(2 * time.Second)

		var httpCode string
		_ = client.RunCommandStream(sshClient, fmt.Sprintf("curl -s -o /dev/null -w \"%%{http_code}\" http://localhost:%d/", port), func(line string) {
			httpCode = strings.TrimSpace(line)
		})

		if httpCode != "" && httpCode != "000" {
			job.AddLog(fmt.Sprintf("[System] Application responded with status: %s. Health check passed!", httpCode))
			return true
		}
		job.AddLog(fmt.Sprintf("[System] Health check attempt %d/30: waiting for port %d to listen...", i+1, port))
	}

	job.AddLog("[Error] Health check timed out after 60s. Rolling back container launch...")
	return false
}

// regenerateCaddyfile rewrites the ingress configuration mapping domains to live containers.
func (s *service) regenerateCaddyfile(sshClient *ssh.Client, srv *model.Server, target *model.Project, targetPort int) error {
	projects, err := s.project.FindAll(context.Background())
	if err != nil {
		return err
	}

	cfg := client.IngressConfig{ServerIP: srv.IPAddress}
	for _, p := range projects {
		if p.ServerID != srv.ID {
			continue
		}

		port := p.ActivePort
		if p.ID == target.ID {
			port = targetPort
		}

		cfg.Routes = append(cfg.Routes, client.IngressRoute{
			Domain:      p.Domain,
			ProjectName: p.ProjectName,
			Port:        port,
		})
	}

	encoded := base64.StdEncoding.EncodeToString([]byte(client.RenderCaddyfile(cfg)))
	writeCmd := fmt.Sprintf("echo '%s' | base64 -d | sudo tee /var/lib/shipdude/caddy/Caddyfile >/dev/null", encoded)
	if err := client.RunCommandStream(sshClient, writeCmd, func(string) {}); err != nil {
		return err
	}

	_ = client.RunCommandStream(sshClient, "docker cp /var/lib/shipdude/caddy/Caddyfile shipdude-caddy:/etc/caddy/Caddyfile", func(string) {})
	return nil
}

func toSSHCreds(srv *model.Server) client.SSHCredentials {
	return client.SSHCredentials{
		IPAddress:  srv.IPAddress,
		Port:       srv.SSHPort,
		User:       srv.SSHUser,
		AuthMethod: consts.AuthKey,
		PrivateKey: srv.SSHPrivateKey,
	}
}

func (s *service) db() *sql.DB {
	return s.factory.InitDB().SQL()
}
