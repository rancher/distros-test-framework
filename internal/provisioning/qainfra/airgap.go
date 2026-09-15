package qainfra

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/rancher/distros-test-framework/internal/pkg/customflag"
	"github.com/rancher/distros-test-framework/internal/provisioning/driver"
	"github.com/rancher/distros-test-framework/internal/resources"

	"gopkg.in/yaml.v3"
)

// Airgap mode: private nodes + bastion from cluster_nodes, product installed offline
// by the qa-infra airgap playbook; the connected path is untouched otherwise.
const (
	airgapModule             = "airgap"
	airgapModuleMarker       = "# __AIRGAP_MODULE_ARGS__"
	airgapPlaybook           = "airgap-playbook.yml"
	airgapKeyCleanupPlaybook = "airgap-cleanup-keys.yml"
	airgapAnsiblePath        = "ansible/airgap"
	airgapFactsName          = "airgap-facts.json"
	airgapSecretsName        = "ansible-secrets.json" //nolint:gosec // file name, not a credential
	airgapFactsSchema        = 2
	bastionTypeEnv           = "BASTION_INSTANCE_TYPE"
	defaultTarballType       = "tar.zst"
	testTagEnv               = customflag.TestTagEnv
	testArgsEnv              = customflag.TestArgsEnv
)

// cniRE is the fallback for flag blobs that are not valid YAML: a "cni: <value>" line, quotes optional.
var cniRE = regexp.MustCompile(`(?m)(?:^|\\n|\n)[ \t]*cni:[ \t]*["']?([A-Za-z0-9_,. -]+)`)

// cniFromFlags returns the normalized cni value of a k3s/rke2 flags blob: the flags are
// YAML lines (literal "\n" separated), so they are parsed as YAML first, regex as fallback.
func cniFromFlags(serverFlags string) string {
	text := strings.ReplaceAll(serverFlags, `\n`, "\n")
	var doc map[string]any
	if err := yaml.Unmarshal([]byte(text), &doc); err == nil {
		switch v := doc["cni"].(type) {
		case string:
			return normalizeCNI(v)
		case []any:
			parts := make([]string, 0, len(v))
			for _, item := range v {
				parts = append(parts, fmt.Sprint(item))
			}

			return normalizeCNI(strings.Join(parts, ","))
		default:
			return ""
		}
	}
	m := cniRE.FindStringSubmatch(serverFlags)
	if len(m) != 2 {
		return ""
	}

	return normalizeCNI(m[1])
}

// normalizeCNI lowercases and drops spaces and empty items: "Multus, Canal" -> "multus,canal".
func normalizeCNI(raw string) string {
	var parts []string
	for _, p := range strings.Split(strings.ToLower(strings.ReplaceAll(raw, " ", "")), ",") {
		if p != "" {
			parts = append(parts, p)
		}
	}

	return strings.Join(parts, ",")
}

func isAirgap(cfg *driver.InfraConfig) bool {
	return strings.EqualFold(strings.TrimSpace(cfg.Module), airgapModule)
}

// normalizeArch maps the many spellings (arm, aarch64, x86_64) onto amd64|arm64.
func normalizeArch(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "amd64", "x86_64", "x86-64":
		return "amd64"
	case "arm64", "arm", "aarch64":
		return "arm64"
	default:
		return strings.ToLower(strings.TrimSpace(raw))
	}
}

// injectAirgapModuleArgs fills (or removes) the module-arguments marker so
// connected runs stay compatible with qa-infra refs that predate the bastion.
func injectAirgapModuleArgs(mainTf string, airgap bool) string {
	args := ""
	if airgap {
		args = "bastion = {\n" +
			"    enabled       = var.bastion_enabled\n" +
			"    instance_type = var.bastion_instance_type\n" +
			"  }\n" +
			"  run_id       = var.run_id\n" +
			"  qa_infra_sha = var.qa_infra_sha\n" +
			"  arch         = var.arch"
	}

	return strings.ReplaceAll(mainTf, airgapModuleMarker, args)
}

