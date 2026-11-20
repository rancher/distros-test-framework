package qase

import (
	"testing"

	"github.com/rancher/distros-test-framework/internal/provisioning/driver"
)

func TestParseResultsStatus(t *testing.T) {
	tests := []struct {
		name     string
		statuses []string
		want     string
	}{
		{"all passed", []string{passStatus, passStatus}, passStatus},
		{"skipped does not fail run", []string{passStatus, skipStatus}, passStatus},
		{"all skipped fails run", []string{skipStatus, skipStatus}, failStatus},
		{"failed fails run", []string{passStatus, skipStatus, failStatus}, failStatus},
		{"panicked fails run", []string{passStatus, "panicked"}, failStatus},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tcs := make([]TestCase, 0, len(tt.statuses))
			for _, s := range tt.statuses {
				tcs = append(tcs, TestCase{Name: s, Status: s})
			}

			got := parseResults(&driver.Cluster{}, tcs, "", &createResultRequest{})
			if got.status != tt.want {
				t.Fatalf("status = %q, want %q", got.status, tt.want)
			}
		})
	}
}
