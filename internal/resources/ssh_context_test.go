package resources

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
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

// A peer that authenticates but never answers the session-open request must not outlive ctx.
func TestNewSessionContextBoundsUnansweredSessionOpen(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &ssh.ServerConfig{NoClientAuth: true}
	cfg.AddHostKey(signer)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		c, acceptErr := ln.Accept()
		if acceptErr != nil {
			return
		}
		_, chans, reqs, handshakeErr := ssh.NewServerConn(c, cfg)
		if handshakeErr != nil {
			return
		}
		go ssh.DiscardRequests(reqs)
		for range chans { // never accept or reject: the open stays unanswered
		}
	}()

	conn, err := ssh.Dial("tcp", ln.Addr().String(), &ssh.ClientConfig{
		User:            "test",
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err = newSessionContext(ctx, conn)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline error, got %v", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("session open outlived its context: %s", elapsed)
	}
}
