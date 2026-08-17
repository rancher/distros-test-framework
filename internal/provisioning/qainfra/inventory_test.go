package qainfra

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func nodesJSON(sshUser string, nodes ...clusterNode) *clusterNodesJSON {
	data := &clusterNodesJSON{Nodes: nodes}
	data.Metadata.SSHUser = sshUser
	data.Metadata.KubeAPIHost = "10.0.0.1"
	data.Metadata.FQDN = "dsf-x.qa.rancher.space"

	return data
}

func TestAssignNodeGroupsNormalCluster(t *testing.T) {
	data := nodesJSON("ec2-user",
		clusterNode{Name: "master", Roles: []string{"etcd", "cp", "worker"}, PublicIP: "1.1.1.1"},
		clusterNode{Name: "etcd-cp-worker-1", Roles: []string{"etcd", "cp", "worker"}, PublicIP: "1.1.1.2"},
		clusterNode{Name: "etcd-cp-worker-2", Roles: []string{"etcd", "cp", "worker"}, PublicIP: "1.1.1.3"},
		clusterNode{Name: "worker-0", Roles: []string{"worker"}, PublicIP: "1.1.1.4"},
	)

	got := assignNodeGroups(data, "rke2")
	want := map[string]string{
		"master": "master", "etcd-cp-worker-1": "servers",
		"etcd-cp-worker-2": "servers", "worker-0": "workers",
	}
	assertGroups(t, got, want, data)
}

func TestAssignNodeGroupsSplitRoles(t *testing.T) {
	data := nodesJSON("ec2-user",
		clusterNode{Name: "etcd-only-0", Roles: []string{"etcd"}, PublicIP: "1.1.1.1"},
		clusterNode{Name: "cp-only-0", Roles: []string{"cp"}, PublicIP: "1.1.1.2"},
		clusterNode{Name: "cp-worker-0", Roles: []string{"cp", "worker"}, PublicIP: "1.1.1.3"},
		clusterNode{Name: "worker-0", Roles: []string{"worker"}, PublicIP: "1.1.1.4"},
	)

	got := assignNodeGroups(data, "rke2")
	want := map[string]string{
		// etcd-only wins master; every other etcd/cp shape must land in servers,
		// or Ansible would skip nodes Tofu provisioned.
		"etcd-only-0": "master", "cp-only-0": "servers",
		"cp-worker-0": "servers", "worker-0": "workers",
	}
	assertGroups(t, got, want, data)
}

func TestAssignNodeGroupsExternalDBNoEtcd(t *testing.T) {
	data := nodesJSON("ec2-user",
		clusterNode{Name: "cp-0", Roles: []string{"cp"}, PublicIP: "1.1.1.1"},
		clusterNode{Name: "cp-1", Roles: []string{"cp"}, PublicIP: "1.1.1.2"},
		clusterNode{Name: "worker-0", Roles: []string{"worker"}, PublicIP: "1.1.1.3"},
	)

	got := assignNodeGroups(data, "k3s")
	want := map[string]string{"cp-0": "master", "cp-1": "servers", "worker-0": "workers"}
	assertGroups(t, got, want, data)
}

func assertGroups(t *testing.T, got, want map[string]string, data *clusterNodesJSON) {
	t.Helper()
	if len(got) != len(data.Nodes) {
		t.Fatalf("every node must land in exactly one group: got %d assignments for %d nodes (%v)",
			len(got), len(data.Nodes), got)
	}
	for node, group := range want {
		if got[node] != group {
			t.Errorf("node %s assigned to %q, want %q", node, got[node], group)
		}
	}
}

//nolint:funlen // table-driven test
func TestBuildStaticInventoryYAML(t *testing.T) {
	data := nodesJSON("ec2-user",
		clusterNode{Name: "etcd-only-0", Roles: []string{"etcd"}, PublicIP: "1.1.1.1", PrivateIP: "10.0.0.11"},
		clusterNode{Name: "cp-worker-0", Roles: []string{"cp", "worker"}, PublicIP: "1.1.1.2", PrivateIP: "10.0.0.12"},
		clusterNode{Name: "worker-0", Roles: []string{"worker"}, PublicIP: "1.1.1.3", PrivateIP: "10.0.0.13"},
	)

	out := buildStaticInventory(data, "rke2")

	var inv struct {
		All struct {
			Vars struct {
				AnsibleUser string `yaml:"ansible_user"`
				KubeAPIHost string `yaml:"kube_api_host"`
			} `yaml:"vars"`
			Hosts map[string]struct {
				AnsibleHost  string   `yaml:"ansible_host"`
				NodeRoles    []string `yaml:"node_roles"`
				RKE2NodeRole string   `yaml:"rke2_node_role"`
			} `yaml:"hosts"`
			Children map[string]struct {
				Hosts map[string]struct {
					AnsibleHost string `yaml:"ansible_host"`
				} `yaml:"hosts"`
			} `yaml:"children"`
		} `yaml:"all"`
	}
	if err := yaml.Unmarshal([]byte(out), &inv); err != nil {
		t.Fatalf("inventory is not valid YAML: %v\n%s", err, out)
	}

	if inv.All.Vars.AnsibleUser != "ec2-user" {
		t.Errorf("ansible_user = %q", inv.All.Vars.AnsibleUser)
	}
	if inv.All.Vars.KubeAPIHost != "10.0.0.1" {
		t.Errorf("kube_api_host = %q", inv.All.Vars.KubeAPIHost)
	}

	if h := inv.All.Hosts["cp-worker-0"]; h.AnsibleHost != "1.1.1.2" ||
		len(h.NodeRoles) != 2 || h.NodeRoles[0] != "cp" || h.RKE2NodeRole != "server" {
		t.Errorf("cp-worker-0 host entry wrong: %+v", h)
	}
	if h := inv.All.Hosts["worker-0"]; h.RKE2NodeRole != "agent" {
		t.Errorf("worker-0 rke2_node_role = %q, want agent", h.RKE2NodeRole)
	}

	groupOf := map[string]string{}
	for group, members := range inv.All.Children {
		for name, h := range members.Hosts {
			if prev, dup := groupOf[name]; dup {
				t.Errorf("node %s in two groups: %s and %s", name, prev, group)
			}
			groupOf[name] = group
			if want := inv.All.Hosts[name].AnsibleHost; h.AnsibleHost != want {
				t.Errorf("children %s/%s ansible_host = %q, want %q", group, name, h.AnsibleHost, want)
			}
		}
	}
	want := map[string]string{"etcd-only-0": "master", "cp-worker-0": "servers", "worker-0": "workers"}
	for node, group := range want {
		if groupOf[node] != group {
			t.Errorf("node %s in group %q, want %q", node, groupOf[node], group)
		}
	}
}
