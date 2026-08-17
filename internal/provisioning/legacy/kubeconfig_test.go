package legacy

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rancher/distros-test-framework/internal/resources"
)

const kubeconfigTemplate = `apiVersion: v1
clusters:
- cluster:
    certificate-authority-data: Q0FEQVRB
    server: %s
  name: default
contexts:
- context:
    cluster: default
    user: default
  name: default
users:
- name: default
  user:
    client-certificate-data: Q0VSVA==
`

// withKubeconfig writes a kubeconfig with the given server URL to a temp file
// and points resources.KubeConfigFile at it for the duration of the test.
func withKubeconfig(t *testing.T, serverURL string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "kubeconfig")
	content := strings.ReplaceAll(kubeconfigTemplate, "%s", serverURL)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	prev := resources.KubeConfigFile
	resources.KubeConfigFile = path
	t.Cleanup(func() { resources.KubeConfigFile = prev })

	return path
}

//nolint:funlen // table-driven test
func TestUpdateKubeConfigLocal(t *testing.T) {
	cases := []struct {
		name       string
		oldServer  string
		newIP      string
		wantServer string
	}{
		{
			name:       "ipv4 keeps port",
			oldServer:  "https://1.2.3.4:6443",
			newIP:      "5.6.7.8",
			wantServer: "server: https://5.6.7.8:6443",
		},
		{
			name:       "nlb dns swapped for node ip",
			oldServer:  "https://dsf-x-nlb.elb.us-east-2.amazonaws.com:6443",
			newIP:      "5.6.7.8",
			wantServer: "server: https://5.6.7.8:6443",
		},
		{
			name:       "ipv6 target gets brackets",
			oldServer:  "https://1.2.3.4:6443",
			newIP:      "2600:1f16::1",
			wantServer: "server: https://[2600:1f16::1]:6443",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := withKubeconfig(t, tc.oldServer)

			encoded, err := updateKubeConfigLocal(tc.newIP, "resname", "rke2")
			if err != nil {
				t.Fatal(err)
			}

			decoded, decErr := base64.StdEncoding.DecodeString(encoded)
			if decErr != nil {
				t.Fatalf("return value is not base64: %v", decErr)
			}
			got := string(decoded)

			if !strings.Contains(got, tc.wantServer) {
				t.Errorf("server entry not rewritten, want %q in:\n%s", tc.wantServer, got)
			}
			// only the server field may change
			for _, untouched := range []string{
				"certificate-authority-data: Q0FEQVRB",
				"client-certificate-data: Q0VSVA==",
				"name: default",
			} {
				if !strings.Contains(got, untouched) {
					t.Errorf("unrelated field changed, missing %q:\n%s", untouched, got)
				}
			}

			onDisk, readErr := os.ReadFile(path)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if string(onDisk) != got {
				t.Error("file on disk differs from returned kubeconfig")
			}
		})
	}
}
