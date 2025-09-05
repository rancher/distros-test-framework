package qainfra

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/rancher/distros-test-framework/internal/resources"
)

type Inventory struct {
	Path string
}

func executeAnsiblePlaybook(config *QAInfraConfig) error {
	playbookName, playbookNameErr := getPlaybookName(config.Product)
	if playbookNameErr != nil {
		return fmt.Errorf("failed to get playbook name: %w", playbookNameErr)
	}

	resources.LogLevel("info", "Installing %s with Ansible using playbook: %s", config.Product, playbookName)

	// Build ansible arguments
	args, err := buildAnsibleArgs(config, playbookName)
	if err != nil {
		return fmt.Errorf("failed to build ansible arguments: %w", err)
	}

	// Execute playbook with enhanced error handling
	resources.LogLevel("info", "Executing Ansible playbook with args: %v", args)
	if err := runCmdWithTimeout(config.Ansible.Dir, 25*time.Minute, "ansible-playbook", args...); err != nil {
		resources.LogLevel("error", "Ansible playbook execution failed. Check the following:")
		resources.LogLevel("error", "1. SSH connectivity to nodes")
		resources.LogLevel("error", "2. RKE2 service status on target nodes")
		resources.LogLevel("error", "3. SELinux/firewall configuration")
		resources.LogLevel("error", "4. CNI compatibility with the chosen configuration")
		return fmt.Errorf("ansible playbook failed: %w", err)
	}

	resources.LogLevel("info", "Ansible playbook execution completed successfully")
	resources.LogLevel("debug", "Completed ansible playbook execution")

	return nil
}

// setupAnsibleEnvironment clones ansible playbooks and sets up environment
func setupAnsibleEnvironment(config *QAInfraConfig) error {
	resources.LogLevel("info", "Downloading Ansible playbooks for %s installation...", config.Product)

	fmoralBranch := "add.vsphere"
	repoURL := "https://github.com/fmoral2/qa-infra-automation.git"
	if err := runCmd(config.RootDir, "git", "clone", "--depth", "1", "--filter=blob:none", "--sparse", "--branch",
		fmoralBranch, repoURL, config.TempDir); err != nil {
		return fmt.Errorf("git clone failed: %w", err)
	}

	if err := runCmd(config.TempDir, "git", "sparse-checkout", "set", config.Ansible.Path); err != nil {
		return fmt.Errorf("sparse checkout failed: %w", err)
	}

	// Set environment variables for Ansible
	if err := setupAnsibleEnvironmentVars(config); err != nil {
		return fmt.Errorf("failed to setup environment variables: %w", err)
	}

	resources.LogLevel("debug", "Setup ansible environment")

	return nil
}

// generateInventory creates and validates the ansible inventory
func generateInventory(config *QAInfraConfig) error {
	nodes, err := getAllNodesFromState(config.NodeSource)
	if err != nil {
		return fmt.Errorf("failed to get node information from state: %w", err)
	}

	inventoryPath, err := writeINIInventory(config.Ansible.Dir, nodes, config.SSHConfig.KeyPath, config.SSHConfig.User)
	if err != nil {
		return fmt.Errorf("failed to write INI inventory file: %w", err)
	}

	if err := preflightInventory(config.Ansible.Dir, inventoryPath); err != nil {
		return fmt.Errorf("inventory validation failed: %w", err)
	}

	// set inventory path in config.
	config.Inventory.Path = inventoryPath

	resources.LogLevel("info", "Generated and validated inventory: %s", inventoryPath)

	return nil
}

