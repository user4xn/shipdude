package client

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"

	"shipdude/pkg/consts"
)

// SSHCredentials describes how to reach a target VPS over SSH.
type SSHCredentials struct {
	IPAddress  string            `json:"ip_address"`
	Port       int               `json:"ssh_port"`
	User       string            `json:"ssh_user"`
	AuthMethod consts.AuthMethod `json:"auth_method"`
	Password   string            `json:"password,omitempty"`
	PrivateKey string            `json:"private_key,omitempty"`
}

// NewSSHClient dials the target and completes the SSH handshake using the
// credentials' auth method (password or private key).
func NewSSHClient(creds SSHCredentials) (*ssh.Client, error) {
	var authMethods []ssh.AuthMethod

	switch creds.AuthMethod {
	case consts.AuthPassword:
		authMethods = append(authMethods, ssh.Password(creds.Password))
	case consts.AuthKey:
		signer, err := ssh.ParsePrivateKey([]byte(creds.PrivateKey))
		if err != nil {
			return nil, fmt.Errorf("failed to parse private key: %w", err)
		}
		authMethods = append(authMethods, ssh.PublicKeys(signer))
	default:
		return nil, fmt.Errorf("unsupported auth method: %s", creds.AuthMethod)
	}

	config := &ssh.ClientConfig{
		User:            creds.User,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         15 * time.Second,
	}

	addr := net.JoinHostPort(creds.IPAddress, strconv.Itoa(creds.Port))
	conn, err := net.DialTimeout("tcp", addr, 15*time.Second)
	if err != nil {
		return nil, err
	}

	sshConn, chans, reqs, err := ssh.NewClientConn(conn, addr, config)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}

	return ssh.NewClient(sshConn, chans, reqs), nil
}

// RunCommandStream executes a command on the remote host and streams its
// stdout and stderr line by line through onLine.
func RunCommandStream(client *ssh.Client, cmd string, onLine func(string)) error {
	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	stdout, err := session.StdoutPipe()
	if err != nil {
		return err
	}

	stderr, err := session.StderrPipe()
	if err != nil {
		return err
	}

	if err := session.Start(cmd); err != nil {
		return err
	}

	var wg sync.WaitGroup
	var mu sync.Mutex

	safeOnLine := func(line string) {
		mu.Lock()
		defer mu.Unlock()
		onLine(line)
	}

	copyStream := func(r io.Reader) {
		defer wg.Done()
		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			safeOnLine(scanner.Text())
		}
	}

	wg.Add(2)
	go copyStream(stdout)
	go copyStream(stderr)

	wg.Wait()
	return session.Wait()
}
