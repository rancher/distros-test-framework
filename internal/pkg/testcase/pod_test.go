package testcase

import (
	"testing"

	"github.com/onsi/gomega"

	"github.com/rancher/distros-test-framework/internal/provisioning/driver"
	"github.com/rancher/distros-test-framework/internal/resources"
)

func TestProcessPodStatusSecurityResponder(t *testing.T) {
	tests := []struct {
		name       string
		product    string
		version    string
		namespace  string
		podName    string
		status     string
		wantFailed bool
	}{
		{"completed run on rke2 1.37", "rke2", "v1.37.1+rke2r1", "kube-system",
			"rke2-security-responder-29886782-lc5tm", statusCompleted, false},
		{"running run on rke2 1.37", "rke2", "v1.37.1+rke2r1", "kube-system",
			"rke2-security-responder-29886782-lc5tm", statusRunning, false},
		{"completed run on rke2 1.38 rc", "rke2", "v1.38.0-rc1+rke2r1", "kube-system",
			"rke2-security-responder-29886782-lc5tm", statusCompleted, false},
		{"failed run on rke2 1.37", "rke2", "v1.37.1+rke2r1", "kube-system",
			"rke2-security-responder-29886782-lc5tm", "Error", true},
		{"completed pod on rke2 1.36", "rke2", "v1.36.5+rke2r1", "kube-system",
			"rke2-security-responder-29886782-lc5tm", statusCompleted, true},
		{"completed pod on k3s", "k3s", "v1.37.1+k3s1", "kube-system",
			"rke2-security-responder-29886782-lc5tm", statusCompleted, true},
		{"completed pod in another namespace", "rke2", "v1.37.1+rke2r1", "default",
			"rke2-security-responder-29886782-lc5tm", statusCompleted, true},
		{"completed pod with a commit install version", "rke2", "0123456789abcdef", "kube-system",
			"rke2-security-responder-29886782-lc5tm", statusCompleted, true},
		{"other completed pod on rke2 1.37", "rke2", "v1.37.1+rke2r1", "kube-system",
			"rke2-coredns-rke2-coredns-54d5f755bb-j4fsk", statusCompleted, true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var failures []string
			g := gomega.NewGomega(func(message string, _ ...int) {
				failures = append(failures, message)
			})

			cluster := &driver.Cluster{Config: driver.Config{Product: test.product, Version: test.version}}
			pod := &resources.Pod{NameSpace: test.namespace, Name: test.podName, Status: test.status}

			processPodStatus(cluster, g, pod, nil, nil)

			if gotFailed := len(failures) > 0; gotFailed != test.wantFailed {
				t.Fatalf("pod %s with status %s on %s %s: failed=%t, want %t (%v)",
					test.podName, test.status, test.product, test.version, gotFailed, test.wantFailed, failures)
			}
		})
	}
}
