package model

// Project represents a deployed application container configuration.
type Project struct {
	ID             int64  `gorm:"column:id;primaryKey;autoIncrement" column:"id" json:"id"`
	ServerID       int64  `gorm:"column:server_id;not null" column:"server_id" json:"server_id"`
	GitHubRepo     string `gorm:"column:github_repo;not null" column:"github_repo" json:"github_repo"`
	GitHubBranch   string `gorm:"column:github_branch;default:main" column:"github_branch" json:"github_branch"`
	ProjectName    string `gorm:"column:project_name;not null" column:"project_name" json:"project_name"`
	DeploymentPath string `gorm:"column:deployment_path;not null" column:"deployment_path" json:"deployment_path"`
	StackType      string `gorm:"column:stack_type;not null" column:"stack_type" json:"stack_type"`
	StackVersion   string `gorm:"column:stack_version;not null" column:"stack_version" json:"stack_version"`
	AppPort        int    `gorm:"column:app_port;not null" column:"app_port" json:"app_port"`
	Domain         string `gorm:"column:domain;default:''" column:"domain" json:"domain"`
	ActivePort     int    `gorm:"column:active_port;default:0" column:"active_port" json:"active_port"`
	ActiveImage    string `gorm:"column:active_image;default:''" column:"active_image" json:"active_image"`

	// ServerIP is a joined, read-only field populated by repository queries.
	ServerIP string `gorm:"column:server_ip;->" column:"server_ip" json:"server_ip"`
}

// TableName returns the physical table for projects.
func (Project) TableName() string {
	return "projects"
}
