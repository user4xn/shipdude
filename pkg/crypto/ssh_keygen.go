package crypto

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"

	"golang.org/x/crypto/ssh"
)

// SSHKeyPair holds a freshly generated RSA key pair rendered as PEM and
// authorized_keys strings.
type SSHKeyPair struct {
	PrivateKeyPEM string
	PublicSSHKey  string
}

// GenerateSSHKeyPair creates a 2048-bit RSA key pair suitable for
// authenticating GitHub Actions deployments against a VPS.
func GenerateSSHKeyPair() (*SSHKeyPair, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}

	privBlock := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}

	publicKey, err := ssh.NewPublicKey(&privateKey.PublicKey)
	if err != nil {
		return nil, err
	}

	return &SSHKeyPair{
		PrivateKeyPEM: string(pem.EncodeToMemory(privBlock)),
		PublicSSHKey:  string(ssh.MarshalAuthorizedKey(publicKey)),
	}, nil
}
