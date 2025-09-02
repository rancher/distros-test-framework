package shared

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gruntwork-io/terratest/modules/terraform"
)

// ProvisionInfrastructure provisions infrastructure using the configured provider.
// If the provider is not set, it will default to legacy.
func ProvisionInfrastructure(infra InfraConfig, c *Cluster) (*Cluster, error) {
	switch infra.InfraProvider {
	case "legacy", "":
		LogLevel("info", "Start provisioning with legacy infrastructure for %s", infra.Product)

		return provisionLegacy(infra)
	case "qa-infra":
		LogLevel("info", "Start provisioning with qa-infra infrastructure for %s", infra.Product)

		return provisionQAInfra(infra, c)
	default:
		return nil, fmt.Errorf("unknown infrastructure provider: %s", infra.InfraProvider)
	}
}

// provisionLegacy uses the existing terraform modules approach(legacy)
func provisionLegacy(infra InfraConfig) (*Cluster, error) {
	return ClusterConfig(infra.Product, infra.Module), nil
}

// provisionQAInfra uses the qa-infra-automation repository approach with remote modules.
func provisionQAInfra(infra InfraConfig, c *Cluster) (*Cluster, error) {
	cfg := addQAInfraEnv(infra, c)

	LogLevel("info", "Starting qa-infra provisioning with workspace: %s", cfg.Workspace)

	// Execute provisioning pipeline
	pipeline := []ProvisioningStep{
		setupDirectories,
		prepareTerraformFiles,
		executeOpenTofuOperations,
		setupAnsibleEnvironment,
		generateInventory,
		applySystemBypasses,
		executeAnsiblePlaybook,
	}

	for i, step := range pipeline {
		if err := step(cfg); err != nil {
			return nil, fmt.Errorf("provisioning step %d failed: %w", i+1, err)
		}
	}

	outputs, err := getOpenTofuOutputs(infra.NodeSource)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster outputs: %w", err)
	}

	ccc := QAInfraClusterConfig(cfg, outputs.FQDN)

	LogLevel("info", "Infrastructure provisioned successfully with qa-infra remote modules")
	LogLevel("info", "Kubeconfig available at: %s", cfg.KubeconfigPath)
	LogLevel("info", "Ansible playbooks downloaded to: %s", cfg.TempDir)

	return ccc, nil
}