// writeINIInventory creates an INI inventory file with proper groups for Ansible
func writeINIInventory(ansibleDir string, nodes []InfraNode, KeyPath, sshUser string) (string, error) {
	var b strings.Builder

	// Separate nodes into master and worker groups
	var masterNodes, workerNodes []InfraNode
	for _, n := range nodes {
		if n.Name == "" || n.PublicIP == "" {
			continue
		}
		// Check if node has master/control-plane roles
		if strings.Contains(n.Role, "etcd") || strings.Contains(n.Role, "cp") || n.Name == "master" {
			masterNodes = append(masterNodes, n)
		} else {
			workerNodes = append(workerNodes, n)
		}
	}

	// Write master group
	if len(masterNodes) > 0 {
		b.WriteString("[master]\n")
		for _, n := range masterNodes {
			if n.Role != "" {
				b.WriteString(fmt.Sprintf("%s ansible_host=%s ansible_role=%s\n", n.Name, n.PublicIP, n.Role))
			} else {
				b.WriteString(fmt.Sprintf("%s ansible_host=%s\n", n.Name, n.PublicIP))
			}
		}
		b.WriteString("\n")
	}

	// Write worker group
	if len(workerNodes) > 0 {
		b.WriteString("[worker]\n")
		for _, n := range workerNodes {
			if n.Role != "" {
				b.WriteString(fmt.Sprintf("%s ansible_host=%s ansible_role=%s\n", n.Name, n.PublicIP, n.Role))
			} else {
				b.WriteString(fmt.Sprintf("%s ansible_host=%s\n", n.Name, n.PublicIP))
			}
		}
		b.WriteString("\n")
	}

	// Write all group with children
	b.WriteString("[all:children]\n")
	if len(masterNodes) > 0 {
		b.WriteString("master\n")
	}
	if len(workerNodes) > 0 {
		b.WriteString("worker\n")
	}
	b.WriteString("\n")

	// Write common variables
	b.WriteString("[all:vars]\n")

	b.WriteString(fmt.Sprintf("ansible_user=%s\n", sshUser))
	if KeyPath != "" {
		b.WriteString(fmt.Sprintf("ansible_ssh_private_key_file=%s\n", KeyPath))
	}
	b.WriteString("ansible_ssh_common_args=-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null\n")

	invPath := filepath.Join(ansibleDir, "inventory.ini")
	if err := os.WriteFile(invPath, []byte(b.String()), 0644); err != nil {
		return "", err
	}

	resources.LogLevel("info", "Generated INI inventory with %d master nodes and %d worker nodes", len(masterNodes), len(workerNodes))
	resources.LogLevel("debug", "INI Inventory content:\n%s", b.String())

	return invPath, nil
}

// preflightInventory runs `ansible-inventory --list` against a path to verify parsing
func preflightInventory(workingDir, inventoryPath string) error {
	resources.LogLevel("info", "Validating inventory: %s", inventoryPath)
	cmd := exec.Command("ansible-inventory", "-i", inventoryPath, "--list")
	cmd.Dir = workingDir
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	output := string(out)

	// Only fail on actual errors, not warnings
	if err != nil {
		resources.LogLevel("warn", "Inventory validation failed with error: %v\n%s", err, output)
		return fmt.Errorf("inventory validation error: %w", err)
	}

	// Check if the output contains valid JSON with hosts (successful parsing)
	if strings.Contains(output, `"_meta"`) &&
		(strings.Contains(output, `"hostvars"`) || strings.Contains(output, `"hosts"`)) {
		resources.LogLevel("info", "Inventory validation successful")
		return nil
	}

	// If no valid hosts found, this indicates a parsing issue
	if strings.Contains(output, "No inventory was parsed") ||
		strings.Contains(output, "only implicit localhost") {
		resources.LogLevel("warn", "Inventory validation failed - no valid hosts found:\n%s", output)
		return fmt.Errorf("no valid hosts found in inventory")
	}

	resources.LogLevel("info", "Inventory validation passed with warnings (non-critical)")
	return nil
}

