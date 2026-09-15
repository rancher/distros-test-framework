package qainfra

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rancher/distros-test-framework/internal/provisioning/driver"
)

// createAnsibleVarsFile must never invent a CNI: an empty CNI means "product
// default" and the playbook then writes no `cni:` key, so RKE2 picks canal.
func TestCreateAnsibleVarsFileCNI(t *testing.T) {
	cases := []struct {
		name    string
		product string
		cni     string
		wantCNI string // "" = the cni key must be absent
	}{
		{name: "rke2 without cni -> no cni key (RKE2 default canal)", product: "rke2", cni: ""},
		{name: "rke2 with blank cni -> no cni key", product: "rke2", cni: "  "},
		{name: "rke2 with explicit cni is forwarded", product: "rke2", cni: "cilium", wantCNI: "cni: 'cilium'"},
		{
			name: "rke2 multi cni is forwarded verbatim", product: "rke2",
			cni: "multus,calico", wantCNI: "cni: 'multus,calico'",
		},
		{name: "k3s never gets a cni key", product: "k3s", cni: "calico"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			cfg := &driver.InfraConfig{
				Product:        tc.product,
				InstallVersion: "v1.37.0+rke2r1",
				CNI:            tc.cni,
				InfraProvisioner: &driver.InfraProvisionerConfig{
					KubeconfigPath: filepath.Join(dir, "kubeconfig.yaml"),
					Ansible:        driver.Ansible{Dir: dir},
				},
			}

			if err := createAnsibleVarsFile(cfg); err != nil {
				t.Fatalf("createAnsibleVarsFile: %v", err)
			}
			data, err := os.ReadFile(filepath.Join(dir, "vars.yaml"))
			if err != nil {
				t.Fatalf("read vars.yaml: %v", err)
			}
			got := string(data)

			if !strings.Contains(got, "kubernetes_version: 'v1.37.0+rke2r1'") {
				t.Errorf("kubernetes_version missing in:\n%s", got)
			}
			switch {
			case tc.wantCNI == "" && strings.Contains(got, "cni:"):
				t.Errorf("cni key must be absent for %q/%q, got:\n%s", tc.product, tc.cni, got)
			case tc.wantCNI != "" && !strings.Contains(got, tc.wantCNI):
				t.Errorf("want %q in vars.yaml, got:\n%s", tc.wantCNI, got)
			}
			if strings.Contains(got, "calico") && !strings.Contains(tc.cni, "calico") {
				t.Errorf("calico must not be injected as a default, got:\n%s", got)
			}
		})
	}
}
