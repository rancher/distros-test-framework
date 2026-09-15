package qainfra

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rancher/distros-test-framework/internal/pkg/customflag"
	"github.com/rancher/distros-test-framework/internal/provisioning/driver"
)

const clusterNodesV2Fixture = `{
  "type": "cluster_nodes",
  "metadata": {"schema_version": 2, "kube_api_host": "10.0.1.10", "fqdn": "dsf-x.qa.rancher.space",
               "ssh_user": "ubuntu", "airgap": true, "arch": "amd64", "run_id": "run-a", "qa_infra_sha": "abc"},
  "nodes": [
    {"name": "master", "roles": ["etcd","cp","worker"], "public_ip": "", "private_ip": "10.0.1.10", "instance_id": "i-1"},
    {"name": "worker-0", "roles": ["worker"], "public_ip": "", "private_ip": "10.0.1.11", "instance_id": "i-2"}
  ],
  "bastion": {"public_ip": "3.3.3.3", "public_dns": "ec2-3-3-3-3.compute.amazonaws.com",
              "private_ip": "10.0.1.5", "instance_id": "i-0"}
}`

func airgapTestConfig(t *testing.T, module string) *driver.InfraConfig {
	t.Helper()
	root := t.TempDir()

	return &driver.InfraConfig{
		Product: "k3s", Module: module, InstallVersion: "v1.36.0+k3s1", CNI: "",
		Cluster: &driver.Cluster{
			Config: driver.Config{Arch: "arm", ServerFlags: "", WorkerFlags: ""},
			SSH:    driver.SSHConfig{User: "ubuntu", PrivKeyPath: "/tmp/k.pem", KeyName: "jenkins-key"},
		},
		InfraProvisioner: &driver.InfraProvisionerConfig{
			RunID: "run-a", RunDir: root, QAInfraSHA: strings.Repeat("a", 40),
			AirgapSetup:    module == airgapModule,
			KubeconfigPath: filepath.Join(root, "kubeconfig.yaml"),
			Inventory:      driver.Inventory{Path: filepath.Join(root, "inventory.yml")},
			Ansible:        driver.Ansible{Dir: root},
			Terraform:      driver.Terraform{TFVarsPath: filepath.Join(root, "vars.tfvars")},
		},
	}
}