func setupAnsibleEnvironmentVars(config *QAInfraConfig) error {
	envVars := map[string]string{
		"TERRAFORM_NODE_SOURCE": config.NodeSource,
		"ANSIBLE_CONFIG":        filepath.Join(config.Ansible.Dir, "ansible.cfg"),
		"REPO_ROOT":             config.TempDir,
		"WORKSPACE_NAME":        config.Workspace,
		"TFVARS_FILE":           "vars.tfvars",
		"KUBECONFIG_FILE":       config.KubeconfigPath,
		"KUBECONFIG":            config.KubeconfigPath,
		"TF_WORKSPACE":          config.Workspace,
	}

	// Set workspace state file paths
	workspaceStateFile := filepath.Join(config.NodeSource, "terraform.tfstate.d", config.Workspace, "terraform.tfstate")
	envVars["TERRAFORM_STATE_FILE"] = workspaceStateFile
	envVars["TF_STATE_FILE"] = workspaceStateFile
	envVars["TERRAFORM_WORKSPACE"] = config.Workspace

	// Apply all environment variables
	for key, value := range envVars {
		os.Setenv(key, value)
	}

	// Set global kubeconfig
	resources.KubeConfigFile = config.KubeconfigPath

	resources.LogLevel("debug", "Set environment variables for Ansible")

	return nil
}

// getPlaybookName returns the appropriate playbook name for the product
func getPlaybookName(product string) (string, error) {
	switch strings.ToLower(product) {
	case "k3s":
		return "k3s-playbook.yml", nil
	case "rke2":
		return "rke2-playbook.yml", nil

	default:
		return "", fmt.Errorf("unknown product for playbook: %s", product)
	}
}

// buildAnsibleArgs builds arguments for ansible-playbook command
func buildAnsibleArgs(config *QAInfraConfig, playbookName string) ([]string, error) {
	installVersion := config.InstallVersion

	// Build base arguments
	args := []string{
		"-i", config.Inventory.Path, playbookName,
		"--extra-vars", fmt.Sprintf("kubeconfig_file=%s", config.KubeconfigPath),
		"--extra-vars", fmt.Sprintf("kubernetes_version=%s", installVersion),
	}

	// Add server and worker flags
	args = addAnsibleFlags(args)

	// Add channel if specified
	if channelVal := getChannelValue(); channelVal != "" {
		args = append(args, "--extra-vars", fmt.Sprintf("channel=%s", channelVal))
	}

	// Add install method for RKE2
	if strings.Contains(config.Product, "rke2") {
		installMethod := ""
		if envInstallMethod := strings.TrimSpace(os.Getenv("INSTALL_METHOD")); envInstallMethod != "" {
			installMethod = envInstallMethod
		}
		args = append(args, "--extra-vars", fmt.Sprintf("install_method=%s", installMethod))

		// Add CNI for RKE2 - use canal as default for better compatibility
		cniValue := "canal"
		if envCNI := strings.TrimSpace(os.Getenv("CNI")); envCNI != "" {
			cniValue = envCNI
		}
		args = append(args, "--extra-vars", fmt.Sprintf("cni=%s", cniValue))

	}

	return args, nil
}

// getChannelValue gets the channel value from environment
func getChannelValue() string {
	return strings.TrimSpace(os.Getenv("CHANNEL"))
}

// addAnsibleFlags adds server and worker flags to ansible arguments
func addAnsibleFlags(args []string) []string {
	normalize := func(s string) string {
		s = strings.TrimSpace(s)
		s = strings.ReplaceAll(s, "\\n", "\n")
		return s
	}

	serverFlagsVal := normalize(os.Getenv("SERVER_FLAGS"))
	if serverFlagsVal != "" {
		args = append(args, "--extra-vars", fmt.Sprintf("server_flags=%q", serverFlagsVal))
		resources.LogLevel("debug", "Passing server flags to Ansible: %s", serverFlagsVal)
	}

	workerFlagsVal := normalize(os.Getenv("WORKER_FLAGS"))
	if workerFlagsVal != "" {
		args = append(args, "--extra-vars", fmt.Sprintf("worker_flags=%q", workerFlagsVal))
		resources.LogLevel("debug", "Passing worker flags to Ansible: %s", workerFlagsVal)
	}

	return args
}
