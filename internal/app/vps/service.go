package vps

import (
	"context"
	"database/sql"
	"strconv"

	"shipdude/internal/factory"
	"shipdude/internal/model"
	"shipdude/internal/repository"
	"shipdude/pkg/client"
	"shipdude/pkg/config"
	"shipdude/pkg/consts"
	"shipdude/pkg/crypto"
	"shipdude/pkg/socket"
	"shipdude/pkg/util"
)

// Service is the business logic contract for VPS nodes.
type Service interface {
	Probe(ctx context.Context, creds client.SSHCredentials) (*client.VPSDiagnostics, error)
	LastConnection(ctx context.Context) (*model.LastConnection, error)
	SaveLastConnection(ctx context.Context, conn model.LastConnection) error
	Provision(req ProvisionRequest) (jobID string, err error)
	FindServer(ctx context.Context, id int64) (*model.Server, error)
	SaveServer(ctx context.Context, server model.Server) (int64, error)
}

// ProvisionRequest carries the resolved credentials for a bootstrap job.
type ProvisionRequest struct {
	ServerID      int64
	IPAddress     string
	Port          int
	User          string
	AuthMethod    consts.AuthMethod
	Password      string
	PrivateKey    string
	InstallDocker bool
}

type service struct {
	factory *factory.Factory
	server  repository.Server
	setting repository.Setting
	hub     *socket.Hub
}

// NewService builds the VPS service backed by the factory.
func NewService(f *factory.Factory) Service {
	return &service{
		factory: f,
		server:  f.Server,
		setting: f.Setting,
		hub:     f.Hub,
	}
}

// Probe verifies a target VPS and persists the credentials as last-used.
func (s *service) Probe(ctx context.Context, creds client.SSHCredentials) (*client.VPSDiagnostics, error) {
	if creds.IPAddress == "" || creds.User == "" {
		return nil, util.NewBadRequestError("IP Address and SSH User are required", nil)
	}

	diag, err := client.RunVPSDiagnostics(creds)
	if err != nil {
		return nil, util.NewInternalServerError("Failed to run VPS diagnostics", err)
	}

	s.persistLastConnection(ctx, model.LastConnection{
		IPAddress:  creds.IPAddress,
		SSHPort:    creds.Port,
		SSHUser:    creds.User,
		AuthMethod: string(creds.AuthMethod),
		Password:   creds.Password,
		PrivateKey: creds.PrivateKey,
	})

	return diag, nil
}

// LastConnection returns the stored SSH connection details.
func (s *service) LastConnection(ctx context.Context) (*model.LastConnection, error) {
	ip := s.settingValue(ctx, consts.SettingLastVPSIP)
	portStr := s.settingValue(ctx, consts.SettingLastVPSPort)

	port := config.DefaultSSHPort()
	if portStr != "" {
		if parsed, err := strconv.Atoi(portStr); err == nil {
			port = parsed
		}
	}

	return &model.LastConnection{
		IPAddress:  ip,
		SSHPort:    port,
		SSHUser:    s.settingValue(ctx, consts.SettingLastVPSUser),
		AuthMethod: s.settingValue(ctx, consts.SettingLastVPSAuthMethod),
		Password:   s.settingValue(ctx, consts.SettingLastVPSPassword),
		PrivateKey: s.settingValue(ctx, consts.SettingLastVPSPrivateKey),
	}, nil
}

// SaveLastConnection persists the connection details for the next visit.
func (s *service) SaveLastConnection(ctx context.Context, conn model.LastConnection) error {
	s.persistLastConnection(ctx, conn)
	return nil
}

// Provision resumes a bootstrap job for a VPS node in the background.
func (s *service) Provision(req ProvisionRequest) (string, error) {
	creds, err := s.resolveCredentials(req)
	if err != nil {
		return "", err
	}

	jobID := consts.JobID(consts.JobProvision, creds.IPAddress)
	job := s.hub.GetOrCreateJob(jobID)

	go s.runProvision(job, creds, req.InstallDocker)

	return jobID, nil
}

