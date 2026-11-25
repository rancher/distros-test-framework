package testcase

import (
	"testing"

	"github.com/onsi/gomega"

	"github.com/rancher/distros-test-framework/shared"
)

func TestProcessPodStatusHelmPods(t *testing.T) {
	tests := []struct {
		name, status string
		wantFail     bool
	}{
		{"helm-operation-s828p", "Completed", false},
		{"helm-operation-s828p", "Error", false},
		{"helm-operation-s828p", "Running", true},
		{"helm-operation-s828p", "CrashLoopBackOff", true},
		{"helm-install-traefik-bj7pr", "Completed", false},
		{"helm-install-traefik-bj7pr", "Error", true},
		{"helm-delete-traefik-x1", "Error", true},
	}

	for _, tt := range tests {
		t.Run(tt.name+"/"+tt.status, func(t *testing.T) {
			failed := false
			g := gomega.NewGomega(func(string, ...int) { failed = true })
			pod := shared.Pod{NameSpace: "cattle-system", Name: tt.name, Status: tt.status}

			processPodStatus(&shared.Cluster{}, g, &pod, nil, nil)

			if failed != tt.wantFail {
				t.Fatalf("failed = %v, want %v", failed, tt.wantFail)
			}
		})
	}
}