// setupDirectories creates required directories for the provisioning process.
func setupDirectories(config *QAInfraConfig) error {
	directories := []string{
		filepath.Join(config.RootDir, "tmp"),
		config.NodeSource,
		config.TempDir,
	}

	for _, dir := range directories {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	return nil
}

// TerraformFiles copies and updates terraform configuration files
func prepareTerraformFiles(config *QAInfraConfig) error {
	// Copy main.tf
	mainTfSrc := filepath.Join(config.RootDir, "infrastructure/qa-infra/main.tf")
	if err := copyFile(mainTfSrc, config.Terraform.MainTfPath); err != nil {
		return fmt.Errorf("failed to copy main.tf: %w", err)
	}

	// Update main.tf module source
	if err := updateMainTfModuleSource(config.QAInfraModule, config.Terraform.MainTfPath); err != nil {
		return fmt.Errorf("failed to update main.tf module source: %w", err)
	}
	// Copy variables.tf
	variablesTfSrc := filepath.Join(config.RootDir, "infrastructure/qa-infra/variables.tf")
	variablesTfDst := filepath.Join(config.NodeSource, "variables.tf")
	LogLevel("info", "Copying variables.tf from %s to %s", variablesTfSrc, variablesTfDst)
	if err := copyFile(variablesTfSrc, variablesTfDst); err != nil {
		return fmt.Errorf("failed to copy variables.tf: %w", err)
	}
	LogLevel("info", "Successfully copied variables.tf")

	// Copy and update vars.tfvars
	tfvarsSrc := filepath.Join(config.RootDir, "infrastructure/qa-infra/vars.tfvars")
	if err := copyAndUpdateVarsFile(tfvarsSrc, config.Terraform.TFVarsPath, config.UniqueID); err != nil {
		return fmt.Errorf("failed to prepare vars file: %w", err)
	}

	// List files in the working directory for debugging
	if files, err := os.ReadDir(config.NodeSource); err == nil {
		LogLevel("info", "Files in working directory %s:", config.NodeSource)
		for _, file := range files {
			LogLevel("info", "  - %s", file.Name())
		}
	}

	LogLevel("debug", "Prepared terraform files")
	return nil
}

// updateMainTfModuleSource updates main.tf to use the correct infrastructure module based on INFRA_MODULE env var
func updateMainTfModuleSource(qaInfraModule, mainTfPath string) error {

	LogLevel("info", "Using infrastructure module: %s", qaInfraModule)

	// Read current main.tf content
	content, err := os.ReadFile(mainTfPath)
	if err != nil {
		return fmt.Errorf("failed to read main.tf: %w", err)
	}

	contentStr := string(content)

	// Update module source to use correct infra module.
	placeholder := "placeholder-for-remote-module"
	fmoralBranch := "add.vsphere"
	modulePath := qaInfraModule + "/modules/cluster_nodes"

	srcModule := fmt.Sprintf("github.com/fmoral2/qa-infra-automation//tofu/%s?ref=%s", modulePath, fmoralBranch)
	contentStr = strings.ReplaceAll(contentStr, placeholder, srcModule)

	// Write updated content back to file
	if err := os.WriteFile(mainTfPath, []byte(contentStr), 0644); err != nil {
		return fmt.Errorf("failed to write updated main.tf: %w", err)
	}

	LogLevel("debug", "Successfully updated main.tf for %s infrastructure", qaInfraModule)

	return nil
}

// executeOpenTofuOperations runs OpenTofu init, workspace, and apply operations
func executeOpenTofuOperations(config *QAInfraConfig) error {
	LogLevel("info", "Provisioning infrastructure with qa-infra remote modules...")

	// Initialize OpenTofu
	if err := runCmdWithTimeout(config.NodeSource, 5*time.Minute, "tofu", "init"); err != nil {
		return fmt.Errorf("tofu init failed: %w", err)
	}

	// Create and select workspace
	_ = runCmd(config.NodeSource, "tofu", "workspace", "new", config.Workspace) // Ignore error if exists
	if err := runCmd(config.NodeSource, "tofu", "workspace", "select", config.Workspace); err != nil {
		return fmt.Errorf("tofu workspace select failed: %w", err)
	}

	// Apply configuration
	tofuArgs := buildTofuApplyArgs(config)
	if err := runCmdWithTimeout(config.NodeSource, 15*time.Minute, "tofu", tofuArgs...); err != nil {
		return fmt.Errorf("tofu apply failed: %w", err)
	}

	LogLevel("debug", "Completed OpenTofu operations")

	return nil
}

// setupAnsibleEnvironment clones ansible playbooks and sets up environment
func setupAnsibleEnvironment(config *QAInfraConfig) error {
	LogLevel("info", "Downloading Ansible playbooks for %s installation...", config.Product)

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

	LogLevel("debug", "Setup ansible environment")

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

	LogLevel("info", "Generated and validated inventory: %s", inventoryPath)

	return nil
}

// executeAnsiblePlaybook runs the ansible playbook for installation
func executeAnsiblePlaybook(config *QAInfraConfig) error {
	playbookName, playbookNameErr := getPlaybookName(config.Product)
	if playbookNameErr != nil {
		return fmt.Errorf("failed to get playbook name: %w", playbookNameErr)
	}

	LogLevel("info", "Installing %s with Ansible using playbook: %s", config.Product, playbookName)

	// Build ansible arguments
	args, err := buildAnsibleArgs(config, playbookName)
	if err != nil {
		return fmt.Errorf("failed to build ansible arguments: %w", err)
	}

	// Execute playbook with enhanced error handling
	LogLevel("info", "Executing Ansible playbook with args: %v", args)
	if err := runCmdWithTimeout(config.Ansible.Dir, 25*time.Minute, "ansible-playbook", args...); err != nil {
		LogLevel("error", "Ansible playbook execution failed. Check the following:")
		LogLevel("error", "1. SSH connectivity to nodes")
		LogLevel("error", "2. RKE2 service status on target nodes")
		LogLevel("error", "3. SELinux/firewall configuration")
		LogLevel("error", "4. CNI compatibility with the chosen configuration")
		return fmt.Errorf("ansible playbook failed: %w", err)
	}

	LogLevel("info", "Ansible playbook execution completed successfully")
	LogLevel("debug", "Completed ansible playbook execution")

	return nil
}

// copyAndUpdateVarsFile copies and updates vars.tfvars file with unique ID
func copyAndUpdateVarsFile(srcPath, dstPath, uniqueID string) error {
	if data, err := os.ReadFile(srcPath); err == nil {
		if err := os.WriteFile(dstPath, data, 0644); err != nil {
			return fmt.Errorf("failed to write vars.tfvars: %w", err)
		}
	} else {
		return fmt.Errorf("failed to read vars file: %w", err)
	}

	return updateVarsFileWithUniqueID(dstPath, uniqueID)
}

// buildTofuApplyArgs builds arguments for tofu apply command
func buildTofuApplyArgs(config *QAInfraConfig) []string {
	args := []string{"apply", "-auto-approve", "-var-file=" + config.TFVarsPath}

	// Add public SSH key if available
	if akf := strings.TrimSpace(os.Getenv("ACCESS_KEY_FILE")); akf != "" {
		pubSrc := akf + ".pub"
		pubDst := filepath.Join(config.NodeSource, "id_ssh.pub")
		if err := copyFile(pubSrc, pubDst); err == nil {
			args = append(args, "-var", fmt.Sprintf("public_ssh_key=%s", pubDst))
		} else {
			args = append(args, "-var", fmt.Sprintf("public_ssh_key=%s", pubSrc))
		}
	}

	return args
}

// setupAnsibleEnvironmentVars sets up environment variables for Ansible
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
	KubeConfigFile = config.KubeconfigPath

	LogLevel("debug", "Set environment variables for Ansible")

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
		LogLevel("debug", "Passing server flags to Ansible: %s", serverFlagsVal)
	}

	workerFlagsVal := normalize(os.Getenv("WORKER_FLAGS"))
	if workerFlagsVal != "" {
		args = append(args, "--extra-vars", fmt.Sprintf("worker_flags=%q", workerFlagsVal))
		LogLevel("debug", "Passing worker flags to Ansible: %s", workerFlagsVal)
	}

	return args
}

