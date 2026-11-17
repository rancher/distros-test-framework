package qainfra

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/rancher/distros-test-framework/internal/provisioning/driver"
)

func TestNetworkEnv(t *testing.T) {
	for in, want := range map[string]string{
		"dualstack": envDualstack, "DualStack": envDualstack, "\tdualstack\n": envDualstack, "ipv6only": envIPv6,
		"": "", "airgap": "", "ipv6": "",
	} {
		if got := networkEnv(in); got != want {
			t.Errorf("networkEnv(%q) = %q, want %q", in, got, want)
		}
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
	if got, _ := getPlaybookPath("k3s", false, envDualstack); got != "k3s-playbook-dualstack.yml" {
		t.Errorf("k3s dualstack playbook = %q", got)
	}
	if got, _ := getPlaybookPath("k3s", false, envIPv6); got != "k3s-playbook-ipv6.yml" {
		t.Errorf("k3s ipv6 playbook = %q", got)
	}
	if got, _ := getAnsiblePath("k3s", false, envIPv6); got != "ansible/k3s/ipv6" {
		t.Errorf("k3s ipv6 ansible path = %q", got)
	}
	if got, _ := getPlaybookPath("k3s", false, ""); got != "k3s-playbook.yml" {
		t.Errorf("k3s default playbook changed: %q", got)
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

const networkNodesJSON = `{"type":"cluster_nodes","metadata":{"kube_api_host":"2600::10","fqdn":"x.example.com",
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
	if !strings.Contains(w.SSHArgs, `-W "[%h]:%p" '\''ubuntu@3.3.3.3'\''`) {
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
		t.Errorf("address selection wrong: %q %q %q",
			n.address(false, true), n.address(true, false), n.address(false, false))
	}
}

func TestNodeExternalIPPatch(t *testing.T) {
	if got := nodeExternalIPPatch("", false); got != "node-external-ip: {{ ansible_host }}" {
		t.Errorf("default patch changed: %q", got)
	}
	got := nodeExternalIPPatch(envDualstack, false)
	if !strings.Contains(got, "{{ ansible_host }},{{ ansible_host_ipv6 }}") {
		t.Errorf("IPv4-first dual-stack must advertise IPv4 first: %q", got)
	}
	if got := nodeExternalIPPatch(envDualstack, true); !strings.Contains(got, "{{ ansible_host_ipv6 }},{{ ansible_host }}") {
		t.Errorf("IPv6-first dual-stack must advertise IPv6 first: %q", got)
	}
}

func TestIPv6FirstCIDR(t *testing.T) {
	// The node-ip template is the Jenkins IPv6-first form: unquoted, so not valid YAML on its own.
	const nodeIPTemplate = `node-ip: {{ ansible_host_ipv6 }},{{ ansible_default_ipv4.address }}`
	cases := []struct {
		flags string
		want  bool
	}{
		{`cni: calico\nservice-cidr: 2001:cafe:42:0::/56,10.42.0.0/16`, true},
		{`service-cidr: 10.42.0.0/16,2001:cafe:42:0::/56\ncni: calico`, false},
		{`cluster-cidr: 2001:cafe:42::/56,10.42.0.0/16`, false},
		{`service-cidr: 2001:cafe:43::/112,10.43.0.0/16`, true},
		{`service-cidr: [10.43.0.0/16, "2001:cafe:43::/112"]`, false},
		{"service-cidr:\n  - 2001:cafe:43::/112\n  - 10.43.0.0/16", true},
		{"service-cidr:\n- 2001:cafe:43::/112\n- 10.43.0.0/16\ncni: calico", true},
		{`service-cidr: 2001:cafe:43::/112,10.43.0.0/16\n` + nodeIPTemplate, true},
		{nodeIPTemplate + "\nservice-cidr:\n  - 2001:cafe:43::/112", true},
		{`cni: calico`, false},
		{``, false},
	}
	for _, c := range cases {
		if got := ipv6FirstCIDR(c.flags); got != c.want {
			t.Errorf("ipv6FirstCIDR(%q) = %t, want %t", c.flags, got, c.want)
		}
	}
}

func TestBuildAnsibleArgsPerRoleFlags(t *testing.T) {
	for _, module := range []string{dualstackModule, ipv6OnlyModule, ""} {
		cfg := airgapTestConfig(t, module)
		cfg.Product = "rke2"
		cfg.Cluster.Config.ServerFlags = `cni: calico\nkubelet-arg:\n  - node-ip=0.0.0.0`
		cfg.Cluster.Config.WorkerFlags = `kubelet-arg:\n  - node-ip=0.0.0.0`
		args, err := buildAnsibleArgs(cfg, "p.yml")
		if err != nil {
			t.Fatal(err)
		}
		joined := strings.Join(args, " ")
		if module == "" {
			if !strings.Contains(joined, "rke2_additional_config") {
				t.Errorf("default mode must keep rke2_additional_config: %s", joined)
			}
			continue
		}
		if strings.Contains(joined, "rke2_additional_config") {
			t.Errorf("%s: rke2_additional_config would override worker_flags on agents", module)
		}
		for _, want := range []string{
			`server_flags="cni: calico\nkubelet-arg:\n  - node-ip=0.0.0.0"`,
			`worker_flags="kubelet-arg:\n  - node-ip=0.0.0.0"`,
		} {
			if !strings.Contains(joined, want) {
				t.Errorf("%s: want %s in args, got %s", module, want, joined)
			}
		}
	}
}

func TestBuildNetworkInventoryQuotesKeyPath(t *testing.T) {
	data, err := parseClusterNodesJSON([]byte(networkNodesJSON))
	if err != nil {
		t.Fatal(err)
	}
	out, err := buildNetworkInventory(data, envIPv6, "ubuntu", "/tmp/my key.pem")
	if err != nil {
		t.Fatal(err)
	}
	var inv networkInventory
	if err := yaml.Unmarshal([]byte(out), &inv); err != nil {
		t.Fatal(err)
	}
	if got := inv.All.Hosts["worker-0"].SSHArgs; !strings.Contains(got, `ssh -i '\''/tmp/my key.pem'\''`) {
		t.Errorf("key path with a space must stay one quoted argument: %q", got)
	}
}

func TestInjectNetworkModuleArgsLineEndings(t *testing.T) {
	for _, tf := range []string{
		"a = 1\r\n  airgap_setup = false\r\nb = 2\r\n", "a = 1\nproxy_setup = false",
	} {
		got := injectNetworkModuleArgs(tf, envDualstack)
		if strings.Contains(got, "airgap_setup") || strings.Contains(got, "proxy_setup") {
			t.Errorf("not stripped from %q: %q", tf, got)
		}
	}
}

// k3sTemplateTail mirrors the node-external-ip block in qa-infra's roles/k3s_install/templates/k3s-config.yaml.j2.
const k3sTemplateTail = `{% if public_ip is defined and public_ip | trim != '' and (k3s_node_external_ip | default(true)) %}
node-external-ip:
  - "{{ public_ip }}"
{% endif %}
tls-san:
  - "{{ public_ip | default('127.0.0.1') }}"
`

func TestPatchK3sExternalIP(t *testing.T) {
	cases := []struct {
		env       string
		ipv6First bool
		want      string
	}{
		{envDualstack, false, `node-external-ip: "{{ ansible_host }},{{ ansible_host_ipv6 }}"`},
		{envDualstack, true, `node-external-ip: "{{ ansible_host_ipv6 }},{{ ansible_host }}"`},
		{envIPv6, false, `node-external-ip: "{{ ansible_host_ipv6 }}"`},
	}
	for _, c := range cases {
		got, ok := patchK3sExternalIP(k3sTemplateTail, c.env, c.ipv6First)
		if !ok || !strings.Contains(got, c.want) || strings.Contains(got, `  - "{{ public_ip }}"`+"\n{% endif %}") {
			t.Errorf("%s ipv6First=%t: ok=%t\n%s", c.env, c.ipv6First, ok, got)
		}
		// tls-san still uses public_ip; only the node-external-ip list is rewritten.
		if !strings.Contains(got, `tls-san:`+"\n"+`  - "{{ public_ip | default('127.0.0.1') }}"`) {
			t.Errorf("%s: tls-san must be untouched:\n%s", c.env, got)
		}
	}
	if got, ok := patchK3sExternalIP(k3sTemplateTail, "", false); ok || got != k3sTemplateTail {
		t.Error("default k3s runs must keep the template unchanged")
	}
	if _, ok := patchK3sExternalIP("node-external-ip: x\n", envDualstack, false); ok {
		t.Error("a missing anchor must be reported, not silently patched")
	}
}

func TestPatchK3sConfigTemplate(t *testing.T) {
	write := func(t *testing.T) (*driver.InfraConfig, string) {
		t.Helper()
		cfg := &driver.InfraConfig{
			Product: "k3s", Module: dualstackModule,
			Cluster:          &driver.Cluster{},
			InfraProvisioner: &driver.InfraProvisionerConfig{TempDir: t.TempDir()},
		}
		cfg.Cluster.Config.ServerFlags = "service-cidr: 2001:cafe:43::/112,10.43.0.0/16"
		path := filepath.Join(cfg.InfraProvisioner.TempDir,
			"ansible", "roles", "k3s_install", "templates", "k3s-config.yaml.j2")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(k3sTemplateTail), 0o644); err != nil {
			t.Fatal(err)
		}

		return cfg, path
	}

	cfg, path := write(t)
	if err := patchK3sConfigTemplate(cfg); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if !strings.Contains(string(got), `node-external-ip: "{{ ansible_host_ipv6 }},{{ ansible_host }}"`) {
		t.Errorf("IPv6-first k3s dualstack template not patched:\n%s", got)
	}

	for _, c := range []struct{ product, module string }{{"rke2", dualstackModule}, {"k3s", ""}} {
		cfg, path := write(t)
		cfg.Product, cfg.Module = c.product, c.module
		if err := patchK3sConfigTemplate(cfg); err != nil {
			t.Fatal(err)
		}
		if got, _ := os.ReadFile(path); string(got) != k3sTemplateTail {
			t.Errorf("%s/%q must not touch the k3s template", c.product, c.module)
		}
	}

	cfg, path = write(t)
	_ = os.Remove(path)
	if err := patchK3sConfigTemplate(cfg); err == nil {
		t.Error("a missing k3s template on a dualstack run must fail")
	}
}
