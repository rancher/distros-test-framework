package testcase

import (
	"testing"

	"github.com/rancher/distros-test-framework/internal/provisioning/driver"
)

func TestCurlNodeIP(t *testing.T) {
	if got := curlNodeIP(&driver.Cluster{ServerIPs: []string{"s"}, AgentIPs: []string{"a"}}); got != "a" {
		t.Errorf("want agent, got %q", got)
	}
	if got := curlNodeIP(&driver.Cluster{ServerIPs: []string{"s"}}); got != "s" {
		t.Errorf("want server fallback, got %q", got)
	}
}
