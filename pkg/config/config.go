package config

import "shipdude/pkg/util"

// AppName is the public brand of the service.
func AppName() string {
	return "ShipDude"
}

// AppEnv returns the current runtime environment (e.g. "development", "production").
func AppEnv() string {
	return util.GetEnv("APP_ENV", "development")
}

// AppPort returns the HTTP listen port of the portal.
func AppPort() string {
	return util.GetEnv("APP_PORT", "8080")
}

// DBPath returns the filesystem location of the embedded SQLite database.
func DBPath() string {
	return util.GetEnv("DB_PATH", "./config/shipdude.db")
}

// GitHubClientID returns the fallback OAuth client ID used before one is
// persisted through the settings page.
func GitHubClientID() string {
	return util.GetEnv("GITHUB_CLIENT_ID", "")
}

// GitHubClientSecret returns the fallback OAuth client secret.
func GitHubClientSecret() string {
	return util.GetEnv("GITHUB_CLIENT_SECRET", "")
}

// GitHubOAuthToken returns the fallback personal access token used when no
// OAuth session has been stored yet.
func GitHubOAuthToken() string {
	return util.GetEnv("GITHUB_OAUTH_TOKEN", "")
}

// DefaultSSHPort returns the SSH port used when a caller leaves it empty.
func DefaultSSHPort() int {
	return util.GetEnvInt("DEFAULT_SSH_PORT", 22)
}

// DefaultSSHUser returns the SSH user used when a caller leaves it empty.
func DefaultSSHUser() string {
	return util.GetEnv("DEFAULT_SSH_USER", "root")
}
