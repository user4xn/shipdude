package client

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/ssh"
)

// BootstrapVPS installs Docker, Docker Compose, the deployer user and the
// Caddy ingress gateway on the target server, streaming progress to logFunc.
func BootstrapVPS(client *ssh.Client, installDocker bool, pubKey string, logFunc func(string)) error {
	logFunc("[System] Initiating VPS bootstrapping sequence...")

	runCmd := func(cmd string) error {
		logFunc(fmt.Sprintf("[System] Executing: %s", cmd))
		return RunCommandStream(client, cmd, func(line string) {
			logFunc(line)
		})
	}

	logFunc("[Provision] Checking for Docker installation...")
	var hasDocker bool
	err := RunCommandStream(client, "docker --version", func(line string) {
		logFunc(line)
		if strings.Contains(line, "version") {
			hasDocker = true
		}
	})

	if err != nil || !hasDocker {
		if !installDocker {
			return errors.New("docker is not installed on the target VPS, and install_docker was false")
		}

		logFunc("[Provision] Docker not found. Fetching and installing Docker...")
		if err := runCmd("curl -fsSL https://get.docker.com -o get-docker.sh"); err != nil {
			return fmt.Errorf("failed to download Docker installation script: %w", err)
		}
		if err := runCmd("sudo sh get-docker.sh || sh get-docker.sh"); err != nil {
			return fmt.Errorf("failed to execute Docker installation: %w", err)
		}
		_ = runCmd("rm get-docker.sh")
	} else {
		logFunc("[Provision] Docker is already installed.")
	}

	_ = runCmd("sudo systemctl start docker || true")

	logFunc("[Provision] Checking for Docker Compose...")
	var hasCompose bool
	_ = RunCommandStream(client, "docker compose version", func(line string) {
		logFunc(line)
		if strings.Contains(line, "version") {
			hasCompose = true
		}
	})

	if !hasCompose {
		logFunc("[Provision] Docker Compose not found. Installing standalone binary...")
		if err := runCmd(`sudo curl -L "https://github.com/docker/compose/releases/download/v2.24.5/docker-compose-$(uname -s)-$(uname -m)" -o /usr/local/bin/docker-compose`); err != nil {
			return fmt.Errorf("failed to download Docker Compose: %w", err)
		}
		if err := runCmd("sudo chmod +x /usr/local/bin/docker-compose"); err != nil {
			return fmt.Errorf("failed to set permissions on Docker Compose: %w", err)
		}
		_ = runCmd("sudo ln -sf /usr/local/bin/docker-compose /usr/bin/docker-compose")
	} else {
		logFunc("[Provision] Docker Compose is already installed.")
	}

	logFunc("[Provision] Configuring deployer group and user permissions...")
	_ = runCmd("getent group docker || sudo groupadd docker || sudo addgroup docker || true")

	var userExists bool
	_ = RunCommandStream(client, "id -u deployer", func(line string) {
		if _, err := strconv.Atoi(strings.TrimSpace(line)); err == nil {
			userExists = true
		}
	})

	if !userExists {
		if err := runCmd(`sudo useradd -m -s /bin/bash -g docker deployer || useradd -m -s /bin/bash -g docker deployer || sudo adduser -D -g "" -s /bin/sh -G docker deployer || adduser -D -g "" -s /bin/sh -G docker deployer`); err != nil {
			return fmt.Errorf("failed to create deployer user: %w", err)
		}
	}

	_ = runCmd("sudo usermod -aG docker deployer || usermod -aG docker deployer || sudo addgroup deployer docker || addgroup deployer docker")

	logFunc("[Provision] Installing SSH deployment authorization keys...")
	setupSSHCmds := fmt.Sprintf(`sudo mkdir -p /home/deployer/.ssh &&
sudo chmod 700 /home/deployer/.ssh &&
echo '%s' | sudo tee /home/deployer/.ssh/authorized_keys >/dev/null &&
sudo chmod 600 /home/deployer/.ssh/authorized_keys &&
sudo chown -R deployer:docker /home/deployer/.ssh`, strings.TrimSpace(pubKey))

	if err := runCmd(setupSSHCmds); err != nil {
		return fmt.Errorf("failed to configure deployer SSH keys: %w", err)
	}

	logFunc("[Provision] Bootstrapping Caddy Ingress Gateway...")
	caddySetupCmds := `sudo mkdir -p /var/lib/shipdude/caddy &&
sudo chown -R deployer:docker /var/lib/shipdude/caddy &&
[ -f /var/lib/shipdude/caddy/Caddyfile ] || echo "# ShipDude Ingress Gateway" | sudo tee /var/lib/shipdude/caddy/Caddyfile >/dev/null`

	if err := runCmd(caddySetupCmds); err != nil {
		return fmt.Errorf("failed to initialize Caddyfile directories: %w", err)
	}

	var caddyRunning bool
	_ = RunCommandStream(client, "docker ps --filter 'name=shipdude-caddy' --format '{{.Names}}'", func(line string) {
		if strings.TrimSpace(line) == "shipdude-caddy" {
			caddyRunning = true
		}
	})

	if !caddyRunning {
		logFunc("[Provision] Launching Caddy ingress container...")
		_ = runCmd("docker rm -f shipdude-caddy || true")
		caddyLaunchCmd := "docker run -d --name shipdude-caddy --restart unless-stopped --network host -v shipdude-caddy-config:/etc/caddy caddy:2-alpine"
		if err := runCmd(caddyLaunchCmd); err != nil {
			return fmt.Errorf("failed to start Caddy ingress container: %w", err)
		}
		_ = runCmd("docker cp /var/lib/shipdude/caddy/Caddyfile shipdude-caddy:/etc/caddy/Caddyfile || true")
	} else {
		logFunc("[Provision] Caddy ingress container is already active.")
	}

	logFunc("[System] VPS bootstrapping sequence finished successfully!")
	return nil
}
