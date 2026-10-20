package dto

// PayloadSettingsGitHub updates the stored GitHub OAuth application.
type PayloadSettingsGitHub struct {
	ClientID     string `json:"client_id" binding:"required"`
	ClientSecret string `json:"client_secret" binding:"required"`
}

// PayloadGitHubDetectStack asks ShipDude to analyze a repository.
type PayloadGitHubDetectStack struct {
	Repo  string `json:"repo" binding:"required"`
	Token string `json:"token"`
}

// ResponseSettingsGitHub reports whether OAuth credentials are configured.
type ResponseSettingsGitHub struct {
	ClientID   string `json:"client_id"`
	Configured bool   `json:"configured"`
}

// ResponseDisconnect is returned after clearing the GitHub session.
type ResponseDisconnect struct {
	Message string `json:"message"`
}