func (s *service) resolveCredentials(req ProvisionRequest) (client.SSHCredentials, error) {
	if req.ServerID > 0 {
		srv, err := s.FindServer(context.Background(), req.ServerID)
		if err != nil {
			return client.SSHCredentials{}, err
		}
		return client.SSHCredentials{
			IPAddress:  srv.IPAddress,
			Port:       srv.SSHPort,
			User:       srv.SSHUser,
			AuthMethod: consts.AuthKey,
			PrivateKey: srv.SSHPrivateKey,
		}, nil
	}

	if req.IPAddress == "" || req.User == "" {
		return client.SSHCredentials{}, util.NewBadRequestError("IP Address and SSH User are required", nil)
	}

	port := req.Port
	if port <= 0 {
		port = config.DefaultSSHPort()
	}

	return client.SSHCredentials{
		IPAddress:  req.IPAddress,
		Port:       port,
		User:       req.User,
		AuthMethod: req.AuthMethod,
		Password:   req.Password,
		PrivateKey: req.PrivateKey,
	}, nil
}

func (s *service) runProvision(job *socket.Job, creds client.SSHCredentials, installDocker bool) {
	job.Reset()
	job.AddLog("[System] Establishing connection to VPS...")

	sshClient, err := client.NewSSHClient(creds)
	if err != nil {
		job.AddLog("[Error] Failed to connect: " + err.Error())
		return
	}
	defer sshClient.Close()

	keyPair, err := crypto.GenerateSSHKeyPair()
	if err != nil {
		job.AddLog("[Error] Failed to generate SSH keypair: " + err.Error())
		return
	}

	if err := client.BootstrapVPS(sshClient, installDocker, keyPair.PublicSSHKey, job.AddLog); err != nil {
		job.AddLog("[Error] Bootstrapping failed: " + err.Error())
		return
	}

	serverID, err := s.SaveServer(context.Background(), model.Server{
		Name:          "vps-" + creds.IPAddress,
		IPAddress:     creds.IPAddress,
		SSHPort:       creds.Port,
		SSHUser:       "deployer",
		SSHPrivateKey: keyPair.PrivateKeyPEM,
	})
	if err != nil {
		job.AddLog("[Warning] Bootstrapped successfully but failed to save server credentials: " + err.Error())
		return
	}

	job.AddLog("[System] Bootstrapped server successfully saved with ID: " + strconv.FormatInt(serverID, 10))
}

// FindServer returns a server or a typed not-found error.
func (s *service) FindServer(ctx context.Context, id int64) (*model.Server, error) {
	srv, err := s.server.FindOne(ctx, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, util.NewNotFoundError("Server not found", err)
		}
		return nil, util.NewInternalServerError("Failed to find server", err)
	}
	return srv, nil
}

// SaveServer persists a new server node.
func (s *service) SaveServer(ctx context.Context, srv model.Server) (int64, error) {
	id, err := s.server.Store(s.db(), srv)
	if err != nil {
		return 0, util.NewInternalServerError("Failed to save server", err)
	}
	return id, nil
}

func (s *service) persistLastConnection(ctx context.Context, conn model.LastConnection) {
	_ = s.setting.Store(s.db(), consts.SettingLastVPSIP.String(), conn.IPAddress)
	_ = s.setting.Store(s.db(), consts.SettingLastVPSPort.String(), strconv.Itoa(conn.SSHPort))
	_ = s.setting.Store(s.db(), consts.SettingLastVPSUser.String(), conn.SSHUser)
	_ = s.setting.Store(s.db(), consts.SettingLastVPSAuthMethod.String(), conn.AuthMethod)
	_ = s.setting.Store(s.db(), consts.SettingLastVPSPassword.String(), conn.Password)
	_ = s.setting.Store(s.db(), consts.SettingLastVPSPrivateKey.String(), conn.PrivateKey)
}

func (s *service) settingValue(ctx context.Context, key consts.SettingKey) string {
	value, _ := s.setting.FindValue(ctx, key.String())
	return value
}

func (s *service) db() *sql.DB {
	return s.factory.InitDB().SQL()
}
