package conf

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var Priv []*rsa.PrivateKey
var Pub []*rsa.PublicKey

var KeyDir = "../../keys"

func InitKeys(id int) {
	Priv = make([]*rsa.PrivateKey, N+M)

	// Read private key for this node
	privKeyPath := filepath.Join(KeyDir, fmt.Sprintf("%d.txt", id))
	privKeyData, err := os.ReadFile(privKeyPath)
	if err != nil {
		panic(fmt.Errorf("failed to read private key file: %w", err))
	}

	// Parse private key
	privKeyStr := strings.TrimSpace(string(privKeyData))
	privKeyBytes, err := base64.StdEncoding.DecodeString(privKeyStr)
	if err != nil {
		panic(fmt.Errorf("failed to decode private key: %w", err))
	}

	priv, err := x509.ParsePKCS1PrivateKey(privKeyBytes)
	if err != nil {
		panic(fmt.Errorf("failed to parse private key: %w", err))
	}

	Priv[id] = priv

	// Read public keys for all nodes
	pubKeyPath := filepath.Join(KeyDir, "pub.txt")
	pubKeyData, err := os.ReadFile(pubKeyPath)
	if err != nil {
		panic(fmt.Errorf("failed to read public keys file: %w", err))
	}

	pubKeyStr := string(pubKeyData)
	lines := strings.Split(pubKeyStr, "\n")
	if len(lines) < N+M {
		panic(errors.New("not enough public keys"))
	}

	Pub = make([]*rsa.PublicKey, 0, N+M)
	for i := 0; i < N+M; i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}

		pubKeyBytes, err := base64.StdEncoding.DecodeString(line)
		if err != nil {
			panic(fmt.Errorf("failed to decode public key %d: %w", i, err))
		}

		pub, err := x509.ParsePKCS1PublicKey(pubKeyBytes)
		if err != nil {
			panic(fmt.Errorf("failed to parse public key %d: %w", i, err))
		}

		Pub = append(Pub, pub)
	}
}