// writeAirgapTFVars turns the run into an airgap topology: private nodes + bastion,
// tagged with the run id and the pinned qa-infra commit.
func writeAirgapTFVars(config *driver.InfraConfig) error {
	ip := config.InfraProvisioner
	raw := map[string]string{
		"airgap_setup":    "true",
		"bastion_enabled": "true",
	}
	for k, v := range raw {
		if err := setOrAppendTFVarRaw(ip.Terraform.TFVarsPath, k, v); err != nil {
			return fmt.Errorf("set %s in tfvars: %w", k, err)
		}
	}

	quoted := map[string]string{
		"run_id":       ip.RunID,
		"qa_infra_sha": ip.QAInfraSHA,
		"arch":         normalizeArch(config.Cluster.Config.Arch),
	}
	if bastionType := strings.TrimSpace(os.Getenv(bastionTypeEnv)); bastionType != "" {
		quoted["bastion_instance_type"] = bastionType
	}
	for k, v := range quoted {
		if err := setOrAppendTFVar(ip.Terraform.TFVarsPath, k, v); err != nil {
			return fmt.Errorf("set %s in tfvars: %w", k, err)
		}
	}

	return nil
}

// effectiveCNI is the explicit CNI or the "cni:" line of server flags; when both
// are set they must agree, so artifacts and config.yaml can never diverge.
func effectiveCNI(explicit, serverFlags string) (string, error) {
	fromFlags := cniFromFlags(serverFlags)
	explicit = normalizeCNI(explicit)
	if explicit != "" && fromFlags != "" && explicit != fromFlags {
		return "", fmt.Errorf("CNI=%q conflicts with server_flags cni: %q; set only one or make them match",
			explicit, fromFlags)
	}
	if explicit != "" {
		return explicit, nil
	}

	return fromFlags, nil
}

// randomPassword returns n random [a-z0-9] chars from crypto/rand, failing closed.
func randomPassword(n int) (string, error) {
	const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("crypto/rand unavailable for the registry password: %w", err)
	}
	for i, v := range b {
		b[i] = alphabet[int(v)%len(alphabet)]
	}

	return string(b), nil
}

// testTag is the suite build tag (see customflag.TestTag).
func testTag() string { return customflag.TestTag() }

// airgapScenario resolves the playbook method from the suite tag and returns the
// airgap flags, defaulting the tarball type and the registry credentials.
func airgapScenario() (string, customflag.FlagConfig, error) {
	flags := customflag.ServiceFlag
	method, err := customflag.AirgapMethod()
	if err != nil {
		return "", flags, err
	}
	if flags.AirgapFlag.TarballType == "" {
		flags.AirgapFlag.TarballType = defaultTarballType
	}
	if method == "private_registry" &&
		(flags.AirgapFlag.RegistryUsername == "" || flags.AirgapFlag.RegistryPassword == "") {
		// Throwaway registry: a fresh random password per run, kept in the shared flags
		// so every later step (secrets file, report masking) sees the same value.
		resources.LogLevel("info", "REGISTRY_USERNAME/REGISTRY_PASSWORD not set; generating a per-run registry password")
		password, err := randomPassword(24)
		if err != nil {
			return "", flags, err
		}
		customflag.ServiceFlag.AirgapFlag.RegistryUsername = customflag.DefaultRegistryUsername
		customflag.ServiceFlag.AirgapFlag.RegistryPassword = password
		flags.AirgapFlag = customflag.ServiceFlag.AirgapFlag
	}

	return method, flags, nil
}

