package vps

import (
	"net/http"

	app "shipdude/internal/app"
	"shipdude/internal/dto"
	"shipdude/internal/factory"
	"shipdude/internal/model"
	"shipdude/pkg/client"
	"shipdude/pkg/consts"
	"shipdude/pkg/util"
)

type handler struct {
	service Service
	factory *factory.Factory
}

// NewHandler builds the VPS HTTP handler.
func NewHandler(f *factory.Factory) *handler {
	return &handler{service: NewService(f), factory: f}
}

// Probe runs SSH diagnostics against a target VPS and remembers the credentials.
func (h *handler) Probe(w http.ResponseWriter, r *http.Request) {
	var body dto.PayloadVPSConnection
	if err := util.DecodeJSON(r, &body); err != nil {
		util.RespondError(w, util.NewBadRequestError("Invalid request payload", err))
		return
	}

	creds := client.SSHCredentials{
		IPAddress:  body.IPAddress,
		Port:       body.SSHPort,
		User:       body.SSHUser,
		AuthMethod: consts.AuthMethod(body.AuthMethod),
		Password:   body.Password,
		PrivateKey: body.PrivateKey,
	}

	diag, err := h.service.Probe(r.Context(), creds)
	if err != nil {
		util.RespondError(w, err)
		return
	}

	util.RespondJSON(w, http.StatusOK, "Successfully probed VPS", diag)
}

// GetLastConnection returns the saved SSH connection details.
func (h *handler) GetLastConnection(w http.ResponseWriter, r *http.Request) {
	conn, err := h.service.LastConnection(r.Context())
	if err != nil {
		util.RespondError(w, err)
		return
	}

	res := dto.ResponseVPSConnection{
		IPAddress:  conn.IPAddress,
		SSHPort:    conn.SSHPort,
		SSHUser:    conn.SSHUser,
		AuthMethod: conn.AuthMethod,
		Password:   conn.Password,
		PrivateKey: conn.PrivateKey,
	}
	util.RespondJSON(w, http.StatusOK, "Successfully retrieved last connection", &res)
}

// PostLastConnection persists SSH connection details manually.
func (h *handler) PostLastConnection(w http.ResponseWriter, r *http.Request) {
	var body dto.PayloadVPSConnection
	if err := util.DecodeJSON(r, &body); err != nil {
		util.RespondError(w, util.NewBadRequestError("Invalid request payload", err))
		return
	}

	conn := model.LastConnection{
		IPAddress:  body.IPAddress,
		SSHPort:    body.SSHPort,
		SSHUser:    body.SSHUser,
		AuthMethod: body.AuthMethod,
		Password:   body.Password,
		PrivateKey: body.PrivateKey,
	}

	if err := h.service.SaveLastConnection(r.Context(), conn); err != nil {
		util.RespondError(w, err)
		return
	}

	util.RespondJSON(w, http.StatusOK, "Successfully saved last connection", nil)
}

// Provision starts an asynchronous VPS bootstrap job.
func (h *handler) Provision(w http.ResponseWriter, r *http.Request) {
	var body dto.PayloadVPSProvision
	if err := util.DecodeJSON(r, &body); err != nil {
		util.RespondError(w, util.NewBadRequestError("Invalid request payload", err))
		return
	}

	jobID, err := h.service.Provision(ProvisionRequest{
		ServerID:      body.ServerID,
		IPAddress:     body.IPAddress,
		Port:          body.SSHPort,
		User:          body.SSHUser,
		AuthMethod:    consts.AuthMethod(body.AuthMethod),
		Password:      body.Password,
		PrivateKey:    body.PrivateKey,
		InstallDocker: body.InstallDocker,
	})
	if err != nil {
		util.RespondError(w, err)
		return
	}

	util.RespondJSON(w, http.StatusOK, "Successfully started provisioning", &dto.ResponseJobAck{
		Status: string(consts.StatusStarted),
		JobID:  jobID,
	})
}

// ProvisionLogs streams the bootstrap job logs over a WebSocket connection.
func (h *handler) ProvisionLogs(w http.ResponseWriter, r *http.Request) {
	ip := r.URL.Query().Get("ip")
	jobID := consts.JobID(consts.JobProvision, ip)
	app.StreamJobLogs(w, r, h.factory.Hub, jobID)
}
