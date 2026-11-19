package qainfra

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rancher/distros-test-framework/internal/provisioning/driver"
)

func TestValidateWindowsCNI(t *testing.T) {
	tests := []struct {
		name, product, cni, flags, winNodes string
		wantErr                             bool
	}{
		{"no windows nodes skips check", "rke2", "", "", "", false},
		{"zero windows nodes skips check", "k3s", "", "", "0", false},
		{"calico via CNI", "rke2", "calico", "", "1", false},
		{"flannel via server flags", "rke2", "", "cni: flannel", "2", false},
		{"default canal rejected", "rke2", "", "", "1", true},
		{"explicit canal rejected", "rke2", "canal", "", "1", true},
		{"k3s rejected", "k3s", "calico", "", "1", true},
		{"conflicting CNI and flags rejected", "rke2", "calico", "cni: flannel", "1", true},
		{"invalid count rejected", "rke2", "calico", "", "abc", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("no_of_windows_worker_nodes", tt.winNodes)
			t.Setenv("NO_OF_WINDOWS_WORKER_NODES", "")
			cfg := &driver.InfraConfig{
				Product: tt.product,
				CNI:     tt.cni,
				Cluster: &driver.Cluster{},
			}
			cfg.Cluster.Config.ServerFlags = tt.flags

			if err := validateWindowsCNI(cfg); (err != nil) != tt.wantErr {
				t.Fatalf("validateWindowsCNI() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateWindowsRejectsAirgap(t *testing.T) {
	t.Setenv("no_of_windows_worker_nodes", "1")
	t.Setenv("NO_OF_WINDOWS_WORKER_NODES", "")
	cfg := &driver.InfraConfig{Product: "rke2", CNI: "calico", Cluster: &driver.Cluster{}}
	cfg.InfraProvisioner = &driver.InfraProvisionerConfig{AirgapSetup: true}
	if err := validateWindowsCNI(cfg); err == nil {
		t.Fatal("expected windows+airgap to be rejected")
	}
}

func TestValidateWindowsFromTFVarsNodes(t *testing.T) {
	clearRuntimeEnvs(t)
	root := t.TempDir()
	dir := filepath.Join(root, "infrastructure", "qainfra")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	blob := `nodes = [{ count = 1, role = ["etcd"] }, { count = 1, role = ["worker"], os = "windows" }]` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "vars.tfvars"), []byte(blob), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := &driver.InfraConfig{Product: "rke2", CNI: "canal", Cluster: &driver.Cluster{}}
	cfg.InfraProvisioner = &driver.InfraProvisionerConfig{RootDir: root}
	if err := validateWindowsCNI(cfg); err == nil {
		t.Fatal("expected CNI validation for windows nodes declared in TFVARS")
	}
}
