package qainfra

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/rancher/distros-test-framework/internal/provisioning/driver"
	"github.com/rancher/distros-test-framework/internal/resources"
)

// setupAnsibleEnvironment clones ansible playbooks and sets up environment.
func setupAnsibleEnvironment(config *driver.InfraConfig) error {
	resources.LogLevel("info", "Pulling Ansible playbooks for %s installation...", config.Product)

	if err := runCmdWithTimeout(config.InfraProvisioner.RootDir, 2*time.Minute,
		"git", "clone", "--depth", "1", "--filter=blob:none", "--sparse", "--branch",
		"main", "https://github.com/rancher/qa-infra-automation.git", config.InfraProvisioner.TempDir); err != nil {
		return fmt.Errorf("git clone failed: %w", err)
	}

	ansibleDir := "ansible/" + config.Product
	if err := runCmdWithTimeout(config.InfraProvisioner.TempDir, 2*time.Minute,
		"git", "sparse-checkout", "set", ansibleDir, "ansible/roles"); err != nil {
		return fmt.Errorf("sparse checkout failed: %w", err)
	}

	if err := patchRKE2ConfigTemplate(config); err != nil {
		return fmt.Errorf("failed to patch rke2_config template: %w", err)
	}

	if err := installAnsibleCollection(config.InfraProvisioner.TempDir); err != nil {
		return fmt.Errorf("failed to install ansible collection: %w", err)
	}

	if err := setAnsibleEnvVars(config); err != nil {
		return fmt.Errorf("failed to setup environment variables: %w", err)
	}

	if err := generateTemplateInventory(config); err != nil {
		return fmt.Errorf("template inventory generation failed: %w", err)
	}

	return nil
}

// patchRKE2ConfigTemplate appends a node-external-ip directive to the cloned.
// rke2_config role's config.yaml.j2.
// The substitution uses ansible_host, which our static inventory populates
// from cluster_nodes_json[].public_ip per node.
func patchRKE2ConfigTemplate(config *driver.InfraConfig) error {
	templatePath := filepath.Join(
		config.InfraProvisioner.TempDir,
		"ansible", "roles", "rke2_config", "templates", "config.yaml.j2",
	)
	if _, err := os.Stat(templatePath); err != nil {
		// Sparse checkout may exclude this path on a future upstream
		// reorganization; warn loudly but don't hard-fail provisioning.
		resources.LogLevel("warn",
			"rke2_config template not found at %s — kubelet will not advertise "+
				"node-external-ip and FetchNodeExternalIPs() will be empty (err: %v)",
			templatePath, err)

		return nil
	}

	patch := "\n# Injected by distros-test-framework: surface AWS public IP via kubelet.\n" +
		"node-external-ip: {{ ansible_host }}\n"

	f, err := os.OpenFile(templatePath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open template for append: %w", err)
	}
	defer f.Close()
	if _, err := f.WriteString(patch); err != nil {
		return fmt.Errorf("append node-external-ip stanza: %w", err)
	}

	resources.LogLevel("info",
		"Patched rke2_config template to advertise node-external-ip from ansible_host")

	return nil
}

// installAnsibleCollection installs the required collection for dynamic inventory.
func installAnsibleCollection(workingDir string) error {
	resources.LogLevel("debug", "Installing cloud.terraform Ansible collection for dynamic inventory")

	if err := runCmdWithTimeout(workingDir, 3*time.Minute,
		"ansible-galaxy", "collection", "install", "cloud.terraform", "--force"); err != nil {
		return fmt.Errorf("failed to install cloud.terraform collection: %w", err)
	}

	resources.LogLevel("debug", "Successfully installed cloud.terraform collection")

	return nil
}

