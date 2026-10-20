package project

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/binary"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"

	"shipdude/pkg/crypto"
)

// mustKeyPair generates an RSA private key PEM for use as stored server
// credentials in tests.
func mustKeyPair(t *testing.T) string {
	t.Helper()
	pair, err := crypto.GenerateSSHKeyPair()
	if err != nil {
		t.Fatalf("failed to generate ssh key pair: %v", err)
	}
	return pair.PrivateKeyPEM
}

func generateTestHostKey(t *testing.T) ssh.Signer {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate test RSA key: %v", err)
	}
	block := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}
	signer, err := ssh.ParsePrivateKey(pem.EncodeToMemory(block))
	if err != nil {
		t.Fatalf("failed to parse private test key: %v", err)
	}
	return signer
}

// startMockSSHServer starts a fake SSH daemon that answers deployment,
// container-control, telemetry and log-stream commands.
func startMockSSHServer(t *testing.T, hostKey ssh.Signer, user, password string) (string, io.Closer) {
	t.Helper()

	config := &ssh.ServerConfig{
		PasswordCallback: func(conn ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
			if conn.User() == user && string(pass) == password {
				return nil, nil
			}
			return nil, fmt.Errorf("auth failed")
		},
		PublicKeyCallback: func(conn ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			return nil, nil
		},
	}
	config.AddHostKey(hostKey)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}

	go func() {
		for {
			nConn, err := listener.Accept()
			if err != nil {
				return
			}
			go serveConn(nConn, config)
		}
	}()

	return listener.Addr().String(), listener
}

func serveConn(c net.Conn, config *ssh.ServerConfig) {
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
		go func(in <-chan *ssh.Request, ch ssh.Channel) {
			defer ch.Close()
			for req := range in {
				if req.Type != "exec" {
					req.Reply(false, nil)
					continue
				}
				out, status := mockResponse(string(req.Payload[4 : 4+req.Payload[3]]))
				ch.Write([]byte(out))
				req.Reply(true, nil)
				payload := make([]byte, 4)
				binary.BigEndian.PutUint32(payload, status)
				ch.SendRequest("exit-status", false, payload)
				return
			}
		}(requests, channel)
	}
}

// mockCommandHook memungkinkan test tertentu mengganti balasan perintah SSH,
// termasuk mensimulasikan exit status bukan nol (mis. `caddy reload` gagal).
// Tanda tangan: func(cmd) (output string, exitStatus uint32, handled bool).
var mockCommandHook func(cmd string) (string, uint32, bool)

// mockResponse memilih balasan dari hook (kalau ada) atau dari tabel default.
func mockResponse(cmd string) (string, uint32) {
	if mockCommandHook != nil {
		if out, status, ok := mockCommandHook(cmd); ok {
			return out, status
		}
	}
	return commandResponse(cmd), 0
}

func commandResponse(cmd string) string {
	switch {
	case strings.Contains(cmd, "cat /etc/os-release"):
		return "PRETTY_NAME=\"Ubuntu 22.04.3 LTS\"\n"
	case strings.Contains(cmd, "docker --version"):
		return "Docker version 25.0.3\n"
	case strings.Contains(cmd, "docker compose version"):
		return "Docker Compose version v2.24.5\n"
	case strings.Contains(cmd, "authorized_keys"):
		return "key injected successfully\n"
	case strings.Contains(cmd, "groups"):
		return "deployer docker admin\n"
	case strings.Contains(cmd, "id -u deployer"):
		return "1001\n"
	case strings.Contains(cmd, "docker ps -a --filter 'name=shipdude-caddy'"):
		return "shipdude-caddy\n"
	case strings.Contains(cmd, "docker ps --filter 'name="):
		return "shipdude-my-website-8080\n"
	case strings.Contains(cmd, "docker stats"):
		return "12.5%;120MB / 2.048GB;5.8%\n"
	case strings.Contains(cmd, "docker logs"):
		return "2026-05-26 21:40:02 [INFO] Listening for HTTP requests on port 8080\n"
	default:
		return ""
	}
}
