package model

// LastConnection is the SSH connection details remembered between sessions.
// It is not a table; it is assembled from encrypted setting values.
type LastConnection struct {
	IPAddress  string
	SSHPort    int
	SSHUser    string
	AuthMethod string
	Password   string
	PrivateKey string
}