func setAnsibleEnvVars(config *driver.InfraConfig) error {
	stateFile := filepath.Join(
		config.InfraProvisioner.TFNodeSource, "terraform.tfstate.d",
		config.InfraProvisioner.Workspace, "terraform.tfstate",
	)

	relativePath, err := filepath.Rel(config.InfraProvisioner.TempDir, config.InfraProvisioner.TFNodeSource)
	if err != nil {
		return fmt.Errorf("failed to get relative path for TERRAFORM_NODE_SOURCE: %w", err)
	}

	envVars := map[string]string{
		// Terraform/OpenTofu configuration.
		"TF_WORKSPACE":          config.InfraProvisioner.Workspace,
		"TERRAFORM_NODE_SOURCE": relativePath,
		"TERRAFORM_STATE_FILE":  stateFile,
		"TF_STATE_FILE":         stateFile,
		"TERRAFORM_WORKSPACE":   config.InfraProvisioner.Workspace,
		"TFVARS_FILE":           "vars.tfvars",

		// Ansible configuration.
		"ANSIBLE_CONFIG":            filepath.Join(config.InfraProvisioner.Ansible.Dir, "ansible.cfg"),
		"ANSIBLE_INVENTORY_ENABLED": "host_list,script,auto,yaml,ini",
		"ANSIBLE_COLLECTIONS_PATHS": "/root/.ansible/collections:/usr/share/ansible/collections:~/.ansible/collections",
		"ANSIBLE_TIMEOUT":           "30",
		"ANSIBLE_CONNECT_TIMEOUT":   "30",
		"ANSIBLE_HOST_KEY_CHECKING": "False",

		// SSH configuration for template substitution.
		"ANSIBLE_SSH_PRIVATE_KEY_FILE": config.Cluster.SSH.PrivKeyPath,
		"ANSIBLE_USER":                 config.Cluster.SSH.User,
		"ANSIBLE_SSH_COMMON_ARGS": "-o StrictHostKeyChecking=no " +
			" -o UserKnownHostsFile=/dev/null -o PasswordAuthentication=no",
		"ANSIBLE_SSH_PRIVATE_KEY": config.Cluster.SSH.PrivKeyPath,
		"ANSIBLE_REMOTE_USER":     config.Cluster.SSH.User,

		// paths and workspace.
		"REPO_ROOT":       config.InfraProvisioner.TempDir,
		"WORKSPACE_NAME":  config.InfraProvisioner.Workspace,
		"KUBECONFIG_FILE": config.InfraProvisioner.KubeconfigPath,
		"KUBECONFIG":      config.InfraProvisioner.KubeconfigPath,
	}

	for key, value := range envVars {
		//nolint:revive //no need to check the error.
		os.Setenv(key, value)
	}

	// update framework global value for kubeconfig path.
	resources.KubeConfigFile = config.InfraProvisioner.KubeconfigPath

	return nil
}

// generateTemplateInventory builds a static Ansible inventory.
// from the Tofu cluster_nodes_json output,so no dynamic.
// plugin involvement at playbook runtime —all gets hardcoded in the YAML we write.
func generateTemplateInventory(config *driver.InfraConfig) error {
	if err := setAnsibleEnvVars(config); err != nil {
		return fmt.Errorf("failed to set environment variables: %w", err)
	}

	if err := writeStaticInventory(config); err != nil {
		return fmt.Errorf("failed to write static inventory: %w", err)
	}

	if err := ensureSSHAnsibleConfig(config); err != nil {
		resources.LogLevel("warn", "Failed to update ansible.cfg: %v", err)
	}

	resources.LogLevel("info", "Static inventory written to %s",
		config.InfraProvisioner.Inventory.Path)

	return nil
}

