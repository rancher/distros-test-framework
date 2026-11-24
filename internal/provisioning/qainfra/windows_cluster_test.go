package qainfra

import (
	"slices"
	"testing"
)

const mixedOSClusterFixture = `{
  "type": "cluster_nodes",
  "metadata": {"schema_version": 1, "kube_api_host": "1.1.1.1", "ssh_user": "ubuntu"},
  "nodes": [
    {"name": "master", "roles": ["etcd","cp","worker"], "public_ip": "1.1.1.1", "private_ip": "10.0.0.1", "os": "linux"},
    {"name": "worker-0", "roles": ["worker"], "public_ip": "1.1.1.2", "private_ip": "10.0.0.2", "os": "linux"},
    {"name": "windows-worker-0", "roles": ["worker"], "public_ip": "1.1.1.3", "private_ip": "10.0.0.3", "os": "windows"},
    {"name": "windows-worker-1", "roles": ["worker"], "public_ip": "1.1.1.4", "private_ip": "10.0.0.4"}
  ]
}`

func TestBuildClusterConfigClassifiesWindowsAgents(t *testing.T) {
	stubTofuOutput(t, mixedOSClusterFixture)
	setNodeEnvs(t, nil)
	cfg := airgapTestConfig(t, "")
	cfg.InfraProvisioner.TFNodeSource = cfg.InfraProvisioner.RunDir

	if err := buildClusterConfig(cfg); err != nil {
		t.Fatal(err)
	}
	c := cfg.Cluster
	if !slices.Equal(c.ServerIPs, []string{"1.1.1.1"}) || !slices.Equal(c.AgentIPs, []string{"1.1.1.2"}) {
		t.Errorf("linux nodes misclassified: servers=%v agents=%v", c.ServerIPs, c.AgentIPs)
	}
	// windows-worker-1 has no os field, so the name fallback must still mark it Windows.
	if !slices.Equal(c.WinAgentIPs, []string{"1.1.1.3", "1.1.1.4"}) {
		t.Errorf("windows agents = %v, want both Windows public IPs", c.WinAgentIPs)
	}
	if c.NumServers != 1 || c.NumAgents != 1 || c.NumWinAgents != 2 {
		t.Errorf("counts servers/agents/win = %d/%d/%d, want 1/1/2", c.NumServers, c.NumAgents, c.NumWinAgents)
	}
}