func TestNormalizeArch(t *testing.T) {
	for in, want := range map[string]string{"": "amd64", "x86_64": "amd64", "AMD64": "amd64",
		"arm": "arm64", "aarch64": "arm64", "arm64": "arm64", "riscv": "riscv"} {
		if got := normalizeArch(in); got != want {
			t.Errorf("normalizeArch(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestInjectAirgapModuleArgs(t *testing.T) {
	tf := "module \"cluster_nodes\" {\n  nodes = var.nodes\n  " + airgapModuleMarker + "\n}\n"
	connected := injectAirgapModuleArgs(tf, false)
	if strings.Contains(connected, "bastion") || strings.Contains(connected, airgapModuleMarker) {
		t.Fatalf("connected runs must pass no airgap args (old refs reject them):\n%s", connected)
	}
	airgap := injectAirgapModuleArgs(tf, true)
	wants := []string{"enabled       = var.bastion_enabled", "run_id       = var.run_id", "arch         = var.arch"}
	for _, want := range wants {
		if !strings.Contains(airgap, want) {
			t.Errorf("airgap module args missing %q:\n%s", want, airgap)
		}
	}
}

func TestWriteAirgapTFVars(t *testing.T) {
	cfg := airgapTestConfig(t, airgapModule)
	seed := []byte("aws_region = \"us-east-2\"\n")
	if err := os.WriteFile(cfg.InfraProvisioner.Terraform.TFVarsPath, seed, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(bastionTypeEnv, "t4g.large")

	if err := writeAirgapTFVars(cfg); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, cfg.InfraProvisioner.Terraform.TFVarsPath)
	for _, want := range []string{
		"airgap_setup = true", "bastion_enabled = true", `bastion_instance_type = "t4g.large"`,
		`run_id = "run-a"`, `qa_infra_sha = "` + strings.Repeat("a", 40) + `"`, `arch = "arm64"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("tfvars missing %q:\n%s", want, got)
		}
	}
}

func TestBuildAirgapInventoryProxiesThroughBastion(t *testing.T) {
	var data clusterNodesJSON
	if err := json.Unmarshal([]byte(clusterNodesV2Fixture), &data); err != nil {
		t.Fatal(err)
	}
	inv, err := buildAirgapInventory(&data, "ubuntu", "/tmp/k.pem", "jenkins-key")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`bastion_host: "3.3.3.3"`, `bastion_public_dns: "ec2-3-3-3-3.compute.amazonaws.com"`,
		`ssh_key_name: "jenkins-key"`, "    bastion-0:\n      ansible_host: \"3.3.3.3\"",
		`ansible_host: "10.0.1.10"`, `ansible_host: "10.0.1.11"`,
		`-W %h:%p ubuntu@3.3.3.3`, `node_type: "master"`, `node_type: "agent"`,
		"    bastion:\n      hosts:\n        bastion-0:",
		"    master:\n      hosts:\n        master:",
		"    workers:\n      hosts:\n        worker-0:",
	} {
		if !strings.Contains(inv, want) {
			t.Errorf("inventory missing %q:\n%s", want, inv)
		}
	}
	if strings.Count(inv, "ProxyCommand") != 2 {
		t.Fatalf("every private node (2) needs a ProxyCommand, bastion none:\n%s", inv)
	}

	data.Bastion = nil
	if _, err := buildAirgapInventory(&data, "ubuntu", "/tmp/k.pem", "k"); err == nil {
		t.Fatal("inventory without a bastion must fail")
	}
}

// buildClusterConfig must use private IPs and record the bastion for airgap runs.
func TestBuildClusterConfigAirgapUsesPrivateIPsAndBastion(t *testing.T) {
	stubTofuOutput(t, clusterNodesV2Fixture)
	t.Setenv("split_roles", "")
	cfg := airgapTestConfig(t, airgapModule)
	cfg.InfraProvisioner.TFNodeSource = cfg.InfraProvisioner.RunDir

	if err := buildClusterConfig(cfg); err != nil {
		t.Fatal(err)
	}
	c := cfg.Cluster
	if len(c.ServerIPs) != 1 || c.ServerIPs[0] != "10.0.1.10" || len(c.AgentIPs) != 1 || c.AgentIPs[0] != "10.0.1.11" {
		t.Fatalf("airgap must address nodes by private IP: servers=%v agents=%v", c.ServerIPs, c.AgentIPs)
	}
	if c.NumBastion != 1 || c.Bastion.PublicIPv4Addr != "3.3.3.3" ||
		c.Bastion.PublicDNS == "" || c.Bastion.PrivateIP != "10.0.1.5" {
		t.Fatalf("bastion not recorded: %+v", c.Bastion)
	}
}

func TestBuildClusterConfigAirgapWithoutBastionFails(t *testing.T) {
	stubTofuOutput(t, strings.Replace(clusterNodesV2Fixture, `"bastion": {`, `"unused": {`, 1))
	t.Setenv("split_roles", "")
	cfg := airgapTestConfig(t, airgapModule)
	cfg.InfraProvisioner.TFNodeSource = cfg.InfraProvisioner.RunDir
	if err := buildClusterConfig(cfg); err == nil || !strings.Contains(err.Error(), "bastion") {
		t.Fatalf("airgap without bastion must fail closed, got %v", err)
	}
}

func TestBuildClusterConfigConnectedKeepsPublicIPs(t *testing.T) {
	v1 := `{"type":"cluster_nodes","metadata":{"kube_api_host":"1.1.1.1","fqdn":"f","ssh_user":"u"},
	  "nodes":[{"name":"master","roles":["etcd","cp","worker"],"public_ip":"1.1.1.1","private_ip":"10.0.0.1"}]}`
	stubTofuOutput(t, v1)
	t.Setenv("split_roles", "")
	cfg := airgapTestConfig(t, "")
	cfg.InfraProvisioner.TFNodeSource = cfg.InfraProvisioner.RunDir
	if err := buildClusterConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Cluster.ServerIPs[0] != "1.1.1.1" || cfg.Cluster.NumBastion != 0 {
		t.Fatalf("connected v1 payload changed behavior: %+v", cfg.Cluster)
	}
}

func TestFetchClusterNodesJSONRejectsNewerSchema(t *testing.T) {
	stubTofuOutput(t, `{"type":"cluster_nodes","metadata":{"schema_version":3},"nodes":[]}`)
	if _, err := fetchClusterNodesJSON(t.TempDir()); err == nil || !strings.Contains(err.Error(), "schema_version 3") {
		t.Fatalf("unknown schema must be refused, got %v", err)
	}
}

func TestAirgapAnsibleArgsAndSecretsFile(t *testing.T) {
	cfg := airgapTestConfig(t, airgapModule)
	saved := customflag.ServiceFlag
	t.Cleanup(func() { customflag.ServiceFlag = saved })
	t.Setenv(testTagEnv, "")
	t.Setenv(testArgsEnv, "-tags=privateregistry -destroy true")
	customflag.ServiceFlag.AirgapFlag.ImageRegistryUrl = "https://prime.ribs.rancher.io"
	customflag.ServiceFlag.AirgapFlag.TarballType = ""
	customflag.ServiceFlag.AirgapFlag.RegistryUsername = "testuser"
	customflag.ServiceFlag.AirgapFlag.RegistryPassword = "s3cret-value"

	args, err := airgapAnsibleArgs(cfg, airgapPlaybook)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	for _, want := range []string{
		airgapPlaybook, "product=k3s", "kubernetes_version=v1.36.0+k3s1", "arch=arm64",
		"airgap_method=private_registry", "tarball_type=tar.zst",
		"image_registry_url=https://prime.ribs.rancher.io", "ssh_key_name=jenkins-key",
		"airgap_facts_file=" + filepath.Join(cfg.InfraProvisioner.Ansible.Dir, airgapFactsName),
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("args missing %q: %s", want, joined)
		}
	}
	if strings.Contains(joined, "s3cret-value") || strings.Contains(joined, "testuser") {
		t.Fatalf("credentials must never be on the ansible command line: %s", joined)
	}
	secrets := filepath.Join(cfg.InfraProvisioner.RunDir, airgapSecretsName)
	if !strings.Contains(joined, "@"+secrets) {
		t.Fatalf("secrets file not referenced: %s", joined)
	}
	info, err := os.Stat(secrets)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("secrets file must be 0600: %v %v", err, info)
	}

	// tarball needs no secrets file (TEST_TAG wins over TEST_ARGS).
	t.Setenv(testTagEnv, "tarball")
	args, err = airgapAnsibleArgs(cfg, airgapPlaybook)
	if err != nil || strings.Contains(strings.Join(args, " "), "@") {
		t.Fatalf("tarball must not reference a secrets file: %v %v", err, args)
	}

	// unknown tag fails before provisioning.
	t.Setenv(testTagEnv, "")
	t.Setenv(testArgsEnv, "-destroy true")
	if _, err := airgapAnsibleArgs(cfg, airgapPlaybook); err == nil {
		t.Fatal("missing tag must be rejected")
	}
}

func TestAirgapScenarioDefaultsRegistryCredentials(t *testing.T) {
	cfg := airgapTestConfig(t, airgapModule)
	saved := customflag.ServiceFlag
	t.Cleanup(func() { customflag.ServiceFlag = saved })
	t.Setenv(testTagEnv, "privateregistry")
	customflag.ServiceFlag.AirgapFlag.RegistryUsername = ""
	customflag.ServiceFlag.AirgapFlag.RegistryPassword = ""

	if _, err := airgapAnsibleArgs(cfg, airgapPlaybook); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(cfg.InfraProvisioner.RunDir, airgapSecretsName))
	if err != nil || !strings.Contains(string(data), customflag.DefaultRegistryUsername) {
		t.Fatalf("secrets file must carry the default username when nothing is set: %v %s", err, data)
	}
	pw := customflag.ServiceFlag.AirgapFlag.RegistryPassword
	if len(pw) < 20 || pw == customflag.DefaultRegistryPassword || !strings.Contains(string(data), pw) {
		t.Fatalf("a random per-run password must be generated and written: %q", pw)
	}
}

func TestTestTagParsing(t *testing.T) {
	for _, tc := range []struct{ tag, args, want string }{
		{"tarball", "-tags=privateregistry", "tarball"},
		{"", "-tags=systemdefaultregistry -destroy true", "systemdefaultregistry"},
		{"", "-timeout=60m -tags=tarball,other", "tarball"},
		{"", "-destroy true", ""},
	} {
		t.Setenv(testTagEnv, tc.tag)
		t.Setenv(testArgsEnv, tc.args)
		if got := testTag(); got != tc.want {
			t.Errorf("TEST_TAG=%q TEST_ARGS=%q: got %q want %q", tc.tag, tc.args, got, tc.want)
		}
	}
}

func TestCollectAirgapFacts(t *testing.T) {
	cfg := airgapTestConfig(t, airgapModule)
	saved := customflag.ServiceFlag
	t.Cleanup(func() { customflag.ServiceFlag = saved })
	t.Setenv(testTagEnv, "systemdefaultregistry")

	facts := `{"schema_version":2,"registry_mode":"system_default","registry_host":"ec2-x.compute.amazonaws.com",
	  "registry_port":443,"registry_ca_path_nodes":"/etc/rancher/k3s/airgap-registry-ca.crt",
	  "artifacts_dir":"/opt/dtf-airgap/artifacts/k3s-x","artifact_origin":"community","release_url":"https://github.com/x",
	  "bastion_kubeconfig_path":"/tmp/k3s_kubeconf.yaml","kubectl_path":"/usr/local/bin/kubectl","node_count":2}`
	if err := os.WriteFile(airgapFactsPath(cfg), []byte(facts), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := collectAirgapFacts(cfg); err != nil {
		t.Fatal(err)
	}
	a := cfg.Cluster.Airgap
	if !a.Enabled || a.Method != "system_default_registry" || a.RegistryHost != "ec2-x.compute.amazonaws.com" ||
		a.RegistryPort != 443 || a.BastionKubeconfigPath != "/tmp/k3s_kubeconf.yaml" ||
		a.KubectlPath != "/usr/local/bin/kubectl" {
		t.Fatalf("facts not applied: %+v", a)
	}

	if err := os.WriteFile(airgapFactsPath(cfg), []byte(`{"schema_version":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := collectAirgapFacts(cfg); err == nil {
		t.Fatal("facts with another schema must be rejected")
	}

	connected := airgapTestConfig(t, "")
	if err := collectAirgapFacts(connected); err != nil {
		t.Fatalf("connected runs must skip facts: %v", err)
	}
}

// stubTofuOutput fakes `tofu output -raw cluster_nodes_json`.
func stubTofuOutput(t *testing.T, payload string) {
	t.Helper()
	bin := t.TempDir()
	script := "#!/bin/sh\ncat <<'EOF'\n" + payload + "\nEOF\n"
	if err := os.WriteFile(filepath.Join(bin, "tofu"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestCNIFromFlags(t *testing.T) {
	for in, want := range map[string]string{
		`cni: multus,canal`:                                     "multus,canal",
		`cni: multus, canal`:                                    "multus,canal",
		`cni: "canal"`:                                          "canal",
		`profile: cis\ncni: 'cilium'`:                           "cilium",
		`cni: [multus, canal]`:                                  "multus,canal",
		`write-kubeconfig-mode: 644\nselinux: true`:             "",
		`profile: cis\ncni: cilium\nsystem-default-registry: x`: "cilium",
		"write-kubeconfig-mode: 644\ncni: Calico":               "calico",
		`selinux: true`:                                         "",
		``:                                                      "",
	} {
		if got := cniFromFlags(in); got != want {
			t.Errorf("cniFromFlags(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEffectiveCNI(t *testing.T) {
	for _, tc := range []struct {
		explicit, flags, want string
		wantErr               bool
	}{
		{"", "cni: canal", "canal", false},
		{"cilium", "", "cilium", false},
		{"Multus, Canal", "cni: multus,canal", "multus,canal", false},
		{"cilium", "cni: canal", "", true},
		{"", "", "", false},
	} {
		got, err := effectiveCNI(tc.explicit, tc.flags)
		if (err != nil) != tc.wantErr || got != tc.want {
			t.Errorf("effectiveCNI(%q, %q) = %q, %v; want %q, err=%v", tc.explicit, tc.flags, got, err, tc.want, tc.wantErr)
		}
	}
}

func TestRandomPassword(t *testing.T) {
	a, err := randomPassword(24)
	b, err2 := randomPassword(24)
	if err != nil || err2 != nil || len(a) != 24 || a == b || strings.Trim(a, "0") == "" {
		t.Fatalf("randomPassword: %q %q %v %v", a, b, err, err2)
	}
}

// stubAnsible installs a fake ansible-playbook that logs argv and fails only the main playbook.
func stubAnsible(t *testing.T) (logPath string) {
	t.Helper()
	bin := t.TempDir()
	logPath = filepath.Join(bin, "calls.log")
	script := "#!/bin/sh\necho \"$@\" >> " + logPath + "\ncase \"$*\" in *" + airgapPlaybook + "*) exit 1;; esac\nexit 0\n"
	if err := os.WriteFile(filepath.Join(bin, "ansible-playbook"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	return logPath
}

func TestAirgapKeyCleanupRunsEvenWhenPlaybookFails(t *testing.T) {
	cfg := airgapTestConfig(t, airgapModule)
	saved := customflag.ServiceFlag
	t.Cleanup(func() { customflag.ServiceFlag = saved })
	t.Setenv(testTagEnv, "tarball")
	logPath := stubAnsible(t)

	if err := executeAnsiblePlaybook(cfg); err == nil {
		t.Fatal("main playbook failure must surface")
	}
	calls, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(calls), airgapKeyCleanupPlaybook) {
		t.Fatalf("cleanup playbook must run after a failed install: %s", calls)
	}
	if _, err := os.Stat(filepath.Join(cfg.InfraProvisioner.RunDir, airgapSecretsName)); !os.IsNotExist(err) {
		t.Fatalf("secrets file must be gone after the run: %v", err)
	}
}
