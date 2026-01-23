package qainfra

import (
	"fmt"
	"os"
	"path/filepath"
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
		"git", "sparse-checkout", "set", ansibleDir); err != nil {
		return fmt.Errorf("sparse checkout failed: %w", err)
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
		"ANSIBLE_INVENTORY_ENABLED": "host_list,script,auto,yaml,ini,cloud.terraform.terraform_state",
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

// generateTemplateInventory sets up dynamic inventory using the template.
func generateTemplateInventory(config *driver.InfraConfig) error {
	if err := setAnsibleEnvVars(config); err != nil {
		return fmt.Errorf("failed to set environment variables: %w", err)
	}

	templatePath := filepath.Join(config.InfraProvisioner.TempDir,
		fmt.Sprintf("ansible/%s/default/inventory-template.yml", config.Product))
	inventoryPath := config.InfraProvisioner.Inventory.Path

	cmd := fmt.Sprintf("TERRAFORM_NODE_SOURCE='%s' envsubst < %s > %s",
		config.InfraProvisioner.TFNodeSource, templatePath, inventoryPath)
	// We run envsubst with the absolute path so the inventory plugin can find the state file.
	// The playbook itself requires a relative path (handled by setAnsibleEnvVars).
	if err := runCmdWithTimeout(config.InfraProvisioner.Ansible.Dir, 2*time.Minute, "bash", "-c", cmd); err != nil {
		return fmt.Errorf("failed to generate inventory from template: %w", err)
	}

	if err := ensureSSHAnsibleConfig(config); err != nil {
		resources.LogLevel("warn", "Failed to update ansible.cfg: %v", err)
	}

	resources.LogLevel("info", "Dynamic inventory generated from template with content from: %s", templatePath)

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

	args = addServerFlags(args, config.Cluster.Config.ServerFlags)
	args = addWorkerFlags(args, config.Cluster.Config.WorkerFlags)
	args = addChannel(args, config.Cluster.Config.Channel)

	if strings.Contains(config.Product, "rke2") {
		args = addInstallMethod(args, config.Cluster.Config.InstallMethod)
		args = addCNI(args, config.CNI)
	}

	return args, nil
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
	args = append(args, "--extra-vars", "cni="+cniValue)

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