// getChannelValue gets the channel value from environment
func getChannelValue() string {
	return strings.TrimSpace(os.Getenv("CHANNEL"))
}

// runCmd executes a command with proper logging
func runCmd(dir, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()

	LogLevel("info", "Running: %s %v (in %s)", name, args, dir)
	return cmd.Run()
}

// runCmdWithTimeout executes a command with a specific timeout
func runCmdWithTimeout(dir string, timeout time.Duration, name string, args ...string) error {
	timeoutCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(timeoutCtx, name, args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()

	LogLevel("info", "Running with %v timeout: %s %v (in %s)", timeout, name, args, dir)
	err := cmd.Run()

	if timeoutCtx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("command timed out after %v: %s %v", timeout, name, args)
	}

	return err
}

func copyFile(src, dst string) error {
	in, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.WriteFile(dst, in, 0644); err != nil {
		return err
	}
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

	LogLevel("info", "Generated INI inventory with %d master nodes and %d worker nodes", len(masterNodes), len(workerNodes))
	LogLevel("debug", "INI Inventory content:\n%s", b.String())

	return invPath, nil
}

// preflightInventory runs `ansible-inventory --list` against a path to verify parsing
func preflightInventory(workingDir, inventoryPath string) error {
	LogLevel("info", "Validating inventory: %s", inventoryPath)
	cmd := exec.Command("ansible-inventory", "-i", inventoryPath, "--list")
	cmd.Dir = workingDir
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	output := string(out)

	// Only fail on actual errors, not warnings
	if err != nil {
		LogLevel("warn", "Inventory validation failed with error: %v\n%s", err, output)
		return fmt.Errorf("inventory validation error: %w", err)
	}

	// Check if the output contains valid JSON with hosts (successful parsing)
	if strings.Contains(output, `"_meta"`) &&
		(strings.Contains(output, `"hostvars"`) || strings.Contains(output, `"hosts"`)) {
		LogLevel("info", "Inventory validation successful")
		return nil
	}

	// If no valid hosts found, this indicates a parsing issue
	if strings.Contains(output, "No inventory was parsed") ||
		strings.Contains(output, "only implicit localhost") {
		LogLevel("warn", "Inventory validation failed - no valid hosts found:\n%s", output)
		return fmt.Errorf("no valid hosts found in inventory")
	}

	LogLevel("info", "Inventory validation passed with warnings (non-critical)")
	return nil
}

