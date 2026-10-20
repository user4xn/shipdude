package client

import (
	"fmt"
	"strings"

	"golang.org/x/crypto/ssh"
)

// VPSDiagnostics holds the results of the target VPS verification probe.
type VPSDiagnostics struct {
	Connected      bool   `json:"connected"`
	OS             string `json:"os"`
	DockerVersion  string `json:"docker_version"`
	ComposeVersion string `json:"compose_version"`
	HasSudo        bool   `json:"has_sudo"`
	InDockerGroup  bool   `json:"in_docker_group"`
	Bootstrapped   bool   `json:"bootstrapped"`
	Error          string `json:"error,omitempty"`
}

// RunVPSDiagnostics connects via SSH and inspects the VPS capabilities. A
// failed connection is reported inside the diagnostics rather than as a Go
// error so the portal can render partial results.
func RunVPSDiagnostics(creds SSHCredentials) (*VPSDiagnostics, error) {
	client, err := NewSSHClient(creds)
	if err != nil {
		return &VPSDiagnostics{Connected: false, Error: err.Error()}, nil
	}
	defer client.Close()

	diag := &VPSDiagnostics{Connected: true}

	if osOut, err := runCommand(client, "cat /etc/os-release"); err == nil {
		diag.OS = parseOSRelease(osOut)
	} else {
		diag.OS = "Unknown OS"
	}

	if dockerOut, err := runCommand(client, "docker --version"); err == nil {
		diag.DockerVersion = strings.TrimSpace(dockerOut)
	} else {
		diag.DockerVersion = "Not Installed"
	}

	if composeOut, err := runCommand(client, "docker compose version"); err == nil {
		diag.ComposeVersion = strings.TrimSpace(composeOut)
	} else {
		diag.ComposeVersion = "Not Installed"
	}

	if groupsOut, err := runCommand(client, "groups"); err == nil {
		diag.InDockerGroup = strings.Contains(groupsOut, "docker")
	}

	sudoOut, sudoErr := runCommand(client, "sudo -n true")
	diag.HasSudo = sudoErr == nil && !strings.Contains(sudoOut, "sudo: a password is required")

	deployerOut, err := runCommand(client, "id -u deployer")
	hasDeployer := err == nil && strings.TrimSpace(deployerOut) != ""

	caddyOut, err := runCommand(client, "docker ps -a --filter 'name=shipdude-caddy' --format '{{.Names}}'")
	hasCaddy := err == nil && strings.Contains(caddyOut, "shipdude-caddy")

	diag.Bootstrapped = hasDeployer && hasCaddy

	return diag, nil
}

func runCommand(client *ssh.Client, cmd string) (string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()

	var stdout, stderr strings.Builder
	session.Stdout = &stdout
	session.Stderr = &stderr

	err = session.Run(cmd)
	if err != nil {
		return stdout.String() + stderr.String(), err
	}

	return stdout.String(), nil
}

func parseOSRelease(output string) string {
	lines := strings.Split(output, "\n")
	var name, version string

	for _, line := range lines {
		if strings.HasPrefix(line, "PRETTY_NAME=") {
			return strings.Trim(line[12:], `"`)
		}
		if strings.HasPrefix(line, "NAME=") && name == "" {
			name = strings.Trim(line[5:], `"`)
		}
		if strings.HasPrefix(line, "VERSION=") && version == "" {
			version = strings.Trim(line[8:], `"`)
		}
	}

	if name != "" {
		if version != "" {
			return fmt.Sprintf("%s %s", name, version)
		}
		return name
	}

	return "Generic Linux"
}

// AddAuthorizedKey appends a public SSH key to the target server's
// authorized_keys file.
func AddAuthorizedKey(creds SSHCredentials, pubKey string) error {
	client, err := NewSSHClient(creds)
	if err != nil {
		return fmt.Errorf("failed to connect via SSH: %w", err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create SSH session: %w", err)
	}
	defer session.Close()

	cleanPubKey := strings.TrimSpace(pubKey)
	cmd := fmt.Sprintf("mkdir -p ~/.ssh && chmod 700 ~/.ssh && echo '%s' >> ~/.ssh/authorized_keys && chmod 600 ~/.ssh/authorized_keys", cleanPubKey)

	var stderr strings.Builder
	session.Stderr = &stderr
	if err := session.Run(cmd); err != nil {
		return fmt.Errorf("failed to execute append command: %v (stderr: %s)", err, stderr.String())
	}

	return nil
}
