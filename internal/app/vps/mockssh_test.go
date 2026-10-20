package vps

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
)

// generateTestHostKey creates a throwaway RSA host key for the mock server.
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

// startMockSSHServer spins up a fake SSH daemon that answers the diagnostic
// commands ShipDude issues during a VPS probe.
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
				ch.Write([]byte(diagnosticResponse(string(req.Payload[4 : 4+req.Payload[3]]))))
				req.Reply(true, nil)
				ch.SendRequest("exit-status", false, make([]byte, 4))
				return
			}
		}(requests, channel)
	}
}

func diagnosticResponse(cmd string) string {
	switch {
	case strings.Contains(cmd, "cat /etc/os-release"):
		return "PRETTY_NAME=\"Ubuntu 22.04.3 LTS\"\n"
	case strings.Contains(cmd, "docker --version"):
		return "Docker version 25.0.3\n"
	case strings.Contains(cmd, "docker compose version"):
		return "Docker Compose version v2.24.5\n"
	case strings.Contains(cmd, "groups"):
		return "deployer docker admin\n"
	case strings.Contains(cmd, "id -u deployer"):
		return "1001\n"
	case strings.Contains(cmd, "docker ps -a --filter 'name=shipdude-caddy'"):
		return "shipdude-caddy\n"
	default:
		return ""
	}
}