// ensureSSHAnsibleConfig ensures ansible.cfg has the correct SSH configuration.
func ensureSSHAnsibleConfig(config *driver.InfraConfig) error {
	ansibleCfgPath := filepath.Join(config.InfraProvisioner.Ansible.Dir, "ansible.cfg")

	var content string
	if existingContent, err := os.ReadFile(ansibleCfgPath); err == nil {
		content = string(existingContent)
	}

	// add SSH settings to the existing [defaults] section.
	lines := strings.Split(content, "\n")
	var newLines []string
	inDefaultsSection := false
	sshConfigAdded := false

	for _, line := range lines {
		newLines = append(newLines, line)

		// check if is in the [defaults] section.
		if strings.TrimSpace(line) == "[defaults]" {
			inDefaultsSection = true
		} else if strings.HasPrefix(strings.TrimSpace(line), "[") && strings.TrimSpace(line) != "[defaults]" {
			// moved to a different section, add SSH config if not already added.
			if inDefaultsSection && !sshConfigAdded {
				newLines = append(newLines[:len(newLines)-1],
					"private_key_file = "+config.Cluster.SSH.PrivKeyPath,
					"remote_user = "+config.Cluster.SSH.User,
					line)
				sshConfigAdded = true
			}
			inDefaultsSection = false
		}
	}

	// only if still on defaults section at the end, add SSH config.
	if inDefaultsSection && !sshConfigAdded {
		newLines = append(newLines,
			"private_key_file = "+config.Cluster.SSH.PrivKeyPath,
			"remote_user = "+config.Cluster.SSH.User)
	}

	finalConfig := strings.Join(newLines, "\n")
	if err := os.WriteFile(ansibleCfgPath, []byte(finalConfig), 0o644); err != nil {
		return fmt.Errorf("failed to write ansible.cfg: %w", err)
	}

	resources.LogLevel("debug", "Updated SSH configuration in ansible.cfg [defaults] section")

	return nil
}

func executeAnsiblePlaybook(config *driver.InfraConfig) error {
	playbookPath, playbookNameErr := getPlaybookPath(config.Product)
	if playbookNameErr != nil {
		return fmt.Errorf("failed to get playbook name: %w", playbookNameErr)
	}
	resources.LogLevel("info", "Installing %s with Ansible using playbook: %s", config.Product, playbookPath)

	args, argsErr := buildAnsibleArgs(config, playbookPath)
	if argsErr != nil {
		return fmt.Errorf("failed to build ansible arguments: %w", argsErr)
	}

	if createErr := createAnsibleVarsFile(config); createErr != nil {
		return fmt.Errorf("failed to create vars.yaml: %w", createErr)
	}

	resources.LogLevel("debug", "Executing Ansible playbook with args: %v", args)

	if executeErr := runCmdWithTimeout(config.InfraProvisioner.Ansible.Dir, 25*time.Minute,
		"ansible-playbook", args...); executeErr != nil {
		return fmt.Errorf("ansible playbook failed: %w", executeErr)
	}

	resources.LogLevel("info", "Ansible playbook execution completed successfully")

	return nil
}

func getPlaybookPath(product string) (string, error) {
	switch strings.ToLower(product) {
	case "k3s":
		return "k3s-playbook.yml", nil
	case "rke2":
		return "rke2-playbook.yml", nil

	default:
		return "", fmt.Errorf("unknown product for playbook: %s", product)
	}
}

func buildAnsibleArgs(config *driver.InfraConfig, playbookPath string) ([]string, error) {
	installVersion := config.InstallVersion

	args := []string{
		"-i", config.InfraProvisioner.Inventory.Path, playbookPath,
		"--extra-vars", "kubeconfig_file=" + config.InfraProvisioner.KubeconfigPath,
		"--extra-vars", "kubernetes_version=" + installVersion,
	}

	serverFlags := config.Cluster.Config.ServerFlags
	args = addServerFlags(args, serverFlags)
	args = addWorkerFlags(args, config.Cluster.Config.WorkerFlags)
	args = addChannel(args, config.Cluster.Config.Channel)

	if strings.Contains(config.Product, "rke2") {
		args = addInstallMethod(args, config.Cluster.Config.InstallMethod)
		args = addCNI(args, config.CNI)
		args = addRKE2AdditionalConfig(args, serverFlags)
	}

	return args, nil
}

