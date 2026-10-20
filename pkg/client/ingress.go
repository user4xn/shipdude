package client

import (
	"fmt"
	"strings"
)

// IngressRoute describes a single domain -> upstream mapping for the
// generated reverse proxy configuration. When Domain is empty the route falls
// back to "<ProjectName>.<ServerIP>.sslip.io".
type IngressRoute struct {
	Domain      string
	ProjectName string
	Port        int
}

// IngressConfig holds everything needed to render a reverse proxy file.
type IngressConfig struct {
	ServerIP string
	Routes   []IngressRoute
}

// RenderCaddyfile produces the Caddy reverse-proxy configuration for the
// given ingress routes. Routes without a port are skipped.
func RenderCaddyfile(cfg IngressConfig) string {
	var sb strings.Builder
	sb.WriteString("# ShipDude Ingress Gateway Config\n\n")

	for _, route := range cfg.Routes {
		if route.Port <= 0 {
			continue
		}

		domain := route.Domain
		if domain == "" {
			domain = fmt.Sprintf("%s.%s.sslip.io", CleanProjectName(route.ProjectName), cfg.ServerIP)
		}

		sb.WriteString(fmt.Sprintf("%s {\n", domain))
		sb.WriteString(fmt.Sprintf("    reverse_proxy localhost:%d\n", route.Port))
		sb.WriteString("}\n\n")
	}

	return sb.String()
}

// CleanProjectName formats a project name so it is safe to use as a
// docker container name.
func CleanProjectName(name string) string {
	var sb strings.Builder
	for _, r := range strings.ToLower(name) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-':
			sb.WriteRune(r)
		case r == ' ' || r == '_':
			sb.WriteRune('-')
		}
	}

	res := sb.String()
	if res == "" {
		res = "app"
	}
	return res
}

// ContainerName builds the deterministic docker container name for a project
// on a given port, e.g. shipdude-my-website-3000.
func ContainerName(projectName string, port int) string {
	return fmt.Sprintf("shipdude-%s-%d", CleanProjectName(projectName), port)
}