// airgapAnsibleArgs builds the ansible-playbook argv for the airgap playbook.
// Secrets never appear on the command line; they go through a 0600 vars file.
func airgapAnsibleArgs(config *driver.InfraConfig, playbookPath string) ([]string, error) {
	method, flags, err := airgapScenario()
	if err != nil {
		return nil, err
	}
	ip := config.InfraProvisioner
	af := flags.AirgapFlag

	args := []string{
		"-i", ip.Inventory.Path, playbookPath,
		"--extra-vars", "product=" + config.Product,
		"--extra-vars", "kubernetes_version=" + config.InstallVersion,
		"--extra-vars", "arch=" + normalizeArch(config.Cluster.Config.Arch),
		"--extra-vars", "airgap_method=" + method,
		"--extra-vars", "tarball_type=" + af.TarballType,
		"--extra-vars", "kubeconfig_file=" + ip.KubeconfigPath,
		"--extra-vars", "airgap_facts_file=" + airgapFactsPath(config),
		"--extra-vars", "ssh_private_key_file=" + config.Cluster.SSH.PrivKeyPath,
		"--extra-vars", "ssh_key_name=" + config.Cluster.SSH.KeyName,
	}
	if url := strings.TrimSpace(af.ImageRegistryUrl); url != "" {
		args = append(args, "--extra-vars", "image_registry_url="+url)
	}
	if strings.Contains(config.Product, "rke2") {
		cni, err := effectiveCNI(config.CNI, config.Cluster.Config.ServerFlags)
		if err != nil {
			return nil, err
		}
		args = addCNI(args, cni)
	}
	args = addServerFlags(args, config.Cluster.Config.ServerFlags)
	args = addWorkerFlags(args, config.Cluster.Config.WorkerFlags)

	if method == "private_registry" {
		secretsPath, err := writeAnsibleSecretsFile(ip.RunDir, af.RegistryUsername, af.RegistryPassword)
		if err != nil {
			return nil, err
		}
		args = append(args, "--extra-vars", "@"+secretsPath)
	}

	return args, nil
}

func airgapFactsPath(config *driver.InfraConfig) string {
	return filepath.Join(config.InfraProvisioner.Ansible.Dir, airgapFactsName)
}

// writeAnsibleSecretsFile stores registry credentials for `--extra-vars @file`.
func writeAnsibleSecretsFile(runDir, username, password string) (string, error) {
	payload, err := json.Marshal(map[string]string{
		"registry_username": username,
		"registry_password": password,
	})
	if err != nil {
		return "", fmt.Errorf("marshal registry credentials: %w", err)
	}
	path := filepath.Join(runDir, airgapSecretsName)
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		return "", fmt.Errorf("write ansible secrets file: %w", err)
	}

	return path, nil
}

// airgapFacts mirrors airgap-facts.json written by the playbook (no secrets).
type airgapFacts struct {
	SchemaVersion         int    `json:"schema_version"`
	RegistryMode          string `json:"registry_mode"`
	RegistryHost          string `json:"registry_host"`
	RegistryPort          int    `json:"registry_port"`
	RegistryCAPathNodes   string `json:"registry_ca_path_nodes"`
	ArtifactsDir          string `json:"artifacts_dir"`
	ArtifactOrigin        string `json:"artifact_origin"`
	ReleaseURL            string `json:"release_url"`
	BastionKubeconfigPath string `json:"bastion_kubeconfig_path"`
	KubectlPath           string `json:"kubectl_path"`
	NodeCount             int    `json:"node_count"`
}

