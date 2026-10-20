package client

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"

	"shipdude/pkg/consts"
)

func generateTestHostKey(t *testing.T) ssh.Signer {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Failed to generate test RSA key: %v", err)
	}

	block := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}
	signer, err := ssh.ParsePrivateKey(pem.EncodeToMemory(block))
	if err != nil {
		t.Fatalf("Failed to parse private test key: %v", err)
	}
	return signer
}

func startMockSSHServer(t *testing.T, hostKey ssh.Signer, user, password string) (string, io.Closer) {
	config := &ssh.ServerConfig{
		PasswordCallback: func(conn ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
			if conn.User() == user && string(pass) == password {
				return nil, nil
			}
			return nil, fmt.Errorf("auth failed")
		},
		PublicKeyCallback: func(conn ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			if conn.User() == user {
				return nil, nil
			}
			return nil, fmt.Errorf("auth failed")
		},
	}
	config.AddHostKey(hostKey)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to listen: %v", err)
	}

	go func() {
		for {
			nConn, err := listener.Accept()
			if err != nil {
				return
			}
			go serveMockSession(nConn, config)
		}
	}()

	return listener.Addr().String(), listener
}

func serveMockSession(c net.Conn, config *ssh.ServerConfig) {
	_, chans, reqs, err := ssh.NewServerConn(c, config)
	if err != nil {
		return
	}
	go ssh.DiscardRequests(reqs)

	for newChan := range chans {
		if newChan.ChannelType() != "session" {
			newChan.Reject(ssh.UnknownChannelType, "unknown channel type")
			continue
		}
		channel, requests, err := newChan.Accept()
		if err != nil {
			return
		}
		go respondToExec(requests, channel)
	}
}

func respondToExec(in <-chan *ssh.Request, ch ssh.Channel) {
	defer ch.Close()
	for req := range in {
		if req.Type != "exec" {
			req.Reply(false, nil)
			continue
		}

		cmd := string(req.Payload[4 : 4+req.Payload[3]])
		var response string
		switch {
		case strings.Contains(cmd, "cat /etc/os-release"):
			response = "PRETTY_NAME=\"Ubuntu 22.04.3 LTS\"\n"
		case strings.Contains(cmd, "docker --version"):
			response = "Docker version 25.0.3, build f0df22d\n"
		case strings.Contains(cmd, "docker compose version"):
			response = "Docker Compose version v2.24.5\n"
		case strings.Contains(cmd, "groups"):
			response = "deployer docker admin\n"
		case strings.Contains(cmd, "id -u deployer"):
			response = "1001\n"
		case strings.Contains(cmd, "docker ps -a --filter 'name=shipdude-caddy'"):
			response = "shipdude-caddy\n"
		}

		ch.Write([]byte(response))
		req.Reply(true, nil)
		ch.SendRequest("exit-status", false, make([]byte, 4))
		return
	}
}

func TestRunVPSDiagnostics_Success(t *testing.T) {
	hostKey := generateTestHostKey(t)
	addr, server := startMockSSHServer(t, hostKey, "deployer", "mypassword")
	defer server.Close()

	host, portStr, _ := net.SplitHostPort(addr)
	var port int
	fmt.Sscanf(portStr, "%d", &port)

	diag, err := RunVPSDiagnostics(SSHCredentials{
		IPAddress:  host,
		Port:       port,
		User:       "deployer",
		AuthMethod: consts.AuthPassword,
		Password:   "mypassword",
	})
	if err != nil {
		t.Fatalf("Unexpected error running diagnostics: %v", err)
	}

	if !diag.Connected {
		t.Errorf("Expected Connected=true, got false (error: %s)", diag.Error)
	}
	if diag.OS != "Ubuntu 22.04.3 LTS" {
		t.Errorf("Expected OS 'Ubuntu 22.04.3 LTS', got '%s'", diag.OS)
	}
	if !strings.Contains(diag.DockerVersion, "25.0.3") {
		t.Errorf("Expected Docker version containing '25.0.3', got '%s'", diag.DockerVersion)
	}
	if !strings.Contains(diag.ComposeVersion, "v2.24.5") {
		t.Errorf("Expected Compose version containing 'v2.24.5', got '%s'", diag.ComposeVersion)
	}
	if !diag.InDockerGroup {
		t.Errorf("Expected InDockerGroup=true, got false")
	}
	if !diag.HasSudo {
		t.Errorf("Expected HasSudo=true, got false")
	}
	if !diag.Bootstrapped {
		t.Errorf("Expected Bootstrapped=true, got false")
	}
}

func TestRunVPSDiagnostics_AuthFailure(t *testing.T) {
	hostKey := generateTestHostKey(t)
	addr, server := startMockSSHServer(t, hostKey, "deployer", "mypassword")
	defer server.Close()

	host, portStr, _ := net.SplitHostPort(addr)
	var port int
	fmt.Sscanf(portStr, "%d", &port)

	diag, err := RunVPSDiagnostics(SSHCredentials{
		IPAddress:  host,
		Port:       port,
		User:       "deployer",
		AuthMethod: consts.AuthPassword,
		Password:   "wrongpassword",
	})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if diag.Connected {
		t.Errorf("Expected Connected=false on wrong password, got true")
	}
	if !strings.Contains(diag.Error, "ssh: handshake failed") {
		t.Errorf("Expected handshake failure error, got: %s", diag.Error)
	}
}

func TestRunVPSDiagnostics_PrivateKeySuccess(t *testing.T) {
	clientKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Failed to generate client key: %v", err)
	}
	clientPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(clientKey)})

	hostKey := generateTestHostKey(t)
	addr, server := startMockSSHServer(t, hostKey, "deployer", "")
	defer server.Close()

	host, portStr, _ := net.SplitHostPort(addr)
	var port int
	fmt.Sscanf(portStr, "%d", &port)

	diag, err := RunVPSDiagnostics(SSHCredentials{
		IPAddress:  host,
		Port:       port,
		User:       "deployer",
		AuthMethod: consts.AuthKey,
		PrivateKey: string(clientPEM),
	})
	if err != nil {
		t.Fatalf("Unexpected error running diagnostics: %v", err)
	}
	if !diag.Connected {
		t.Errorf("Expected Connected=true via private key, got false (error: %s)", diag.Error)
	}
	if diag.OS != "Ubuntu 22.04.3 LTS" {
		t.Errorf("Expected OS 'Ubuntu 22.04.3 LTS', got '%s'", diag.OS)
	}
}
