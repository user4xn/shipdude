package github

import (
	"crypto/rand"
	"fmt"
	"net/http"

	"shipdude/internal/dto"
	"shipdude/internal/factory"
	"shipdude/pkg/client"
	"shipdude/pkg/util"
)

type handler struct {
	service Service
}

// NewHandler builds the GitHub HTTP handler.
func NewHandler(f *factory.Factory) *handler {
	return &handler{service: NewService(f)}
}

// GetSettings reports whether the GitHub OAuth app is configured.
func (h *handler) GetSettings(w http.ResponseWriter, r *http.Request) {
	clientID, configured, err := h.service.GetSettings(r.Context())
	if err != nil {
		util.RespondError(w, err)
		return
	}

	res := dto.ResponseSettingsGitHub{ClientID: clientID, Configured: configured}
	util.RespondJSON(w, http.StatusOK, "Successfully retrieved GitHub settings", &res)
}

// PostSettings persists the GitHub OAuth client credentials.
func (h *handler) PostSettings(w http.ResponseWriter, r *http.Request) {
	var body dto.PayloadSettingsGitHub
	if err := util.DecodeJSON(r, &body); err != nil {
		util.RespondError(w, util.NewBadRequestError("Invalid JSON body", err))
		return
	}

	if err := h.service.SaveSettings(r.Context(), body.ClientID, body.ClientSecret); err != nil {
		util.RespondError(w, err)
		return
	}

	util.RespondJSON(w, http.StatusOK, "Successfully saved GitHub settings", nil)
}

// Login redirects the operator to GitHub's OAuth consent screen.
func (h *handler) Login(w http.ResponseWriter, r *http.Request) {
	clientID := h.service.ClientID(r.Context())
	if clientID == "" {
		util.RespondError(w, util.NewBadRequestError("GitHub Client ID is not configured", nil))
		return
	}

	stateBytes := make([]byte, 16)
	if _, err := rand.Read(stateBytes); err != nil {
		util.RespondError(w, util.NewInternalServerError("Failed to generate login state", err))
		return
	}
	state := fmt.Sprintf("%x", stateBytes)
	h.service.RegisterOAuthState(state)

	redirectURI := fmt.Sprintf("%s/api/v1/auth/github/callback", util.ServerURL(r))
	oauthURL := fmt.Sprintf("%s/login/oauth/authorize?client_id=%s&redirect_uri=%s&scope=repo%%20write:packages%%20read:org%%20workflow&state=%s",
		client.OAuthURL, clientID, redirectURI, state)

	http.Redirect(w, r, oauthURL, http.StatusTemporaryRedirect)
}

// Callback exchanges the OAuth code for a token and stores it.
func (h *handler) Callback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")

	if code == "" || state == "" {
		util.RespondError(w, util.NewBadRequestError("Missing code or state parameters", nil))
		return
	}

	if !h.service.ConsumeOAuthState(state) {
		util.RespondError(w, util.NewForbiddenError("Invalid state token (CSRF failure)", nil))
		return
	}

	redirectURI := fmt.Sprintf("%s/api/v1/auth/github/callback", util.ServerURL(r))
	token, err := client.ExchangeCodeForToken(h.service.ClientID(r.Context()), h.service.ClientSecret(r.Context()), code, redirectURI)
	if err != nil {
		util.RespondError(w, util.NewInternalServerError("OAuth token exchange failed", err))
		return
	}

	if err := h.service.SaveOAuthToken(r.Context(), token); err != nil {
		util.RespondError(w, err)
		return
	}

	http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
}

// GetRepos lists repositories the authenticated user can push to.
func (h *handler) GetRepos(w http.ResponseWriter, r *http.Request) {
	repos, err := h.service.ListRepositories(r.Context())
	if err != nil {
		util.RespondError(w, err)
		return
	}

	util.RespondJSON(w, http.StatusOK, "Successfully retrieved repositories", repos)
}

// GetUser returns the authenticated GitHub profile.
func (h *handler) GetUser(w http.ResponseWriter, r *http.Request) {
	profile, err := h.service.GetUserProfile(r.Context())
	if err != nil {
		util.RespondError(w, err)
		return
	}

	util.RespondJSON(w, http.StatusOK, "Successfully retrieved GitHub user", profile)
}

// GetBranches lists the branches of a repository.
func (h *handler) GetBranches(w http.ResponseWriter, r *http.Request) {
	repo := r.URL.Query().Get("repo")
	token := r.URL.Query().Get("token")

	branches, err := h.service.ListBranches(r.Context(), repo, token)
	if err != nil {
		util.RespondError(w, err)
		return
	}

	util.RespondJSON(w, http.StatusOK, "Successfully retrieved branches", branches)
}

// Disconnect clears the stored GitHub OAuth token.
func (h *handler) Disconnect(w http.ResponseWriter, r *http.Request) {
	if err := h.service.Disconnect(r.Context()); err != nil {
		util.RespondError(w, err)
		return
	}

	util.RespondJSON(w, http.StatusOK, "Successfully disconnected from GitHub", nil)
}