// collectAirgapFacts is the last pipeline step of an airgap run: it copies what
// the playbook installed into Cluster.Airgap so the suite can assert on it.
func collectAirgapFacts(config *driver.InfraConfig) error {
	if !config.InfraProvisioner.AirgapSetup {
		return nil
	}
	path := airgapFactsPath(config)
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	var facts airgapFacts
	if err := json.Unmarshal(data, &facts); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	if facts.SchemaVersion != airgapFactsSchema {
		return fmt.Errorf("%s has schema_version %d, want %d", path, facts.SchemaVersion, airgapFactsSchema)
	}
	if facts.BastionKubeconfigPath == "" || facts.KubectlPath == "" {
		return errors.New("airgap facts are missing bastion_kubeconfig_path or kubectl_path")
	}

	method, flags, err := airgapScenario()
	if err != nil {
		return err
	}
	config.Cluster.Airgap = driver.AirgapConfig{
		Enabled:               true,
		Method:                method,
		TarballType:           flags.AirgapFlag.TarballType,
		ImageRegistryURL:      flags.AirgapFlag.ImageRegistryUrl,
		RegistryMode:          facts.RegistryMode,
		RegistryHost:          facts.RegistryHost,
		RegistryPort:          facts.RegistryPort,
		RegistryCAPathNodes:   facts.RegistryCAPathNodes,
		ArtifactsDir:          facts.ArtifactsDir,
		ArtifactOrigin:        facts.ArtifactOrigin,
		ReleaseURL:            facts.ReleaseURL,
		BastionKubeconfigPath: facts.BastionKubeconfigPath,
		KubectlPath:           facts.KubectlPath,
	}
	resources.LogLevel("info", "airgap facts: method=%s registry=%s (%s) artifacts=%s origin=%s",
		method, facts.RegistryHost, facts.RegistryMode, facts.ArtifactsDir, facts.ArtifactOrigin)

	return nil
}

// buildAirgapInventory writes bastion + private node groups; every node hop goes
// through the bastion with a ProxyCommand (never overridden at play level).
func buildAirgapInventory(data *clusterNodesJSON, sshUser, keyPath, keyName string) (string, error) {
	if data.Bastion == nil || data.Bastion.PublicIP == "" {
		return "", errors.New("airgap inventory needs a bastion with a public IP in cluster_nodes_json")
	}
	assigned := assignNodeGroups(data, "")
	proxy := fmt.Sprintf("-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null "+
		"-o ProxyCommand='ssh -i %s -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -W %%h:%%p %s@%s'",
		keyPath, sshUser, data.Bastion.PublicIP)

	var b strings.Builder
	b.WriteString("all:\n  vars:\n")
	writeLine(&b, "    ansible_user: ", yamlQuote(sshUser))
	writeLine(&b, "    ssh_private_key_file: ", yamlQuote(keyPath))
	writeLine(&b, "    ssh_key_name: ", yamlQuote(keyName))
	writeLine(&b, "    bastion_host: ", yamlQuote(data.Bastion.PublicIP))
	writeLine(&b, "    bastion_public_dns: ", yamlQuote(data.Bastion.PublicDNS))
	writeLine(&b, "    bastion_private_ip: ", yamlQuote(data.Bastion.PrivateIP))
	b.WriteString("  hosts:\n    bastion-0:\n")
	writeLine(&b, "      ansible_host: ", yamlQuote(data.Bastion.PublicIP))
	b.WriteString(`      ansible_ssh_common_args: "-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null"` + "\n")
	for i := range data.Nodes {
		n := &data.Nodes[i]
		if n.PrivateIP == "" {
			return "", fmt.Errorf("airgap node %s has no private_ip in cluster_nodes_json", n.Name)
		}
		writeLine(&b, "    ", n.Name, ":")
		writeLine(&b, "      ansible_host: ", yamlQuote(n.PrivateIP))
		writeLine(&b, "      ansible_ssh_common_args: ", yamlQuote(proxy))
		writeLine(&b, "      node_roles: ", yamlInlineList(n.Roles))
		writeLine(&b, "      node_type: ", yamlQuote(nodeRole(n, assigned)))
	}
	b.WriteString("  children:\n    bastion:\n      hosts:\n        bastion-0:\n")
	for _, g := range []string{"master", "servers", "workers"} {
		members := make([]string, 0)
		for _, n := range data.Nodes {
			if assigned[n.Name] == g {
				members = append(members, n.Name)
			}
		}
		if len(members) == 0 {
			continue
		}
		writeLine(&b, "    ", g, ":\n      hosts:")
		for _, m := range members {
			writeLine(&b, "        ", m, ":")
		}
	}

	return b.String(), nil
}
