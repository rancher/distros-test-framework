package resources

import (
	"strings"
	"testing"

	"github.com/rancher/distros-test-framework/internal/provisioning/driver"
)

func installCluster(product, nodeOS, installMethod string) *driver.Cluster {
	return &driver.Cluster{
		NodeOS: nodeOS,
		Config: driver.Config{Product: product, InstallMethod: installMethod},
	}
}

//nolint:funlen // table-driven test
func TestGetInstallCmd(t *testing.T) {
	cases := []struct {
		name       string
		cluster    *driver.Cluster
		envMethod  string
		installTag string
		nodeType   string
		contains   []string
		absent     []string
	}{
		{
			name:       "k3s version tag",
			cluster:    installCluster("k3s", "rhel9", ""),
			installTag: "v1.33.1+k3s1",
			nodeType:   "server",
			contains:   []string{"get.k3s.io", "INSTALL_K3S_VERSION=v1.33.1+k3s1", "INSTALL_K3S_CHANNEL=", "sh -s - server"},
			absent:     []string{"INSTALL_K3S_COMMIT", "INSTALL_K3S_METHOD", "SKIP_ENABLE"},
		},
		{
			name:       "rke2 commit hash",
			cluster:    installCluster("rke2", "rhel9", ""),
			installTag: "3f2a1bc9d8e7",
			nodeType:   "agent",
			contains:   []string{"get.rke2.io", "INSTALL_RKE2_COMMIT=3f2a1bc9d8e7", "sh -s - agent"},
			absent:     []string{"INSTALL_RKE2_VERSION"},
		},
		{
			name:       "typed install method beats legacy env",
			cluster:    installCluster("rke2", "rhel10", "rpm"),
			envMethod:  "tar",
			installTag: "v1.34.10+rke2r1",
			nodeType:   "server",
			contains:   []string{"INSTALL_RKE2_METHOD=rpm"},
			absent:     []string{"INSTALL_RKE2_METHOD=tar"},
		},
		{
			name:       "legacy env method as fallback",
			cluster:    installCluster("rke2", "rhel10", ""),
			envMethod:  "tar",
			installTag: "v1.34.10+rke2r1",
			nodeType:   "server",
			contains:   []string{"INSTALL_RKE2_METHOD=tar"},
		},
		{
			name:       "k3s slemicro adds skip enable",
			cluster:    installCluster("k3s", "slemicro", ""),
			installTag: "v1.33.1+k3s1",
			nodeType:   "server",
			contains:   []string{"INSTALL_K3S_SKIP_ENABLE=true"},
		},
		{
			name:       "k3s slemicro with install method keeps method and skips skip-enable",
			cluster:    installCluster("k3s", "slemicro", "rpm"),
			installTag: "v1.33.1+k3s1",
			nodeType:   "server",
			contains:   []string{"INSTALL_K3S_METHOD=rpm"},
			absent:     []string{"SKIP_ENABLE"},
		},
		{
			name:       "rke2 slemicro has no skip enable",
			cluster:    installCluster("rke2", "slemicro", ""),
			installTag: "v1.34.10+rke2r1",
			nodeType:   "server",
			absent:     []string{"SKIP_ENABLE"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("install_method", tc.envMethod)

			got := GetInstallCmd(tc.cluster, tc.installTag, tc.nodeType)
			for _, want := range tc.contains {
				if !strings.Contains(got, want) {
					t.Errorf("missing %q in:\n%s", want, got)
				}
			}
			for _, ban := range tc.absent {
				if strings.Contains(got, ban) {
					t.Errorf("unexpected %q in:\n%s", ban, got)
				}
			}
		})
	}
}