// updateVarsFileWithUniqueID updates the vars.tfvars file with unique resource names and replaces product variables
func updateVarsFileWithUniqueID(varsFilePath, uniqueID string) error {
	content, err := os.ReadFile(varsFilePath)
	if err != nil {
		return fmt.Errorf("failed to read vars file: %w", err)
	}

	varsContent := string(content)

	// Get the actual product value from environment
	product := strings.ToLower(strings.TrimSpace(os.Getenv("ENV_PRODUCT")))

	// todo get hostname prefix from env var if set
	resourceName := os.Getenv("RESOURCE_NAME")
	uniquePrefix := fmt.Sprintf("distros-qa-%s-%s-%s", resourceName, product, uniqueID)

	// Update aws_hostname_prefix.
	re := regexp.MustCompile(`aws_hostname_prefix\s*=\s*"[^"]*"`)
	varsContent = re.ReplaceAllString(varsContent, fmt.Sprintf(`aws_hostname_prefix = "%s"`, uniquePrefix))

	// Update user_id.
	userIdRe := regexp.MustCompile(`user_id\s*=\s*"[^"]*"`)
	varsContent = userIdRe.ReplaceAllString(varsContent, fmt.Sprintf(`user_id = "distros-qa-%s-%s"`, resourceName, product))

	LogLevel("debug", "Updated vars.tfvars: product=%s, uniquePrefix=%s", product, uniquePrefix)

	// Write back the updated content
	if err := os.WriteFile(varsFilePath, []byte(varsContent), 0644); err != nil {
		return fmt.Errorf("failed to write vars file: %w", err)
	}

	return nil
}

// DestroyInfrastructure destroys infrastructure using the configured provider
func DestroyInfrastructure(product, module string) (string, error) {
	provider := os.Getenv("INFRA_PROVIDER")
	switch provider {
	case "legacy", "":
		terraformOptions, _, err := setTerraformOptionsLegacy(product, module)
		if err != nil {
			return "", err
		}
		terraform.Destroy(&testing.T{}, terraformOptions)

		return "cluster destroyed", nil
	case "qa-infra":
		return destroyQAInfra()
	default:
		return "", fmt.Errorf("unknown infrastructure provider: %s", provider)
	}
}

// destroyQAInfra destroys qa-infra infrastructure
func destroyQAInfra() (string, error) {
	workspace := os.Getenv("TF_WORKSPACE")
	if workspace == "" {
		LogLevel("warn", "No workspace specified for qa-infra destroy")
		return "", nil
	}

	var rootDir string
	if isRunningInContainer() {
		LogLevel("info", "Detected container environment for qa-infra destroy")
		rootDir = "/go/src/github.com/rancher/distros-test-framework"
	} else {
		_, callerFilePath, _, _ := runtime.Caller(0)
		rootDir = filepath.Join(filepath.Dir(callerFilePath), "..")
	}

	nodeSource := os.Getenv("TERRAFORM_NODE_SOURCE")
	if nodeSource == "" {
		nodeSource = filepath.Join(rootDir, "infrastructure/qa-infra")
	}

	LogLevel("info", "Destroying qa-infra infrastructure...")
	if err := runCmd(nodeSource, "tofu", "workspace", "select", workspace); err != nil {
		return "", fmt.Errorf("tofu workspace select failed: %w", err)
	}

	if err := runCmd(nodeSource, "tofu", "destroy", "-auto-approve"); err != nil {
		return "", fmt.Errorf("tofu destroy failed: %w", err)
	}

	return "cluster destroyed", nil
}