// addRKE2AdditionalConfig parses server_flags into a JSON object.
func addRKE2AdditionalConfig(args []string, serverFlags string) []string {
	extras := parseServerFlagsToDict(resources.NormalizeString(serverFlags))
	if len(extras) == 0 {
		return args
	}

	// Wrap in {var: dict} JSON so Ansible parses the value as a dict.
	// Bare "key=json" CLI form treats the value as a literal string.
	wrapper := map[string]any{"rke2_additional_config": extras}
	jsonBytes, err := json.Marshal(wrapper)
	if err != nil {
		resources.LogLevel("warn",
			"failed to marshal rke2_additional_config (%v); SERVER_FLAGS will "+
				"NOT be written into /etc/rancher/rke2/config.yaml", err)
		return args
	}

	keys := make([]string, 0, len(extras))
	for k := range extras {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	resources.LogLevel("info",
		"Forwarding %d SERVER_FLAGS key(s) into rke2_additional_config: %v",
		len(keys), keys)

	return append(args, "--extra-vars", string(jsonBytes))
}

// parseServerFlagsToDict turns the multi-line YAML scalar that lives behind
// SERVER_FLAGS into a flat string→string map. Each non-empty, non-comment
// line is parsed as "key: value"; surrounding whitespace and optional quotes
// on the value are trimmed. Lines without a colon are skipped.
//
// Values stay as strings; the upstream config.yaml.j2 template emits them
// unquoted, and RKE2's YAML parser coerces "true"/"644"/"nginx" to the
// proper Go type at config-read time.
func parseServerFlagsToDict(s string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		i := strings.IndexByte(line, ':')
		if i <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:i])
		val := strings.TrimSpace(line[i+1:])
		val = strings.Trim(val, `"'`)
		if key == "" {
			continue
		}
		out[key] = val
	}

	return out
}

func addChannel(args []string, channel string) []string {
	channelVal := resources.NormalizeString(channel)
	if channelVal == "" {
		return args
	}
	args = append(args, "--extra-vars", "channel="+channelVal)

	return args
}

func addInstallMethod(args []string, installMethod string) []string {
	method := resources.NormalizeString(installMethod)
	if method == "" {
		return args
	}
	args = append(args, "--extra-vars", "install_method="+method)

	return args
}

func addCNI(args []string, cni string) []string {
	cniValue := resources.NormalizeString(cni)
	if cniValue == "" {
		cniValue = "canal"
	}
	// The upstream rke2-playbook asserts `cni is defined && cni | length > 0`,
	// so we set both `cni` (for the assertion) and `rke2_cni` (the var that
	// the rke2_config role actually reads — see ansible/roles/rke2_config/
	// defaults/main.yml). Without rke2_cni, the role's default "calico"
	// always wins regardless of what the caller asked for.
	args = append(args, "--extra-vars", "cni="+cniValue, "--extra-vars", "rke2_cni="+cniValue)

	return args
}

func addServerFlags(args []string, serverFlags string) []string {
	serverFlagsVal := resources.NormalizeString(serverFlags)
	if serverFlagsVal == "" {
		return args
	}
	args = append(args, "--extra-vars", fmt.Sprintf("server_flags=%q", serverFlagsVal))

	return args
}

func addWorkerFlags(args []string, workerFlags string) []string {
	workerFlagsVal := resources.NormalizeString(workerFlags)
	if workerFlagsVal == "" {
		return args
	}
	args = append(args, "--extra-vars", fmt.Sprintf("worker_flags=%q", workerFlagsVal))

	return args
}

// createAnsibleVarsFile creates the vars.yaml file that the playbook expects.
func createAnsibleVarsFile(config *driver.InfraConfig) error {
	varsPath := filepath.Join(config.InfraProvisioner.Ansible.Dir, "vars.yaml")

	varsContent := fmt.Sprintf(`---
kubernetes_version: '%s' 
kubeconfig_file: '%s'
`, config.InstallVersion, config.InfraProvisioner.KubeconfigPath)

	// add CNI only for RKE2.
	if strings.Contains(strings.ToLower(config.Product), "rke2") {
		cni := config.CNI
		if cni == "" {
			cni = "calico"
		}
		varsContent += fmt.Sprintf("cni: '%s'\n", cni)
	}

	if err := os.WriteFile(varsPath, []byte(varsContent), 0o644); err != nil {
		return fmt.Errorf("failed to write vars.yaml: %w", err)
	}

	resources.LogLevel("debug", "Created vars.yaml at %s with content:\n%s", varsPath, varsContent)

	return nil
}
