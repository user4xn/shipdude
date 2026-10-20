package client

import "fmt"

// WorkflowSpec holds the parameters injected into the generated GitHub
// Actions workflow.
type WorkflowSpec struct {
	Branch      string
	Host        string
	Port        int
	User        string
	ProjectName string
	ServerURL   string
}

// RenderWorkflowYAML produces the ShipDude CD workflow committed into a
// project's repository.
func RenderWorkflowYAML(spec WorkflowSpec) string {
	tmpl := `name: ShipDude CD
on:
  push:
    branches:
      - "%s"

permissions:
  contents: read
  packages: write

jobs:
  build-and-deploy:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout Code
        uses: actions/checkout@v4

      - name: Set up QEMU
        uses: docker/setup-qemu-action@v3

      - name: Set up Docker Buildx
        uses: docker/setup-buildx-action@v3

      - name: Log in to GitHub Container Registry
        uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - name: Build and Push Docker Image
        uses: docker/build-push-action@v5
        with:
          context: .
          push: true
          tags: ghcr.io/${{ github.repository }}:${{ github.sha }},ghcr.io/${{ github.repository }}:latest

      - name: Deploy to VPS via SSH
        uses: appleboy/ssh-action@v1.0.3
        with:
          host: "%s"
          port: %d
          username: "%s"
          key: "${{ secrets.SHIPDUDE_SSH_PRIVATE_KEY }}"
          script: |
            # Trigger ShipDude agent deployment (fails silently with || true if ShipDude is offline)
            if command -v curl >/dev/null 2>&1; then
              curl -s -X POST -d '{"sha":"${{ github.sha }}"}' -H "Content-Type: application/json" "%s/api/v1/projects/deploy-trigger?repo=${{ github.repository }}" || true
            elif command -v wget >/dev/null 2>&1; then
              wget -q --post-data='{"sha":"${{ github.sha }}"}' --header='Content-Type: application/json' -O- "%s/api/v1/projects/deploy-trigger?repo=${{ github.repository }}" || true
            else
              echo "Error: Neither curl nor wget found on the target VPS. Cannot trigger deployment."
              exit 1
            fi
`
	return fmt.Sprintf(tmpl, spec.Branch, spec.Host, spec.Port, spec.User, spec.ServerURL, spec.ServerURL)
}
