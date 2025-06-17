package sftp

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"github.com/alist-org/alist/v3/cmd/flags"
	"github.com/alist-org/alist/v3/pkg/utils"
	"golang.org/x/crypto/ssh"
	"os"
	"path/filepath"
)

var SSHSigners []ssh.Signer

func InitHostKey() {
	if SSHSigners != nil {
		return
	}
	sshPath := filepath.Join(flags.DataDir, "ssh")
	if !utils.Exists(sshPath) {
		err := utils.CreateNestedDirectory(sshPath)
		if err != nil {
			utils.Log.Fatalf("failed to create ssh directory: %+v", err)
			return
		}
	}
	SSHSigners = make([]ssh.Signer, 0, 4)
	if rsaKey, ok := LoadOrGenerateRSAHostKey(sshPath); ok {
		SSHSigners = append(SSHSigners, rsaKey)
	}
	// TODO Add keys for other encryption algorithms
}

func LoadOrGenerateRSAHostKey(parentDir string) (ssh.Signer, bool) {
	privateKeyPath := filepath.Join(parentDir, "ssh_host_rsa_key")
	publicKeyPath := filepath.Join(parentDir, "ssh_host_rsa_key.pub")
	privateKeyBytes, err := os.ReadFile(privateKeyPath)
	if err == nil {
		var privateKey *rsa.PrivateKey
		privateKey, err = rsaDecodePrivateKey(privateKeyBytes)
		if err == nil {
			var ret ssh.Signer
			ret, err = ssh.NewSignerFromKey(privateKey)
			if err == nil {
				return ret, true
			}
		}
	}
	_ = os.Remove(privateKeyPath)
	_ = os.Remove(publicKeyPath)
	privateKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		utils.Log.Fatalf("failed to generate RSA private key: %+v", err)
		return nil, false
	}
	publicKey, err := ssh.NewPublicKey(&privateKey.PublicKey)
	if err != nil {
		utils.Log.Fatalf("failed to generate RSA public key: %+v", err)
		return nil, false
	}
	ret, err := ssh.NewSignerFromKey(privateKey)
	if err != nil {
		utils.Log.Fatalf("failed to generate RSA signer: %+v", err)
		return nil, false
	}
	privateBytes := rsaEncodePrivateKey(privateKey)
	publicBytes := ssh.MarshalAuthorizedKey(publicKey)
	err = os.WriteFile(privateKeyPath, privateBytes, 0600)
	if err != nil {
		utils.Log.Fatalf("failed to write RSA private key to file: %+v", err)
		return nil, false
	}
	err = os.WriteFile(publicKeyPath, publicBytes, 0644)
	if err != nil {
		_ = os.Remove(privateKeyPath)
		utils.Log.Fatalf("failed to write RSA public key to file: %+v", err)
		return nil, false
	}
	return ret, true
}

func rsaEncodePrivateKey(privateKey *rsa.PrivateKey) []byte {
	privateKeyBytes := x509.MarshalPKCS1PrivateKey(privateKey)
	privateBlock := &pem.Block{
		Type:    "RSA PRIVATE KEY",
		Headers: nil,
		Bytes:   privateKeyBytes,
	}
	return pem.EncodeToMemory(privateBlock)
}

func rsaDecodePrivateKey(bytes []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(bytes)
	if block == nil {
		return nil, fmt.Errorf("failed to parse PEM block containing the key")
	}
	privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	return privateKey, nil
}
