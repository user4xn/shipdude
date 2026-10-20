package dto

// PayloadVPSConnection mirrors the SSH connection details persisted for the
// next visit. Binding is done manually because the embedded UI omits fields
// that are irrelevant to the selected auth method.
type PayloadVPSConnection struct {
	IPAddress  string `json:"ip_address"`
	SSHPort    int    `json:"ssh_port"`
	SSHUser    string `json:"ssh_user"`
	AuthMethod string `json:"auth_method"`
	Password   string `json:"password"`
	PrivateKey string `json:"private_key"`
}

// PayloadVPSProvision requests a background bootstrap of a VPS node.
type PayloadVPSProvision struct {
	ServerID      int64  `json:"server_id"`
	IPAddress     string `json:"ip_address"`
	SSHPort       int    `json:"ssh_port"`
	SSHUser       string `json:"ssh_user"`
	AuthMethod    string `json:"auth_method"`
	Password      string `json:"password"`
	PrivateKey    string `json:"private_key"`
	InstallDocker bool   `json:"install_docker"`
}

// ResponseVPSConnection returns the saved SSH connection details.
type ResponseVPSConnection struct {
	IPAddress  string `json:"ip_address"`
	SSHPort    int    `json:"ssh_port"`
	SSHUser    string `json:"ssh_user"`
	AuthMethod string `json:"auth_method"`
	Password   string `json:"password"`
	PrivateKey string `json:"private_key"`
}

// ResponseJobAck acknowledges that a background job has been accepted.
type ResponseJobAck struct {
	Status    string `json:"status"`
	JobID     string `json:"job_id"`
	ProjectID int64  `json:"project_id,omitempty"`
}
