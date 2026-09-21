package qainfra

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/rancher/distros-test-framework/internal/provisioning/driver"
)

const legacyTmpPublicKey = "/tmp/key.pub"

// writeTestPEM writes an unencrypted ECDSA private key in the PEM form the AWS key pairs use.
func writeTestPEM(t *testing.T, dir string) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	path := filepath.Join(dir, "aws_key.pem")
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der}), 0o600); err != nil {
		t.Fatalf("write pem: %v", err)
	}

	return path
}

func TestPrepareSSHKeysWritesPublicKeyIntoRunDir(t *testing.T) {
	pemPath := writeTestPEM(t, t.TempDir())
	// The run dir does not exist yet when the keys are prepared.
	runDir := filepath.Join(t.TempDir(), "state", "rke2-validate-cluster-qainfra-45-3c82d1")

	pubPath, err := prepareSSHKeys(pemPath, runDir)
	if err != nil {
		t.Fatalf("prepareSSHKeys: %v", err)
	}
	if pubPath != filepath.Join(runDir, runPublicKeyName) {
		t.Fatalf("public key must live in the persisted run dir, got %s", pubPath)
	}

	pub, err := os.ReadFile(pubPath)
	if err != nil {
		t.Fatalf("read public key: %v", err)
	}
	if !strings.HasPrefix(string(pub), "ecdsa-sha2-nistp256 ") || !strings.HasSuffix(string(pub), "\n") {
		t.Fatalf("unexpected authorized_keys line: %q", pub)
	}
	if pubPath == legacyTmpPublicKey {
		t.Fatal("public key must not fall back to the container-local /tmp/key.pub")
	}
}

// The persisted path must be what tofu reads: the fallback destroy re-evaluates
// file(var.public_ssh_key) from vars.tfvars in a fresh container.
func TestSetupSSHConfigurationPersistsPublicKeyForTofu(t *testing.T) {
	pemPath := writeTestPEM(t, t.TempDir())
	runDir := filepath.Join(t.TempDir(), "state", "rke2-validate-cluster-qainfra-45-3c82d1")
	infra := &driver.InfraConfig{Cluster: &driver.Cluster{}}
	infra.Cluster.SSH.PrivKeyPath = pemPath
	infra.Cluster.SSH.User = "ec2-user"

	sshCfg, err := setupSSHConfiguration(infra, environmentConfig{runDir: runDir})
	if err != nil {
		t.Fatalf("setupSSHConfiguration: %v", err)
	}
	if sshCfg.PubKeyPath != filepath.Join(runDir, runPublicKeyName) {
		t.Fatalf("PubKeyPath must be inside the run dir, got %s", sshCfg.PubKeyPath)
	}

	tfvars := filepath.Join(t.TempDir(), "vars.tfvars")
	seed := "aws_region = \"us-east-2\"\npublic_ssh_key = \"" + legacyTmpPublicKey + "\"\n"
	if err := os.WriteFile(tfvars, []byte(seed), 0o600); err != nil {
		t.Fatalf("seed tfvars: %v", err)
	}
	if err := setOrAppendTFVar(tfvars, "public_ssh_key", sshCfg.PubKeyPath); err != nil {
		t.Fatalf("setOrAppendTFVar: %v", err)
	}

	got, err := os.ReadFile(tfvars)
	if err != nil {
		t.Fatalf("read tfvars: %v", err)
	}
	want := "public_ssh_key = " + strconv.Quote(sshCfg.PubKeyPath)
	if !strings.Contains(string(got), want) || strings.Contains(string(got), legacyTmpPublicKey) {
		t.Fatalf("vars.tfvars must reference the persisted key only, got:\n%s", got)
	}
	if _, err := os.Stat(sshCfg.PubKeyPath); err != nil {
		t.Fatalf("persisted public key missing: %v", err)
	}
}

func TestPrepareSSHKeysRejectsUnreadablePEM(t *testing.T) {
	_, err := prepareSSHKeys(filepath.Join(t.TempDir(), "missing.pem"), t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "copy private key") {
		t.Fatalf("expected copy error, got %v", err)
	}
}
