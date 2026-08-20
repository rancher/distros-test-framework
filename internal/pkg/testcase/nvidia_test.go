package testcase

import (
	"fmt"
	"testing"

	"github.com/avast/retry-go"
)

func TestNvidiaOperatorWorkload(t *testing.T) {
	tests := []struct {
		name                  string
		nodeOS                string
		workload              string
		operatorManagedDriver bool
		nriEnabled            bool
	}{
		{"SLES 15", "sles15", "nvidia-operator-sles.yaml", true, false},
		{"SLES 15 service pack", "sles15sp7", "nvidia-operator-sles.yaml", true, false},
		{"SLES 16 mixed case", "SLES16", "nvidia-operator-sles.yaml", true, false},
		{"RHEL", "rhel10.2", "nvidia-operator.yaml", false, true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := nvidiaOperatorWorkload(test.nodeOS)
			if config.workload != test.workload ||
				config.operatorManagedDriver != test.operatorManagedDriver || config.nriEnabled != test.nriEnabled {
				t.Fatalf("got config %+v, want workload %q managed %t NRI %t",
					config, test.workload, test.operatorManagedDriver, test.nriEnabled)
			}
		})
	}
}

func TestMissingKernelModules(t *testing.T) {
	slesOutput := `nvidia_modeset       2166784  0
video                  81920  1 nvidia_modeset
nvidia_uvm           2465792  8
nvidia              16265216  35 nvidia_uvm,nvidia_modeset`

	if missing := missingKernelModules(slesOutput, []string{"nvidia", "nvidia_uvm"}); len(missing) != 0 {
		t.Fatalf("SLES compute modules reported missing: %v", missing)
	}
	if kernelModuleLoaded(slesOutput, "nvidia_drm") {
		t.Fatal("nvidia_drm should not be reported as loaded")
	}
	if !kernelModuleLoaded("nouveau  123  0\n", "nouveau") {
		t.Fatal("nouveau should be reported as loaded")
	}
	if kernelModuleLoaded("nouveau_drm  123  0\n", "nouveau") {
		t.Fatal("module matching must use the exact lsmod name")
	}
}

func TestNvidiaDriverModeValidation(t *testing.T) {
	tests := []struct {
		name        string
		managed     bool
		label       string
		libraryRoot string
	}{
		{"pre-installed", false, "pre-installed", "/usr"},
		{"operator managed", true, "true", "/run/nvidia/driver"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := nvidiaDriverLabelValue(test.managed); got != test.label {
				t.Fatalf("got driver label %q, want %q", got, test.label)
			}
			if got := nvidiaDriverLibraryRoot(test.managed); got != test.libraryRoot {
				t.Fatalf("got library root %q, want %q", got, test.libraryRoot)
			}
		})
	}
}

func TestValidateNvidiaNodeLabels(t *testing.T) {
	baseLabels := `"nvidia.com/cuda.driver.major":"595",` +
		`"nvidia.com/gpu.machine":"true",` +
		`"nvidia.com/gpu.count":"1",` +
		`"nvidia.com/gpu.product":"NVIDIA-H200-NVL"`

	tests := []struct {
		name    string
		managed bool
		labels  string
		wantErr bool
	}{
		{"pre-installed labels", false, `"nvidia.com/gpu.deploy.driver":"pre-installed",` + baseLabels, false},
		{"managed labels", true, `"nvidia.com/gpu.deploy.driver":"true",` + baseLabels, false},
		{"wrong driver mode", true, `"nvidia.com/gpu.deploy.driver":"pre-installed",` + baseLabels, true},
		{"missing required label", true, `"nvidia.com/gpu.deploy.driver":"true"`, true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			output := fmt.Sprintf(`{"metadata":{"labels":{%s}}}`, test.labels)
			err := validateNvidiaNodeLabels(output, test.managed)
			if (err != nil) != test.wantErr {
				t.Fatalf("validateNvidiaNodeLabels() error = %v, wantErr %t", err, test.wantErr)
			}
		})
	}
}

func TestManagedNvidiaDriverReady(t *testing.T) {
	tests := []struct {
		name            string
		policyState     string
		daemonSetStatus string
		wantErr         bool
	}{
		{"ready", "ready", "1:1", false},
		{"policy not ready", "notReady", "1:1", true},
		{"daemonset not ready", "ready", "1:0", true},
		{"daemonset absent", "ready", "0:0", true},
		{"malformed daemonset", "ready", ":", true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := managedNvidiaDriverReady(test.policyState, test.daemonSetStatus)
			if (err != nil) != test.wantErr {
				t.Fatalf("managedNvidiaDriverReady() error = %v, wantErr %t", err, test.wantErr)
			}
		})
	}
}

func TestBenchmarkPodPhaseError(t *testing.T) {
	tests := []struct {
		phase       string
		wantErr     bool
		recoverable bool
	}{
		{"Succeeded", false, true},
		{" Succeeded\n", false, true},
		{"Pending", true, true},
		{"Running", true, true},
		{"Unknown", true, true},
		{"", true, true},
		{"Completed", true, true},
		{"Failed", true, false},
	}

	for _, test := range tests {
		t.Run(test.phase, func(t *testing.T) {
			err := benchmarkPodPhaseError(test.phase)
			if (err != nil) != test.wantErr {
				t.Fatalf("benchmarkPodPhaseError(%q) error = %v, wantErr %t", test.phase, err, test.wantErr)
			}
			if err != nil && retry.IsRecoverable(err) != test.recoverable {
				t.Fatalf("benchmarkPodPhaseError(%q) recoverable = %t, want %t",
					test.phase, retry.IsRecoverable(err), test.recoverable)
			}
		})
	}
}
