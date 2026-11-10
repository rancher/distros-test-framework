package resources

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeTestKey(t *testing.T) string {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "key.pem")
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	if err := os.WriteFile(path, pemBytes, 0o600); err != nil {
		t.Fatal(err)
	}

	return path
}

// A peer that accepts TCP but never speaks SSH must not outlive the context.
func TestConfigureSSHContextBoundsStalledHandshake(t *testing.T) {
	t.Setenv("PROVISIONER_MODULE", "legacy")
	t.Setenv("access_key", writeTestKey(t))
	t.Setenv("aws_user", "test")

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, acceptErr := ln.Accept()
			if acceptErr != nil {
				return
			}
			defer c.Close()
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err = getOrDialSSHContext(ctx, ln.Addr().String())
	if err == nil {
		t.Fatal("expected an error from a stalled handshake")
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("dial outlived its context: %s", elapsed)
	}
}
