package qainfra

import (
	"fmt"
	"strings"
	"testing"

	"github.com/rancher/distros-test-framework/internal/provisioning/driver"
)

// The playbook chooses server_flags or worker_flags by node role. A global rke2_additional_config
// dict takes precedence on every node, leaking server-only settings such as prime onto agents.
func TestBuildAnsibleArgsForwardsFlagsByRole(t *testing.T) {
	for _, tc := range flagForwardingCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("OPTIONAL_FILES", "")
			t.Setenv("optional_files", "")

			checkFlagForwarding(t, &tc)
		})
	}
}

type flagForwardingCase struct {
	name        string
	product     string
	serverFlags string
	workerFlags string
	wantServer  string
	wantWorker  string
}

func flagForwardingCases() []flagForwardingCase {
	// Jenkins delivers the TFVARS strings with literal "\n" separators.
	serverFlags := `prime: true\nprofile: cis\nselinux: true\nsystem-default-registry: stgregistry.suse.com`
	workerFlags := `profile: cis\nselinux: true`

	return []flagForwardingCase{
		{
			name:        "rke2 keeps server and worker flags separate",
			product:     "rke2",
			serverFlags: serverFlags,
			workerFlags: workerFlags,
			wantServer:  "prime: true\nprofile: cis\nselinux: true\nsystem-default-registry: stgregistry.suse.com",
			wantWorker:  "profile: cis\nselinux: true",
		},
		{
			name:        "rke2 without worker flags does not copy server flags to agents",
			product:     "rke2",
			serverFlags: serverFlags,
			wantServer:  "prime: true\nprofile: cis\nselinux: true\nsystem-default-registry: stgregistry.suse.com",
		},
		{
			name:        "k3s forwards both flag sets",
			product:     "k3s",
			serverFlags: `protect-kernel-defaults: true\nselinux: true`,
			workerFlags: `protect-kernel-defaults: true\nselinux: true`,
			wantServer:  "protect-kernel-defaults: true\nselinux: true",
			wantWorker:  "protect-kernel-defaults: true\nselinux: true",
		},
	}
}

func checkFlagForwarding(t *testing.T, tc *flagForwardingCase) {
	t.Helper()

	cfg := &driver.InfraConfig{
		Product:        tc.product,
		InstallVersion: "v1.37.0+rke2r1",
		Cluster: &driver.Cluster{
			Config: driver.Config{
				ServerFlags: tc.serverFlags,
				WorkerFlags: tc.workerFlags,
			},
		},
		InfraProvisioner: &driver.InfraProvisionerConfig{},
	}

	args, buildErr := buildAnsibleArgs(cfg, "playbook.yml")
	if buildErr != nil {
		t.Fatalf("buildAnsibleArgs: %v", buildErr)
	}

	extraVars := ansibleExtraVars(args)
	for _, value := range extraVars {
		if strings.Contains(value, "rke2_additional_config") {
			t.Fatalf("rke2_additional_config must never be sent; it overrides role selection: %q", value)
		}
	}

	assertExtraVar(t, extraVars, "server_flags", tc.wantServer)
	assertExtraVar(t, extraVars, "worker_flags", tc.wantWorker)
}

func ansibleExtraVars(args []string) []string {
	var values []string
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "--extra-vars" {
			values = append(values, args[i+1])
		}
	}

	return values
}

// assertExtraVar checks the exact quoted value; want "" means the variable must be absent.
func assertExtraVar(t *testing.T, extraVars []string, name, want string) {
	t.Helper()

	var found []string
	for _, value := range extraVars {
		if strings.HasPrefix(value, name+"=") {
			found = append(found, value)
		}
	}

	if want == "" {
		if len(found) != 0 {
			t.Errorf("%s must be absent, got %q", name, found)
		}

		return
	}

	expected := fmt.Sprintf("%s=%q", name, want)
	if len(found) != 1 || found[0] != expected {
		t.Errorf("%s: want exactly %q, got %q", name, expected, found)
	}
}
