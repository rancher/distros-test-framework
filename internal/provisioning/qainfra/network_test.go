package qainfra

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestNetworkEnv(t *testing.T) {
	for in, want := range map[string]string{
		"dualstack": envDualstack, " DualStack ": envDualstack, "ipv6only": envIPv6,
		"": "", "airgap": "", "ipv6": "",
	} {
		if got := networkEnv(in); got != want {
			t.Errorf("networkEnv(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestValidateNetworkEnv(t *testing.T) {
	if err := validateNetworkEnv("rke2", "aws", envIPv6); err != nil {
		t.Errorf("rke2/aws ipv6 must be accepted: %v", err)
	}
	if err := validateNetworkEnv("k3s", "aws", ""); err != nil {
		t.Errorf("non-network modules must be untouched: %v", err)
	}
	if err := validateNetworkEnv("k3s", "aws", envDualstack); err == nil {
		t.Error("k3s dualstack must be rejected: qa-infra has no k3s playbook")
	}
	if err := validateNetworkEnv("rke2", "vsphere", envDualstack); err == nil {
		t.Error("non-aws provider must be rejected: the module only exists under tofu/aws")
	}
}

func TestPlaybookAndAnsiblePathForNetworkEnv(t *testing.T) {
	if got, _ := getPlaybookPath("rke2", false, envIPv6); got != "rke2-playbook-ipv6.yml" {
		t.Errorf("ipv6 playbook = %q", got)
	}
	if got, _ := getPlaybookPath("rke2", false, ""); got != "rke2-playbook.yml" {
		t.Errorf("default playbook changed: %q", got)
	}
	if got, _ := getAnsiblePath("rke2", false, envDualstack); got != "ansible/rke2/dualstack" {
		t.Errorf("dualstack ansible path = %q", got)
	}
	if got, _ := getAnsiblePath("rke2", false, ""); got != "ansible/rke2/default" {
		t.Errorf("default ansible path changed: %q", got)
	}
}

// The real template must lose the cluster_nodes-only inputs, which the dualstack/ipv6 modules reject.
func TestInjectNetworkModuleArgsOnTemplate(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "infrastructure", "qainfra", "main.tf"))
	if err != nil {
		t.Fatal(err)
	}
	tf := string(raw)

	if got := injectNetworkModuleArgs(tf, ""); got != tf {
		t.Fatal("connected runs must keep main.tf unchanged")
	}
	for env, publicIP := range map[string]string{envDualstack: "true", envIPv6: "false"} {
		got := injectNetworkModuleArgs(tf, env)
		for _, gone := range []string{"airgap_setup", "proxy_setup", airgapModuleMarker} {
			if strings.Contains(got, gone) {
				t.Errorf("%s main.tf still contains %q", env, gone)
			}
		}
		for _, want := range []string{
			"enable_ipv6         = true", "enable_public_ip    = " + publicIP,
			"no_of_bastion_nodes = 1", "var.aws_bastion_subnet", "var.kube_api_host_ipv6",
		} {
			if !strings.Contains(got, want) {
				t.Errorf("%s main.tf missing %q", env, want)
			}
		}
	}
}

const networkNodesJSON = `{"type":"cluster_nodes","metadata":{"kube_api_host":"2600::10","fqdn":"x.qa.rancher.space",
"ssh_user":"ubuntu","bastion_ip":"3.3.3.3","bastion_dns":"ec2-3.compute"},
"nodes":[{"name":"master","roles":["etcd","cp","worker"],"public_ip":"1.1.1.1","private_ip":"10.0.0.1","ipv6":"2600::10"},
{"name":"worker-0","roles":["worker"],"public_ip":"","private_ip":"10.0.0.2","ipv6":"2600::11"}]}`

func TestParseClusterNodesJSONMetadataBastion(t *testing.T) {
	data, err := parseClusterNodesJSON([]byte(networkNodesJSON))
	if err != nil {
		t.Fatal(err)
	}
	if data.Bastion == nil || data.Bastion.PublicIP != "3.3.3.3" || data.Bastion.PublicDNS != "ec2-3.compute" {
		t.Fatalf("metadata bastion_ip not promoted to Bastion: %+v", data.Bastion)
	}
	if data.Nodes[1].IPv6 != "2600::11" {
		t.Errorf("node ipv6 not parsed: %+v", data.Nodes[1])
	}
}

type networkInventory struct {
	All struct {
		Vars  map[string]string `yaml:"vars"`
		Hosts map[string]struct {
			AnsibleHost     string `yaml:"ansible_host"`
			AnsibleHostIPv6 string `yaml:"ansible_host_ipv6"`
			SSHArgs         string `yaml:"ansible_ssh_common_args"`
			RKE2NodeRole    string `yaml:"rke2_node_role"`
		} `yaml:"hosts"`
		Children map[string]struct {
			Hosts map[string]any `yaml:"hosts"`
		} `yaml:"children"`
	} `yaml:"all"`
}

func TestBuildNetworkInventoryIPv6(t *testing.T) {
	data, err := parseClusterNodesJSON([]byte(networkNodesJSON))
	if err != nil {
		t.Fatal(err)
	}
	out, err := buildNetworkInventory(data, envIPv6, "ubuntu", "/k.pem")
	if err != nil {
		t.Fatal(err)
	}
	var inv networkInventory
	if err := yaml.Unmarshal([]byte(out), &inv); err != nil {
		t.Fatalf("inventory is not valid YAML: %v\n%s", err, out)
	}

	w := inv.All.Hosts["worker-0"]
	if w.AnsibleHost != "2600::11" || w.AnsibleHostIPv6 != "2600::11" || w.RKE2NodeRole != "agent" {
		t.Errorf("ipv6 worker entry wrong: %+v", w)
	}
	if !strings.Contains(w.SSHArgs, "-W [%h]:%p ubuntu@3.3.3.3") {
		t.Errorf("ipv6 nodes must go through the bastion with a bracketed %%h: %q", w.SSHArgs)
	}
	if b := inv.All.Hosts["bastion-0"]; b.AnsibleHost != "3.3.3.3" || strings.Contains(b.SSHArgs, "ProxyCommand") {
		t.Errorf("bastion must be reached directly: %+v", b)
	}
	if _, ok := inv.All.Children["bastion"].Hosts["bastion-0"]; !ok {
		t.Error("playbook targets the 'bastion' group; it is missing")
	}
	if _, ok := inv.All.Children["master"].Hosts["master"]; !ok || inv.All.Vars["kube_api_host"] != "2600::10" {
		t.Errorf("master group / kube_api_host wrong: %+v", inv.All.Vars)
	}
}

func TestBuildNetworkInventoryDualstack(t *testing.T) {
	data, err := parseClusterNodesJSON([]byte(networkNodesJSON))
	if err != nil {
		t.Fatal(err)
	}
	data.Nodes[1].PublicIP = "1.1.1.2"
	out, err := buildNetworkInventory(data, envDualstack, "ubuntu", "/k.pem")
	if err != nil {
		t.Fatal(err)
	}
	var inv networkInventory
	if err := yaml.Unmarshal([]byte(out), &inv); err != nil {
		t.Fatalf("inventory is not valid YAML: %v\n%s", err, out)
	}
	m := inv.All.Hosts["master"]
	if m.AnsibleHost != "1.1.1.1" || m.AnsibleHostIPv6 != "2600::10" || strings.Contains(m.SSHArgs, "ProxyCommand") {
		t.Errorf("dual-stack nodes must be reached directly over IPv4: %+v", m)
	}
}

func TestBuildNetworkInventoryRejectsMissingAddresses(t *testing.T) {
	data, err := parseClusterNodesJSON([]byte(networkNodesJSON))
	if err != nil {
		t.Fatal(err)
	}
	// worker-0 has no public IPv4, so it cannot be a dual-stack node.
	if _, err := buildNetworkInventory(data, envDualstack, "ubuntu", "/k.pem"); err == nil {
		t.Error("dual-stack node without public_ip must be rejected")
	}
	data.Bastion = nil
	if _, err := buildNetworkInventory(data, envIPv6, "ubuntu", "/k.pem"); err == nil {
		t.Error("missing bastion must be rejected")
	}
}

func TestNodeAddressIPv6Only(t *testing.T) {
	n := infraNode{publicIP: "1.1.1.1", privateIP: "10.0.0.1", ipv6: "2600::1"}
	if n.address(false, true) != "2600::1" || n.address(true, false) != "10.0.0.1" || n.address(false, false) != "1.1.1.1" {
		t.Errorf("address selection wrong: %q %q %q", n.address(false, true), n.address(true, false), n.address(false, false))
	}
}

func TestNodeExternalIPPatch(t *testing.T) {
	if got := nodeExternalIPPatch(""); got != "node-external-ip: {{ ansible_host }}" {
		t.Errorf("default patch changed: %q", got)
	}
	if got := nodeExternalIPPatch(envDualstack); !strings.Contains(got, "{{ ansible_host }},{{ ansible_host_ipv6 }}") {
		t.Errorf("dual-stack must advertise both families: %q", got)
	}
}
