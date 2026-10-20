package client

import (
	"strings"
	"testing"
)

func TestCleanProjectName(t *testing.T) {
	cases := map[string]string{
		"My Website":  "my-website",
		"app_service": "app-service",
		"@@@":         "app",
		"UPPER-case":  "upper-case",
	}
	for input, want := range cases {
		if got := CleanProjectName(input); got != want {
			t.Errorf("CleanProjectName(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestContainerName(t *testing.T) {
	if got := ContainerName("My Website", 3000); got != "shipdude-my-website-3000" {
		t.Errorf("unexpected container name %q", got)
	}
}

func TestRenderCaddyfile(t *testing.T) {
	cfg := IngressConfig{
		ServerIP: "1.2.3.4",
		Routes: []IngressRoute{
			{Domain: "example.com", Port: 3000},
			{ProjectName: "my app", Port: 3001},
			{ProjectName: "skipped", Port: 0},
		},
	}

	out := RenderCaddyfile(cfg)

	for _, want := range []string{
		"# ShipDude Ingress Gateway Config",
		"example.com {",
		"reverse_proxy localhost:3000",
		"my-app.1.2.3.4.sslip.io {",
		"reverse_proxy localhost:3001",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected caddyfile to contain %q, got:\n%s", want, out)
		}
	}

	if strings.Contains(out, "skipped") {
		t.Errorf("routes without a port must be skipped, got:\n%s", out)
	}
}

func TestRenderWorkflowYAML(t *testing.T) {
	out := RenderWorkflowYAML(WorkflowSpec{
		Branch:    "main",
		Host:      "1.2.3.4",
		Port:      22,
		User:      "deployer",
		ServerURL: "https://ship.example.com",
	})

	for _, want := range []string{
		"name: ShipDude CD",
		`- "main"`,
		`host: "1.2.3.4"`,
		"port: 22",
		`username: "deployer"`,
		"https://ship.example.com/api/v1/projects/deploy-trigger",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected workflow to contain %q", want)
		}
	}
}
