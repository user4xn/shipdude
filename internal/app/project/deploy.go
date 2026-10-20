package project

import (
	"context"
	"database/sql"
	"encoding/base64"
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
	if err := client.RunCommandStream(sshClient, "docker pull "+shellQuote(image), func(line string) {
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
	_ = client.RunCommandStream(sshClient, "docker rm -f "+shellQuote(newContainer), func(string) {})

	job.AddLog(fmt.Sprintf("[Docker] Launching new container %s on port %d...", newContainer, targetPort))
	runCmd := fmt.Sprintf("docker run -d --name %s --restart unless-stopped -p %d:%d -e PORT=%d %s",
		shellQuote(newContainer), targetPort, proj.AppPort, proj.AppPort, shellQuote(image))
	if err := client.RunCommandStream(sshClient, runCmd, func(line string) {
		job.AddLog("[Docker] " + line)
	}); err != nil {
		return fmt.Errorf("docker run failed: %w", err)
	}

	if err := s.waitHealthy(sshClient, targetPort, job); err != nil {
		_ = client.RunCommandStream(sshClient, "docker rm -f "+shellQuote(newContainer), func(string) {})
		return err
	}

	job.AddLog("[Caddy] Regenerating Caddyfile configurations...")
	previousCaddyfile, err := s.regenerateCaddyfile(sshClient, srv, proj, targetPort)
	if err != nil {
		return fmt.Errorf("caddyfile regeneration failed: %w", err)
	}

	job.AddLog("[Caddy] Reloading Caddy Ingress Gateway...")
	if err := client.RunCommandStream(sshClient, "docker exec shipdude-caddy caddy reload --config /etc/caddy/Caddyfile", func(line string) {
		job.AddLog("[Caddy] " + line)
	}); err != nil {
		// Container lama SENGAJA dipertahankan: kalau reload gagal, Caddy masih
		// memakai konfigurasi lama yang menunjuk port lama. Menghapus container
		// lama di sini akan membuat situs mati total sementara job melaporkan sukses.
		//
		// Dua hal lain harus dibereskan: Caddyfile di disk sudah menunjuk port baru,
		// jadi ia dipulihkan ke isi sebelumnya (kalau tidak, reload berikutnya dari
		// project lain akan mempromosikan container yang belum lolos), dan container
		// baru dibuang karena belum pernah menerima trafik.
		var restoreErr error
		var restoreOutput strings.Builder
		if previousCaddyfile != "" {
			restoreCmd := fmt.Sprintf("echo %s | base64 -d | sudo tee %s >/dev/null", shellQuote(previousCaddyfile), caddyfilePath)
			restoreErr = client.RunCommandStream(sshClient, restoreCmd, func(line string) {
				// stderr ikut mengalir ke sini; simpan sebagai alasan kegagalan.
				restoreOutput.WriteString(strings.TrimSpace(line) + " ")
			})
			if restoreErr == nil {
				_ = client.RunCommandStream(sshClient, "docker cp "+caddyfilePath+" shipdude-caddy:"+caddyInContainer, func(string) {})
			}
		}
		_ = client.RunCommandStream(sshClient, "docker rm -f "+shellQuote(newContainer), func(string) {})
		if restoreErr != nil {
			// Jangan mengaku sudah dipulihkan kalau penulisan cadangan gagal:
			// Caddyfile di disk masih menunjuk container yang barusan dibuang.
			reason := strings.TrimSpace(restoreOutput.String())
			if reason == "" {
				reason = restoreErr.Error()
			}
			return fmt.Errorf("caddy reload failed dan pemulihan Caddyfile ikut GAGAL (%s); Caddyfile di disk masih menunjuk container %s yang sudah dibuang, perlu dipulihkan manual: %w", reason, newContainer, err)
		}
		return fmt.Errorf("caddy reload failed; Caddyfile dipulihkan, container baru dibuang, container lama tetap melayani: %w", err)
	}

	if oldContainer != "" && oldContainer != newContainer {
		job.AddLog(fmt.Sprintf("[Docker] Pruning old container %s...", oldContainer))
		_ = client.RunCommandStream(sshClient, "docker rm -f "+shellQuote(oldContainer), func(string) {})
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
	loginCmd := fmt.Sprintf("echo %s | docker login ghcr.io -u %s --password-stdin", shellQuote(token), shellQuote(owner))
	if err := client.RunCommandStream(sshClient, loginCmd, func(line string) {
		if !strings.Contains(line, token) {
			job.AddLog("[Docker] " + line)
		}
	}); err != nil {
		job.AddLog("[Warning] GHCR authentication failed: " + err.Error() + ". Attempting pull anyway...")
	}
}

// healthCheckInterval bisa diperpendek oleh test supaya tidak menunggu 2 detik
// per percobaan.
var healthCheckInterval = 2 * time.Second

// healthCheckAttempts adalah jumlah percobaan sebelum deploy dinyatakan gagal.
var healthCheckAttempts = 30

// isHealthyStatus menganggap sehat hanya status HTTP 2xx dan 3xx. Nilai kosong
// atau "000" (curl gagal konek) berarti belum siap; status 4xx/5xx berarti
// aplikasi hidup tapi rusak, dan itu tidak boleh dipromosikan ke produksi.
func isHealthyStatus(raw string) bool {
	code := strings.TrimSpace(raw)
	if len(code) > 3 {
		code = code[len(code)-3:]
	}
	if len(code) != 3 {
		return false
	}
	for i := 0; i < 3; i++ {
		if code[i] < '0' || code[i] > '9' {
			return false
		}
	}
	return code[0] == '2' || code[0] == '3'
}

func (s *service) waitHealthy(sshClient *ssh.Client, port int, job *socket.Job) error {
	job.AddLog("[System] Waiting for application to accept HTTP connections...")

	lastCode := ""
	for i := 0; i < healthCheckAttempts; i++ {
		time.Sleep(healthCheckInterval)

		var httpCode string
		_ = client.RunCommandStream(sshClient, fmt.Sprintf("curl -s -o /dev/null -w \"%%{http_code}\" http://127.0.0.1:%d/", port), func(line string) {
			httpCode = strings.TrimSpace(line)
		})
		lastCode = httpCode

		if isHealthyStatus(httpCode) {
			job.AddLog(fmt.Sprintf("[System] Application responded with status: %s. Health check passed!", httpCode))
			return nil
		}
		job.AddLog(fmt.Sprintf("[System] Health check attempt %d/%d: status %q is not healthy yet...", i+1, healthCheckAttempts, httpCode))
	}

	job.AddLog("[Error] Health check gagal; container baru dibuang, container lama tetap melayani.")
	return fmt.Errorf("container health check failed after %d attempts (last status: %q)", healthCheckAttempts, lastCode)
}

// regenerateCaddyfile rewrites the ingress configuration mapping domains to live containers.
func (s *service) regenerateCaddyfile(sshClient *ssh.Client, srv *model.Server, target *model.Project, targetPort int) (string, error) {
	projects, err := s.project.FindAll(context.Background())
	if err != nil {
		return "", err
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

	// Simpan isi lama (base64) supaya bisa dipulihkan kalau reload gagal.
	// `sudo base64 -w0 FILE` membaca file secara langsung, tanpa pipeline, jadi
	// exit code pembacaan tidak tertelan perintah di ujung pipe.
	previous, captureErr := captureCaddyfile(sshClient)
	if captureErr != nil {
		// File ADA tapi tidak terbaca: tanpa cadangan, kegagalan reload akan
		// meninggalkan konfigurasi menunjuk container yang sudah dibuang.
		return "", fmt.Errorf("failed to capture existing Caddyfile; refusing to overwrite without a backup: %w", captureErr)
	}

	encoded := base64.StdEncoding.EncodeToString([]byte(client.RenderCaddyfile(cfg)))
	writeCmd := fmt.Sprintf("echo '%s' | base64 -d | sudo tee %s >/dev/null", encoded, caddyfilePath)
	if err := client.RunCommandStream(sshClient, writeCmd, func(string) {}); err != nil {
		return previous, err
	}

	_ = client.RunCommandStream(sshClient, "docker cp "+caddyfilePath+" shipdude-caddy:"+caddyInContainer, func(string) {})
	return previous, nil
}

const (
	// caddyfilePath adalah sumber kebenaran konfigurasi ingress di VPS.
	caddyfilePath = "/var/lib/shipdude/caddy/Caddyfile"
	// caddyInContainer adalah path yang dibaca Caddy di dalam container.
	caddyInContainer = "/etc/caddy/Caddyfile"
)

// captureCaddyfile mengembalikan isi Caddyfile aktif dalam base64.
//
// File yang BELUM ADA bukan kegagalan: tidak ada isi lama yang bisa hilang,
// jadi deploy pertama di VPS baru tetap boleh menulis konfigurasi. Yang harus
// menggagalkan deploy adalah file yang ada tapi tidak bisa dibaca, karena
// reload yang gagal sesudahnya tidak akan bisa dipulihkan.
func captureCaddyfile(sshClient *ssh.Client) (string, error) {
	var raw string
	readErr := client.RunCommandStream(sshClient, fmt.Sprintf("sudo base64 -w0 %s 2>/dev/null", caddyfilePath), func(line string) {
		raw += strings.TrimSpace(line)
	})
	if readErr == nil {
		return raw, nil
	}

	// Bedakan "belum ada" (aman, lanjut tanpa cadangan) dari "ada tapi tidak
	// terbaca" (gagal tertutup).
	existsErr := client.RunCommandStream(sshClient, fmt.Sprintf("sudo test -f %s", caddyfilePath), func(string) {})
	if existsErr != nil {
		return "", nil
	}
	return "", readErr
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
