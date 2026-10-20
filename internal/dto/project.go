package dto

// PayloadProjectDeploy is the full one-click deployment setup request.
type PayloadProjectDeploy struct {
	IPAddress      string `json:"ip_address" binding:"required"`
	SSHPort        int    `json:"ssh_port"`
	SSHUser        string `json:"ssh_user"`
	AuthMethod     string `json:"auth_method"`
	Password       string `json:"password"`
	PrivateKey     string `json:"private_key"`
	GitHubRepo     string `json:"github_repo" binding:"required"`
	GitHubBranch   string `json:"github_branch"`
	ProjectName    string `json:"project_name" binding:"required"`
	DeploymentPath string `json:"deployment_path"`
	StackType      string `json:"stack_type"`
	StackVersion   string `json:"stack_version"`
	AppPort        int    `json:"app_port"`
	GitHubToken    string `json:"github_token"`
	Domain         string `json:"domain"`
}

// PayloadProjectTrigger is sent by the GitHub Actions runner after a build.
type PayloadProjectTrigger struct {
	SHA string `json:"sha"`
}

// PayloadProjectRollback redeploys a previously pushed image.
type PayloadProjectRollback struct {
	TargetImage string `json:"target_image" binding:"required"`
}

// PayloadProjectControl issues a container lifecycle command.
type PayloadProjectControl struct {
	Action string `json:"action" binding:"required"`
}

// ResponseProject is the project view rendered by the dashboard. The JSON
// keys intentionally match the model so the embedded UI keeps working.
type ResponseProject struct {
	ID             int64  `json:"id"`
	ServerID       int64  `json:"server_id"`
	GitHubRepo     string `json:"github_repo"`
	GitHubBranch   string `json:"github_branch"`
	ProjectName    string `json:"project_name"`
	DeploymentPath string `json:"deployment_path"`
	StackType      string `json:"stack_type"`
	StackVersion   string `json:"stack_version"`
	AppPort        int    `json:"app_port"`
	Domain         string `json:"domain"`
	ActivePort     int    `json:"active_port"`
	ActiveImage    string `json:"active_image"`
	ServerIP       string `json:"server_ip"`
}

// ResponseProjectControl reports the outcome of a container command.
type ResponseProjectControl struct {
	Status string `json:"status"`
	Action string `json:"action"`
	Output string `json:"output"`
}

// ResponseProjectDelete confirms a configuration removal.
type ResponseProjectDelete struct {
	Status string `json:"status"`
}
