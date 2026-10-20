package consts

// SettingKey is a typed key for the encrypted key/value settings store.
type SettingKey string

const (
	SettingGitHubClientID     SettingKey = "github_client_id"
	SettingGitHubClientSecret SettingKey = "github_client_secret"
	SettingGitHubOAuthToken   SettingKey = "github_oauth_token"

	SettingLastVPSIP         SettingKey = "last_vps_ip"
	SettingLastVPSPort       SettingKey = "last_vps_port"
	SettingLastVPSUser       SettingKey = "last_vps_user"
	SettingLastVPSAuthMethod SettingKey = "last_vps_auth_method"
	SettingLastVPSPassword   SettingKey = "last_vps_password"
	SettingLastVPSPrivateKey SettingKey = "last_vps_private_key"
)

// String returns the raw key persisted in the settings table.
func (k SettingKey) String() string {
	return string(k)
}
