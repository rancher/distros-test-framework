package shared

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gruntwork-io/terratest/modules/terraform"
)

// InfraProvider defines which infrastructure provisioning method to use
type InfraProvider string

const (
	// LegacyProvider uses the existing modules/ (now infrastructure/legacy/) approach
	LegacyProvider InfraProvider = "legacy"
	// QAInfraProvider uses the qa-infra-automation approach (supports AWS, vSphere, etc.)
	QAInfraProvider InfraProvider = "qa-infra"
)

// InfraConfig holds configuration for infrastructure provisioning
type InfraConfig struct {
	Provider   InfraProvider
	Product    string
	Module     string
	Workspace  string
	TFVarsFile string
}

// getInfraProvider returns the infrastructure provider from environment or defaults to legacy
func getInfraProvider() InfraProvider {
	provider := os.Getenv("INFRA_PROVIDER")
	switch provider {
	case "qa-infra":
		return QAInfraProvider
	case "legacy":
		return LegacyProvider
	default:
		// Default to legacy for backward compatibility
		LogLevel("info", "No INFRA_PROVIDER set, defaulting to legacy")
		return LegacyProvider
	}
}

// ProvisionInfrastructure provisions infrastructure using the configured provider
func ProvisionInfrastructure(config InfraConfig) (*Cluster, error) {
	if config.Provider == "" {
		config.Provider = getInfraProvider()
	}

	switch config.Provider {
	case LegacyProvider:
		return provisionLegacy(config)
	case QAInfraProvider:
		return provisionQAInfra(config)
	default:
		return nil, fmt.Errorf("unknown infrastructure provider: %s", config.Provider)
	}
}

// DestroyInfrastructure destroys infrastructure using the configured provider
func DestroyInfrastructure(config InfraConfig) error {
	switch config.Provider {
	case LegacyProvider:
		return destroyLegacy(config)
	case QAInfraProvider:
		return destroyQAInfra(config)
	default:
		return fmt.Errorf("unknown infrastructure provider: %s", config.Provider)
	}
}

// provisionLegacy uses the existing terraform approach
func provisionLegacy(config InfraConfig) (*Cluster, error) {
	terraformOptions, varDir, err := setTerraformOptionsLegacy(config.Product, config.Module)
	if err != nil {
		return nil, err
	}
	t := &testing.T{}
	terraform.InitAndApply(t, terraformOptions)

	c := &Cluster{}
	cluster, err := loadTFconfig(t, c, config.Product, config.Module, varDir, terraformOptions)
	if err != nil {
		return nil, err
	}

	// Set the status for the cluster
	cluster.Status = "cluster created"
	LogLevel("debug", "Cluster has been created successfully...")

	return cluster, nil
}

// destroyLegacy destroys legacy terraform infrastructure
func destroyLegacy(config InfraConfig) error {
	terraformOptions, _, err := setTerraformOptionsLegacy(config.Product, config.Module)
	if err != nil {
		return err
	}

	terraform.Destroy(&testing.T{}, terraformOptions)
	return nil
}

// setTerraformOptionsLegacy is the updated version of setTerraformOptions for legacy path
func setTerraformOptionsLegacy(product, module string) (*terraform.Options, string, error) {
	_, callerFilePath, _, _ := runtime.Caller(0)
	dir := filepath.Join(filepath.Dir(callerFilePath), "..")

	varDir, err := filepath.Abs(dir +
		fmt.Sprintf("/config/%s.tfvars", product))
	LogLevel("info", "Using tfvars in: %v", varDir)
	if err != nil {
		return nil, "", fmt.Errorf("invalid product: %s", product)
	}

	// checking if module is empty, use the product as module
	if module == "" {
		module = product
	}

	tfDir, err := filepath.Abs(dir + "/infrastructure/legacy/" + module)
	LogLevel("info", "Using module dir: %v", tfDir)
	if err != nil {
		return nil, "", fmt.Errorf("no module found: %s", module)
	}

	terraformOptions := &terraform.Options{
		TerraformDir: tfDir,
		VarFiles:     []string{varDir},
	}

	return terraformOptions, varDir, nil
}

// provisionQAInfra uses the qa-infra-automation approach with remote modules
func provisionQAInfra(config InfraConfig) (*Cluster, error) {
	ctx := context.Background()

	// Set workspace if not provided
	if config.Workspace == "" {
		config.Workspace = fmt.Sprintf("dsf-%s", time.Now().Format("20060102150405"))
	}

	// Generate unique resource identifier
	uniqueID := time.Now().Format("0102-1504")

	// Set AWS credentials from environment
	if awsAccessKey := os.Getenv("AWS_ACCESS_KEY_ID"); awsAccessKey != "" {
		os.Setenv("AWS_ACCESS_KEY_ID", awsAccessKey)
	}
	if awsSecretKey := os.Getenv("AWS_SECRET_ACCESS_KEY"); awsSecretKey != "" {
		os.Setenv("AWS_SECRET_ACCESS_KEY", awsSecretKey)
	}

	// Get paths - check if running in container first
	var rootDir string
	if containerPath := os.Getenv("GOPATH"); containerPath != "" && strings.Contains(containerPath, "/go") {
		// Running in Docker container
		rootDir = "/go/src/github.com/rancher/distros-test-framework"
	} else {
		// Running locally
		_, callerFilePath, _, _ := runtime.Caller(0)
		rootDir = filepath.Join(filepath.Dir(callerFilePath), "..")
	}

	// Build a temp working dir that references the remote qa-infra module (no local wrapper)
	workRoot := filepath.Join(rootDir, "tmp")
	_ = os.MkdirAll(workRoot, 0o755)
	nodeSource := filepath.Join(workRoot, fmt.Sprintf("qa-infra-tofu-%s", config.Workspace))
	tempDir := filepath.Join(rootDir, "tmp/qa-infra-ansible")
	if err := os.MkdirAll(nodeSource, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create tofu workdir: %w", err)
	}

	// Public/private keys are provided via mounts from .env (ACCESS_KEY_LOCAL and .pub). No generation here.

	// Copy existing main.tf from infrastructure/qa-infra/
	mainTfSrc := filepath.Join(rootDir, "infrastructure/qa-infra/main.tf")
	mainTfDst := filepath.Join(nodeSource, "main.tf")
	if err := copyFile(mainTfSrc, mainTfDst); err != nil {
		return nil, fmt.Errorf("failed to copy main.tf: %w", err)
	}

	// Update main.tf to use the correct infrastructure module (AWS vs vSphere)
	if err := updateMainTfModuleSource(mainTfDst); err != nil {
		return nil, fmt.Errorf("failed to update main.tf module source: %w", err)
	}

	// Copy variable schema and values into temp workdir (schema: variables.tf, values: vars.tfvars)
	if data, err := os.ReadFile(filepath.Join(rootDir, "infrastructure/qa-infra/variables.tf")); err == nil {
		_ = os.WriteFile(filepath.Join(nodeSource, "variables.tf"), data, 0644)
	}

	tfvarsSrc := config.TFVarsFile
	if tfvarsSrc == "" {
		tfvarsSrc = filepath.Join(rootDir, "infrastructure/qa-infra/vars.tfvars")
	}
	tfvarsDst := filepath.Join(nodeSource, "vars.tfvars")
	if b, err := os.ReadFile(tfvarsSrc); err == nil {
		if err := os.WriteFile(tfvarsDst, b, 0644); err != nil {
			return nil, fmt.Errorf("failed to write vars.tfvars: %w", err)
		}
	} else {
		return nil, fmt.Errorf("failed to read vars file: %w", err)
	}

	// Provision with OpenTofu using remote module
	LogLevel("info", "Provisioning infrastructure with qa-infra remote modules...")
	if err := runCmdWithTimeout(ctx, nodeSource, 5*time.Minute, "tofu", "init"); err != nil {
		return nil, fmt.Errorf("tofu init failed: %w", err)
	}

	// Create workspace (ignore error if exists)
	_ = runCmd(ctx, nodeSource, "tofu", "workspace", "new", config.Workspace)

	if err := runCmd(ctx, nodeSource, "tofu", "workspace", "select", config.Workspace); err != nil {
		return nil, fmt.Errorf("tofu workspace select failed: %w", err)
	}

	// Always use the temp workdir vars.tfvars
	tfvarsPath := tfvarsDst
	LogLevel("info", "Using tfvars file: %s", tfvarsPath)

	// Update vars.tfvars with unique hostname prefix
	if err := updateVarsFileWithUniqueID(tfvarsPath, uniqueID); err != nil {
		return nil, fmt.Errorf("failed to update vars file: %w", err)
	}

	// Build tofu apply args, optionally overriding public_ssh_key from env
	tofuArgs := []string{"apply", "-auto-approve", "-var-file=" + tfvarsPath}
	if akf := strings.TrimSpace(os.Getenv("ACCESS_KEY_FILE")); akf != "" {
		pubSrc := akf + ".pub"
		pubDst := filepath.Join(nodeSource, "id_ssh.pub")
		if err := copyFile(pubSrc, pubDst); err == nil {
			tofuArgs = append(tofuArgs, "-var", fmt.Sprintf("public_ssh_key=%s", pubDst))
		} else {
			// fallback to source path if copy fails
			tofuArgs = append(tofuArgs, "-var", fmt.Sprintf("public_ssh_key=%s", pubSrc))
		}
	}
	if err := runCmdWithTimeout(ctx, nodeSource, 15*time.Minute, "tofu", tofuArgs...); err != nil {
		return nil, fmt.Errorf("tofu apply failed: %w", err)
	}

	// Clone ansible playbooks temporarily for installation
	LogLevel("info", "Downloading Ansible playbooks for %s installation...", config.Product)
	if err := os.MkdirAll(tempDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}

	var ansiblePath string
	switch config.Product {
	case "k3s":
		ansiblePath = "ansible/k3s/default"
		LogLevel("info", "Using K3s ansible path")
	default:
		ansiblePath = "ansible/rke2/default"
		LogLevel("info", "Using RKE2 ansible path")
	}

	// Use add.vsphere branch from fmoral2 fork for all products
	// Clone only the ansible directory we need from the appropriate repository and branch
	gitBranch := "add.vsphere"
	repoURL := "https://github.com/fmoral2/qa-infra-automation.git"
	if err := runCmd(ctx, rootDir, "git", "clone", "--depth", "1", "--filter=blob:none", "--sparse", "--branch",
		gitBranch, repoURL, tempDir); err != nil {
		return nil, fmt.Errorf("git clone failed: %w", err)
	}

	// Set up sparse checkout for ansible directory only
	if err := runCmd(ctx, tempDir, "git", "sparse-checkout", "set", ansiblePath); err != nil {
		return nil, fmt.Errorf("sparse checkout failed: %w", err)
	}

	ansibleDir := filepath.Join(tempDir, ansiblePath)

	// Set up environment variables for ansible (kubeconfig will be in ansibleDir)
	kubeconfigPath := filepath.Join(ansibleDir, "kubeconfig.yaml")
	os.Setenv("TERRAFORM_NODE_SOURCE", nodeSource)
	os.Setenv("ANSIBLE_CONFIG", filepath.Join(ansibleDir, "ansible.cfg"))
	os.Setenv("REPO_ROOT", tempDir)
	os.Setenv("WORKSPACE_NAME", config.Workspace)
	os.Setenv("TFVARS_FILE", "vars.tfvars")
	os.Setenv("KUBECONFIG_FILE", kubeconfigPath)

	// Set the correct state file path for the workspace
	// OpenTofu stores workspace state in terraform.tfstate.d/{workspace}/terraform.tfstate
	workspaceStateFile := filepath.Join(nodeSource, "terraform.tfstate.d", config.Workspace, "terraform.tfstate")
	os.Setenv("TERRAFORM_STATE_FILE", workspaceStateFile)

	// Also set the alternative paths that Ansible configurations expect
	os.Setenv("TF_STATE_FILE", workspaceStateFile)
	os.Setenv("TERRAFORM_WORKSPACE", config.Workspace)

	// Note: No longer need terraform_state_file env var since we generate inventory dynamically

	// Generate inventory and run Ansible
	LogLevel("info", "Installing %s with Ansible...", config.Product)

	// Verify the state file exists and log its location
	stateFile := filepath.Join(nodeSource, "terraform.tfstate.d", config.Workspace, "terraform.tfstate")
	if _, err := os.Stat(stateFile); err != nil {
		LogLevel("info", "State file not found at %s, checking alternate locations", stateFile)
		// Try the main terraform.tfstate location (non-workspace)
		altStateFile := filepath.Join(nodeSource, "terraform.tfstate")
		if _, err := os.Stat(altStateFile); err == nil {
			os.Setenv("TERRAFORM_STATE_FILE", altStateFile)
			os.Setenv("TF_STATE_FILE", altStateFile)
			LogLevel("info", "Using main state file: %s", altStateFile)
		}
	} else {
		LogLevel("info", "Using workspace state file: %s", stateFile)
	}

	// Patch the playbook to use our inventory variables instead of terraform lookups
	if err := patchPlaybook(ansibleDir, config.Product); err != nil {
		return nil, fmt.Errorf("failed to patch playbook: %w", err)
	}

	// Get OpenTofu outputs and prepare basic cluster config for inventory creation
	outputs, err := getOpenTofuOutputs(ctx, nodeSource)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster outputs: %w", err)
	}

	// Create a minimal cluster object for SSH configuration
	tempCluster := &Cluster{
		Aws: AwsConfig{
			EC2: EC2{},
		},
	}

	// Load SSH configuration for inventory
	if err := loadQAInfraConfig(nodeSource, tempCluster, config.Product); err != nil {
		LogLevel("debug", "Failed to load SSH config from tfvars: %v", err)
	}

	// Set SSH configuration from environment variables if not already set
	if tempCluster.Aws.EC2.AccessKey == "" {
		if accessKeyFile := os.Getenv("ACCESS_KEY_FILE"); accessKeyFile != "" {
			tempCluster.Aws.EC2.AccessKey = accessKeyFile
		}
	}

	// TODO: THIS SHOULD COME FROM VARS.TFVARS NOT ENV.
	if tempCluster.Aws.EC2.AwsUser == "" {
		if sshUser := os.Getenv("AWS_SSH_USER"); sshUser != "" {
			tempCluster.Aws.EC2.AwsUser = sshUser
		} else {
			tempCluster.Aws.EC2.AwsUser = "ec2-user"
		}
	}

	// Get node information from OpenTofu state for direct host list
	nodes, err := getAllNodesFromState(ctx, nodeSource)
	if err != nil {
		return nil, fmt.Errorf("failed to get node information from state: %w", err)
	}

	LogLevel("debug", "NODES FROM getAllNodesFromState() ===  STATE: %v", nodes)

	// Extract workspace name from the state file path
	stateFilePattern := regexp.MustCompile(`terraform\.tfstate\.d/([^/]+)/terraform\.tfstate`)
	matches := stateFilePattern.FindStringSubmatch(stateFile)
	var workspace string
	if len(matches) > 1 {
		workspace = matches[1]
	} else {
		workspace = "default"
	}

	// Set environment variables for Ansible terraform lookups
	os.Setenv("TF_WORKSPACE", workspace)
	os.Setenv("TERRAFORM_NODE_SOURCE", nodeSource)
	LogLevel("info", "Set TF_WORKSPACE=%s, TERRAFORM_NODE_SOURCE=%s", workspace, nodeSource)

	var playbookName string
	switch config.Product {
	case "k3s":
		playbookName = "k3s-playbook.yml"
		LogLevel("info", "Selected K3s playbook: %s", playbookName)
	default:
		playbookName = "rke2-playbook.yml"
		LogLevel("info", "Selected RKE2 playbook: %s", playbookName)
	}

	// Create temporary inventory and validate with ansible-inventory, fallback between YAML and INI
	inventoryPath, err := writeYAMLInventory(ansibleDir, nodes, tempCluster)
	if err != nil {
		return nil, fmt.Errorf("failed to write YAML inventory file: %w", err)
	}

	if err := preflightInventory(ctx, ansibleDir, inventoryPath); err != nil {
		LogLevel("warn", "YAML inventory validation failed, falling back to INI: %v", err)
		if iniPath, iniErr := writeINIInventory(ansibleDir, nodes, tempCluster); iniErr == nil {
			if preErr := preflightInventory(ctx, ansibleDir, iniPath); preErr == nil {
				inventoryPath = iniPath
			} else {
				return nil, fmt.Errorf("both YAML and INI inventory parsing failed: %w", preErr)
			}
		} else {
			return nil, fmt.Errorf("failed writing INI inventory: %w", iniErr)
		}
	}
	LogLevel("info", "Using inventory file: %s", inventoryPath)

	// Build ansible-playbook args: env-driven versions and kubeconfig
	installVersion := strings.TrimSpace(os.Getenv("INSTALL_VERSION"))
	kubeconfigPath = filepath.Join(ansibleDir, "kubeconfig.yaml")
	os.Setenv("KUBECONFIG", kubeconfigPath)
	KubeConfigFile = kubeconfigPath

	// Set architecture for workload tests
	if tempCluster.Config.Arch != "" {
		os.Setenv("arch", tempCluster.Config.Arch)
	} else {
		os.Setenv("arch", "amd64") // Default to amd64
	}

	// Use absolute path for kubeconfig to avoid path resolution issues
	LogLevel("debug", "Passing kubeconfig path to Ansible: %s", kubeconfigPath)
	args := []string{"-i", inventoryPath, playbookName,
		"--extra-vars", fmt.Sprintf("kubeconfig_file=%s", kubeconfigPath),
	}
	if installVersion != "" {
		args = append(args, "--extra-vars", fmt.Sprintf("kubernetes_version=%s", installVersion))

	} else {
		return nil, fmt.Errorf("INSTALL_VERSION is not set")
	}

	normalize := func(s string) string {
		s = strings.TrimSpace(s)
		// Convert escaped newlines to real newlines
		s = strings.ReplaceAll(s, "\\n", "\n")
		return s
	}

	serverFlagsVal := normalize(os.Getenv("SERVER_FLAGS"))
	if serverFlagsVal == "" {
		serverFlagsVal = normalize(tempCluster.Config.ServerFlags)
	}

	workerFlagsVal := normalize(os.Getenv("WORKER_FLAGS"))
	if workerFlagsVal == "" {
		workerFlagsVal = normalize(tempCluster.Config.WorkerFlags)
	}

	// Pass server and worker flags separately with proper quoting for multi-line values
	if serverFlagsVal != "" {
		args = append(args, "--extra-vars", fmt.Sprintf("server_flags=%q", serverFlagsVal))
		LogLevel("debug", "Passing server flags to Ansible: %s", serverFlagsVal)
	}
	if workerFlagsVal != "" {
		args = append(args, "--extra-vars", fmt.Sprintf("worker_flags=%q", workerFlagsVal))
		LogLevel("debug", "Passing worker flags to Ansible: %s", workerFlagsVal)
	}

	// Pass channel if specified (for K3s/RKE2 release channel)
	channelVal := strings.TrimSpace(os.Getenv("CHANNEL"))
	if channelVal == "" {
		channelVal = strings.TrimSpace(tempCluster.Config.Channel)
	}
	if channelVal != "" {
		args = append(args, "--extra-vars", fmt.Sprintf("channel=%s", channelVal))
		LogLevel("debug", "Passing channel to Ansible: %s", channelVal)
	}

	installMethod := strings.TrimSpace(os.Getenv("INSTALL_METHOD"))
	if strings.Contains(config.Product, "rke2") {
		// For RKE2, prefer tempCluster config if set, otherwise use environment variable
		if tempCluster.Config.InstallMethod != "" {
			installMethod = strings.TrimSpace(tempCluster.Config.InstallMethod)
		}
	}
	if installMethod != "" {
		args = append(args, "--extra-vars", fmt.Sprintf("install_method=%s", installMethod))
		LogLevel("debug", "Passing install method to Ansible: %s", installMethod)
	}

	// For RKE2, extract CNI from server_flags and pass it as a separate variable
	if strings.Contains(config.Product, "rke2") {
		var cniValue string

		// Check if CNI environment variable is explicitly set and not empty
		envCNI := os.Getenv("CNI")
		if envCNI != "" {
			cniValue = strings.TrimSpace(envCNI)
			LogLevel("debug", "Using CNI from environment variable: %s", cniValue)
		} else {

			cniValue = extractCNIFromFlags(serverFlagsVal)
			if cniValue != "" {
				LogLevel("debug", "Extracted CNI from server_flags: %s", cniValue)
			} else {
				cniValue = "canal"
				LogLevel("debug", "Using default CNI for RKE2: %s", cniValue)
			}
		}

		args = append(args, "--extra-vars", fmt.Sprintf("cni=%s", cniValue))
		LogLevel("debug", "Passing CNI to RKE2 Ansible: %s", cniValue)
	}

	// Run ansible-playbook with timeout
	if err := runCmdWithTimeout(ctx, ansibleDir, 20*time.Minute, "ansible-playbook", args...); err != nil {
		return nil, fmt.Errorf("ansible playbook failed: %w", err)
	}

	// Calculate node counts from the nodes configuration
	numServers, numAgents := calculateNodeCounts(nodeSource)

	// Get actual node IPs from Terraform state
	nodes, err = getAllNodesFromState(ctx, nodeSource)
	if err != nil {
		return nil, fmt.Errorf("failed to get node IPs from state: %w", err)
	}

	// Separate server and agent IPs based on node roles
	var serverIPs, agentIPs []string
	for _, node := range nodes {
		if strings.Contains(node.Role, "etcd") || strings.Contains(node.Role, "cp") || strings.Contains(node.Role, "control-plane") {
			serverIPs = append(serverIPs, node.PublicIP)
		} else if strings.Contains(node.Role, "worker") && !strings.Contains(node.Role, "etcd") && !strings.Contains(node.Role, "cp") {
			agentIPs = append(agentIPs, node.PublicIP)
		}
	}

	// Create comprehensive cluster config matching legacy provider
	c := &Cluster{
		Status:     "cluster created",
		ServerIPs:  serverIPs,
		AgentIPs:   agentIPs,
		NumServers: numServers,
		NumAgents:  numAgents,
		FQDN:       outputs.FQDN,
		Aws: AwsConfig{
			EC2: EC2{},
		},
	}

	// Load configuration from vars.tfvars to match legacy provider
	// LogLevel("debug", "Loading qa-infra configuration from %s", nodeSource)
	if err := loadQAInfraConfig(nodeSource, c, config.Product); err != nil {
		LogLevel("warn", "Failed to load complete configuration: %v", err)
	}

	// Copy SSH configuration from tempCluster
	c.Aws.EC2.AccessKey = tempCluster.Aws.EC2.AccessKey
	c.Aws.EC2.AwsUser = tempCluster.Aws.EC2.AwsUser

	// Set the global cluster variable to prevent legacy ClusterConfig from being called
	SetGlobalCluster(c)

	LogLevel("info", "Infrastructure provisioned successfully with qa-infra remote modules")
	LogLevel("info", "Kubeconfig available at: %s", kubeconfigPath)
	LogLevel("info", "Ansible playbooks downloaded to: %s", tempDir)

	return c, nil
}

// extractCNIFromFlags extracts the CNI value from YAML-formatted server flags
func extractCNIFromFlags(serverFlags string) string {
	if serverFlags == "" {
		return ""
	}

	// Parse YAML-style server flags to extract CNI
	lines := strings.Split(serverFlags, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "cni:") {
			// Extract value after "cni:"
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				cni := strings.TrimSpace(parts[1])
				// Remove quotes if present
				cni = strings.Trim(cni, `"'`)
				return cni
			}
		}
	}

	return ""
}

// destroyQAInfra destroys qa-infra infrastructure
func destroyQAInfra(config InfraConfig) error {
	ctx := context.Background()

	workspace := config.Workspace
	if workspace == "" {
		workspace = os.Getenv("TF_WORKSPACE")
	}
	if workspace == "" {
		LogLevel("warn", "No workspace specified for qa-infra destroy")
		return nil
	}

	// Get paths - check if running in container first
	var rootDir string
	if containerPath := os.Getenv("GOPATH"); containerPath != "" && strings.Contains(containerPath, "/go") {
		// Running in Docker container
		rootDir = "/go/src/github.com/rancher/distros-test-framework"
	} else {
		// Running locally
		_, callerFilePath, _, _ := runtime.Caller(0)
		rootDir = filepath.Join(filepath.Dir(callerFilePath), "..")
	}

	nodeSource := os.Getenv("TERRAFORM_NODE_SOURCE")
	if nodeSource == "" {
		nodeSource = filepath.Join(rootDir, "infrastructure/qa-infra")
	}

	os.Setenv("TF_WORKSPACE", workspace)

	LogLevel("info", "Destroying qa-infra infrastructure...")
	if err := runCmd(ctx, nodeSource, "tofu", "workspace", "select", workspace); err != nil {
		return fmt.Errorf("tofu workspace select failed: %w", err)
	}

	if err := runCmd(ctx, nodeSource, "tofu", "destroy", "-auto-approve"); err != nil {
		return fmt.Errorf("tofu destroy failed: %w", err)
	}

	return nil
}

// runCmd executes a command with proper logging
func runCmd(ctx context.Context, dir, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()

	LogLevel("info", "Running: %s %v (in %s)", name, args, dir)
	return cmd.Run()
}

// runCmdWithTimeout executes a command with a specific timeout
func runCmdWithTimeout(ctx context.Context, dir string, timeout time.Duration, name string, args ...string) error {
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
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

// writeYAMLInventory creates a YAML inventory file with proper groups for Ansible
// structure:
// master:
//
//	hosts:
//	  <master-node>:
//	    ansible_host: <ip>
//	    ansible_role: <role>
//
// worker:
//
//	hosts:
//	  <worker-node>:
//	    ansible_host: <ip>
//	    ansible_role: <role>
//
// all:
//
//	children:
//	  master:
//	  worker:
//	vars:
//	  ansible_user: <user>
//	  ansible_ssh_private_key_file: <key>
//	  ansible_ssh_common_args: "-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null"
func writeYAMLInventory(ansibleDir string, nodes []InfraNode, cluster *Cluster) (string, error) {
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
		b.WriteString("master:\n")
		b.WriteString("  hosts:\n")
		for _, n := range masterNodes {
			b.WriteString(fmt.Sprintf("    %s:\n", n.Name))
			b.WriteString(fmt.Sprintf("      ansible_host: %s\n", n.PublicIP))
			if n.Role != "" {
				b.WriteString(fmt.Sprintf("      ansible_role: %s\n", n.Role))
			}
		}
		b.WriteString("\n")
	}

	// Write worker group
	if len(workerNodes) > 0 {
		b.WriteString("worker:\n")
		b.WriteString("  hosts:\n")
		for _, n := range workerNodes {
			b.WriteString(fmt.Sprintf("    %s:\n", n.Name))
			b.WriteString(fmt.Sprintf("      ansible_host: %s\n", n.PublicIP))
			if n.Role != "" {
				b.WriteString(fmt.Sprintf("      ansible_role: %s\n", n.Role))
			}
		}
		b.WriteString("\n")
	}

	// Write all group that includes children
	b.WriteString("all:\n")
	if len(masterNodes) > 0 || len(workerNodes) > 0 {
		b.WriteString("  children:\n")
		if len(masterNodes) > 0 {
			b.WriteString("    master:\n")
		}
		if len(workerNodes) > 0 {
			b.WriteString("    worker:\n")
		}
	}

	// Write common variables
	b.WriteString("  vars:\n")
	user := cluster.Aws.EC2.AwsUser
	if user == "" {
		user = "ec2-user"
	}

	key := cluster.Aws.EC2.AccessKey
	b.WriteString(fmt.Sprintf("    ansible_user: %s\n", user))
	if key != "" {
		b.WriteString(fmt.Sprintf("    ansible_ssh_private_key_file: %s\n", key))
	}
	b.WriteString("    ansible_ssh_common_args: \"-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null\"\n")

	invPath := filepath.Join(ansibleDir, "terraform-inventory.yml")
	if err := os.WriteFile(invPath, []byte(b.String()), 0644); err != nil {
		return "", err
	}

	LogLevel("info", "Generated inventory with %d master nodes and %d worker nodes", len(masterNodes), len(workerNodes))
	LogLevel("debug", "Inventory content:\n%s", b.String())

	return invPath, nil
}

// writeINIInventory creates an INI inventory file with proper groups as a fallback
func writeINIInventory(ansibleDir string, nodes []InfraNode, cluster *Cluster) (string, error) {
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
	user := cluster.Aws.EC2.AwsUser
	if user == "" {
		user = "ec2-user"
	}
	key := cluster.Aws.EC2.AccessKey
	b.WriteString(fmt.Sprintf("ansible_user=%s\n", user))
	if key != "" {
		b.WriteString(fmt.Sprintf("ansible_ssh_private_key_file=%s\n", key))
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
func preflightInventory(ctx context.Context, workingDir, inventoryPath string) error {
	LogLevel("info", "Validating inventory: %s", inventoryPath)
	cmd := exec.CommandContext(ctx, "ansible-inventory", "-i", inventoryPath, "--list")
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
	if product == "" {
		product = "k3s" // Default fallback
	}

	// Generate unique hostname prefix
	uniquePrefix := fmt.Sprintf("fmoral-qa-%s-%s", uniqueID, product)

	// Update aws_hostname_prefix with unique ID
	re := regexp.MustCompile(`aws_hostname_prefix\s*=\s*"[^"]*"`)
	varsContent = re.ReplaceAllString(varsContent, fmt.Sprintf(`aws_hostname_prefix = "%s"`, uniquePrefix))

	// Update user_id to include product
	userIdRe := regexp.MustCompile(`user_id\s*=\s*"[^"]*"`)
	varsContent = userIdRe.ReplaceAllString(varsContent, fmt.Sprintf(`user_id = "distros-qa-fmoral-%s"`, product))

	LogLevel("debug", "Updated vars.tfvars: product=%s, uniquePrefix=%s", product, uniquePrefix)

	// Write back the updated content
	if err := os.WriteFile(varsFilePath, []byte(varsContent), 0644); err != nil {
		return fmt.Errorf("failed to write vars file: %w", err)
	}

	return nil
}

// TofuOutputs represents the outputs from OpenTofu
type TofuOutputs struct {
	KubeAPIHost string
	FQDN        string
}

// InfraNode represents a cluster node from infrastructure
type InfraNode struct {
	Name     string
	PublicIP string
	Role     string
}

func getOpenTofuOutputs(ctx context.Context, nodeSource string) (*TofuOutputs, error) {
	kubeAPIHostCmd := exec.CommandContext(ctx, "tofu", "output", "-raw", "kube_api_host")
	kubeAPIHostCmd.Dir = nodeSource
	kubeAPIHostOutput, err := kubeAPIHostCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get kube_api_host output: %w", err)
	}

	fqdnCmd := exec.CommandContext(ctx, "tofu", "output", "-raw", "fqdn")
	fqdnCmd.Dir = nodeSource
	fqdnOutput, err := fqdnCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get fqdn output: %w", err)
	}

	return &TofuOutputs{
		KubeAPIHost: strings.TrimSpace(string(kubeAPIHostOutput)),
		FQDN:        strings.TrimSpace(string(fqdnOutput)),
	}, nil
}

// getAllNodesFromState extracts all node information from OpenTofu state
func getAllNodesFromState(ctx context.Context, nodeSource string) ([]InfraNode, error) {
	stateCmd := exec.CommandContext(ctx, "tofu", "show", "-json")
	stateCmd.Dir = nodeSource
	stateOutput, err := stateCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get tofu state: %w", err)
	}

	// Parse the JSON state to extract node information
	var state map[string]interface{}
	if err := json.Unmarshal(stateOutput, &state); err != nil {
		return nil, fmt.Errorf("failed to parse state JSON: %w", err)
	}

	// Extract nodes from state
	var nodes []InfraNode

	// Navigate through the state structure to find instances
	if values, ok := state["values"].(map[string]interface{}); ok {
		if rootModule, ok := values["root_module"].(map[string]interface{}); ok {
			if childModules, ok := rootModule["child_modules"].([]interface{}); ok {
				for _, moduleInterface := range childModules {
					if module, ok := moduleInterface.(map[string]interface{}); ok {
						if resources, ok := module["resources"].([]interface{}); ok {
							for _, resourceInterface := range resources {
								if resource, ok := resourceInterface.(map[string]interface{}); ok {
									if resourceType, ok := resource["type"].(string); ok && resourceType == "aws_instance" {
										if values, ok := resource["values"].(map[string]interface{}); ok {
											// Extract node information
											node := InfraNode{}

											// Get the node name from tags
											if tags, ok := values["tags"].(map[string]interface{}); ok {
												if name, ok := tags["Name"].(string); ok {
													// Convert "tf-fmoral-master" to "master"
													nameParts := strings.Split(name, "-")
													if len(nameParts) > 2 {
														node.Name = strings.Join(nameParts[2:], "-")
													} else {
														node.Name = name
													}
												}
											}

											// Get public IP
											if publicIP, ok := values["public_ip"].(string); ok {
												node.PublicIP = publicIP
											}

											// Determine role based on name
											if strings.Contains(node.Name, "master") {
												node.Role = "etcd,cp,worker"
											} else if strings.Contains(node.Name, "worker") {
												node.Role = "worker"
											}

											if node.Name != "" && node.PublicIP != "" && node.Role != "" {
												nodes = append(nodes, node)
											}
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}

	if len(nodes) == 0 {
		return nil, fmt.Errorf("no nodes found in state")
	}

	LogLevel("info", "Found %d nodes in OpenTofu state", len(nodes))
	for _, node := range nodes {
		LogLevel("info", "Node: %s (%s) - Role: %s", node.Name, node.PublicIP, node.Role)
	}

	return nodes, nil
}

// patchPlaybook checks if the product playbook needs patching for missing environment variables
func patchPlaybook(ansibleDir, product string) error {
	// Determine the correct playbook filename based on the product
	var playbookFilename string
	switch product {
	case "k3s":
		playbookFilename = "k3s-playbook.yml"
	case "rke2":
		playbookFilename = "rke2-playbook.yml"
	default:
		// Default to RKE2 for backward compatibility
		playbookFilename = "rke2-playbook.yml"
		LogLevel("warn", "Unknown product '%s', defaulting to RKE2 playbook", product)
	}

	playbookPath := filepath.Join(ansibleDir, playbookFilename)
	LogLevel("debug", "Checking playbook for patching: %s (product: %s)", playbookPath, product)

	// Read the playbook file
	content, err := os.ReadFile(playbookPath)
	if err != nil {
		LogLevel("debug", "Could not read playbook for patching: %v", err)
		return nil // Don't fail if we can't patch
	}

	contentStr := string(content)

	LogLevel("debug", "Content of playbook: %s", contentStr)

	// Check if the required variables are already present in the upstream playbook
	hasServerFlags := strings.Contains(contentStr, "SERVER_FLAGS")
	hasWorkerFlags := strings.Contains(contentStr, "WORKER_FLAGS")
	hasChannel := strings.Contains(contentStr, "CHANNEL")
	hasInstallMethod := strings.Contains(contentStr, "INSTALL_METHOD")

	// For RKE2, also check INSTALL_METHOD
	requiredVarsPresent := hasServerFlags && hasWorkerFlags
	if product == "rke2" {
		requiredVarsPresent = requiredVarsPresent && hasInstallMethod
	}

	if requiredVarsPresent {
		LogLevel("info", "Playbook already contains required environment variables (SERVER_FLAGS, WORKER_FLAGS), no patching needed")
		if hasChannel {
			LogLevel("info", "Playbook also includes CHANNEL variable support")
		}
		if product == "rke2" && hasInstallMethod {
			LogLevel("info", "Playbook also includes INSTALL_METHOD variable support for RKE2")
		}
		return nil
	}

	if hasServerFlags {
		LogLevel("info", "Playbook contains SERVER_FLAGS but missing WORKER_FLAGS/CHANNEL - using upstream version as-is")
		return nil
	}

	// Add missing environment variables to the environment section for master hosts
	// Find the environment section and add SERVER_FLAGS, WORKER_FLAGS, CHANNEL, and INSTALL_METHOD (for RKE2)
	var envVarsToAdd string
	if product == "rke2" {
		envVarsToAdd = `      environment:
        KUBERNETES_VERSION: "{{ kubernetes_version }}"
        KUBE_API_HOST: "{{ kube_api_host }}"
        FQDN: "{{ fqdn }}"
        NODE_ROLE: "{{ ansible_role }}"
        SERVER_FLAGS: "{{ server_flags | default('') }}"
        WORKER_FLAGS: "{{ worker_flags | default('') }}"
        CHANNEL: "{{ channel | default('') }}"
        INSTALL_METHOD: "{{ install_method | default('') }}"`
	} else {
		envVarsToAdd = `      environment:
        KUBERNETES_VERSION: "{{ kubernetes_version }}"
        KUBE_API_HOST: "{{ kube_api_host }}"
        FQDN: "{{ fqdn }}"
        NODE_ROLE: "{{ ansible_role }}"
        SERVER_FLAGS: "{{ server_flags | default('') }}"
        WORKER_FLAGS: "{{ worker_flags | default('') }}"
        CHANNEL: "{{ channel | default('') }}"`
	}

	patchedContent := strings.ReplaceAll(contentStr,
		`      environment:
        KUBERNETES_VERSION: "{{ kubernetes_version }}"
        KUBE_API_HOST: "{{ kube_api_host }}"
        FQDN: "{{ fqdn }}"
        NODE_ROLE: "{{ ansible_role }}"`,
		envVarsToAdd)

	// Also patch the "all" hosts section for init-server.sh
	var allHostsEnvVars string
	if product == "rke2" {
		allHostsEnvVars = `      environment:
        KUBERNETES_VERSION: "{{ kubernetes_version }}"
        KUBE_API_HOST: "{{ kube_api_host }}"
        FQDN: "{{ fqdn }}"
        NODE_TOKEN: "{{ node_token }}"
        NODE_ROLE: "{{ ansible_role }}"
        SERVER_FLAGS: "{{ server_flags | default('') }}"
        WORKER_FLAGS: "{{ worker_flags | default('') }}"
        CHANNEL: "{{ channel | default('') }}"
        INSTALL_METHOD: "{{ install_method | default('') }}"`
	} else {
		allHostsEnvVars = `      environment:
        KUBERNETES_VERSION: "{{ kubernetes_version }}"
        KUBE_API_HOST: "{{ kube_api_host }}"
        FQDN: "{{ fqdn }}"
        NODE_TOKEN: "{{ node_token }}"
        NODE_ROLE: "{{ ansible_role }}"
        SERVER_FLAGS: "{{ server_flags | default('') }}"
        WORKER_FLAGS: "{{ worker_flags | default('') }}"
        CHANNEL: "{{ channel | default('') }}"`
	}

	patchedContent = strings.ReplaceAll(patchedContent,
		`      environment:
        KUBERNETES_VERSION: "{{ kubernetes_version }}"
        KUBE_API_HOST: "{{ kube_api_host }}"
        FQDN: "{{ fqdn }}"
        NODE_TOKEN: "{{ node_token }}"
        NODE_ROLE: "{{ ansible_role }}"`,
		allHostsEnvVars)

	// Write the patched content back
	if patchedContent != contentStr {
		if err := os.WriteFile(playbookPath, []byte(patchedContent), 0644); err != nil {
			LogLevel("warn", "Failed to write patched playbook: %v", err)
			return nil // Don't fail the entire process
		}
		if product == "rke2" {
			LogLevel("info", "Applied legacy patch to %s playbook (SERVER_FLAGS, WORKER_FLAGS, CHANNEL, INSTALL_METHOD added)", product)
		} else {
			LogLevel("info", "Applied legacy patch to %s playbook (SERVER_FLAGS, WORKER_FLAGS, CHANNEL added)", product)
		}
	}

	return nil
}

// calculateNodeCounts reads the vars.tfvars file and calculates node counts
func calculateNodeCounts(nodeSource string) (int, int) {
	varsFile := filepath.Join(nodeSource, "vars.tfvars")
	content, err := os.ReadFile(varsFile)
	if err != nil {
		LogLevel("warn", "Failed to read vars.tfvars, using default counts: %v", err)
		return 1, 0 // Default fallback
	}

	numServers := 0
	numAgents := 0

	// Parse the nodes configuration from vars.tfvars
	lines := strings.Split(string(content), "\n")
	inNodesBlock := false

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Start of nodes block
		if strings.HasPrefix(line, "nodes = [") {
			inNodesBlock = true
			continue
		}

		// End of nodes block
		if inNodesBlock && strings.Contains(line, "]") && !strings.Contains(line, "role") {
			break
		}

		// Parse individual node configurations - look for role in same block
		if inNodesBlock && strings.Contains(line, "role") {
			// Extract role array to determine node type
			roleMatch := regexp.MustCompile(`role\s*=\s*\[(.*?)\]`).FindStringSubmatch(line)
			if len(roleMatch) > 1 {
				roleStr := roleMatch[1]
				isServer := strings.Contains(roleStr, "etcd") || strings.Contains(roleStr, "cp")

				// Look backward for the count in this same block
				currentCount := 1 // Default count
				for i := len(lines) - 1; i >= 0; i-- {
					if strings.Contains(lines[i], line) { // Find current role line
						// Look backward for count in same block
						for j := i - 1; j >= 0 && j > i-5; j-- {
							if strings.Contains(lines[j], "count =") {
								if countMatch := regexp.MustCompile(`count\s*=\s*(\d+)`).FindStringSubmatch(lines[j]); len(countMatch) > 1 {
									if c, err := strconv.Atoi(countMatch[1]); err == nil {
										currentCount = c
									}
								}
								break
							}
							// Stop if we hit another block or the start of nodes
							if strings.Contains(lines[j], "{") || strings.Contains(lines[j], "nodes = [") {
								break
							}
						}
						break
					}
				}

				if isServer {
					numServers += currentCount
				} else {
					numAgents += currentCount
				}
			}
		}
	}

	LogLevel("info", "Calculated node counts from vars.tfvars: servers=%d, agents=%d", numServers, numAgents)
	return numServers, numAgents
}

// loadQAInfraConfig loads comprehensive cluster configuration from vars.tfvars
func loadQAInfraConfig(nodeSource string, c *Cluster, product string) error {
	varsFile := filepath.Join(nodeSource, "vars.tfvars")

	// Load AWS configuration
	c.Aws.AccessKeyID = os.Getenv("AWS_ACCESS_KEY_ID")
	c.Aws.SecretAccessKey = os.Getenv("AWS_SECRET_ACCESS_KEY")

	// Load from qa-infra vars.tfvars
	if err := loadVarsFromFile(varsFile, c); err != nil {
		return fmt.Errorf("failed to load qa-infra vars: %w", err)
	}

	// Set product from parameter
	if product != "" {
		c.Config.Product = product
	} else if c.Config.Product == "" {
		c.Config.Product = "rke2"
	}

	if c.Config.DataStore == "" {
		c.Config.DataStore = "etcd"
	}

	LogLevel("info", "Loaded comprehensive cluster configuration for qa-infra")
	return nil
}

// loadVarsFromFile loads variables from a .tfvars file into cluster config
func loadVarsFromFile(filename string, c *Cluster) error {
	content, err := os.ReadFile(filename)
	if err != nil {
		return err
	}

	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse key = value pairs
		if parts := strings.SplitN(line, "=", 2); len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.Trim(strings.TrimSpace(parts[1]), "\"")
			// LogLevel("debug", "Parsing tfvars line - key: '%s', value: '%s'", key, value)

			// Map variables to cluster config
			switch key {
			case "aws_region":
				c.Aws.Region = value
			case "aws_subnet", "subnets":
				c.Aws.Subnets = value
			case "availability_zone":
				c.Aws.AvailabilityZone = value
			case "sg_id":
				c.Aws.SgId = value
			case "vpc_id":
				c.Aws.VPCID = value
			case "public_ssh_key":
				// Convert public key path to private key path
				// Remove any trailing quotes and comments
				cleanValue := strings.Split(value, "\"")[0]
				cleanValue = strings.Split(cleanValue, "#")[0]
				cleanValue = strings.TrimSpace(cleanValue)
				privateKeyPath := strings.TrimSuffix(cleanValue, ".pub")
				c.Aws.EC2.AccessKey = privateKeyPath
			case "aws_ssh_user":
				LogLevel("debug", "Setting SSH User from vars.tfvars: %s", value)
				c.Aws.EC2.AwsUser = value
			case "aws_ami":
				c.Aws.EC2.Ami = value
			case "volume_size":
				c.Aws.EC2.VolumeSize = value
			case "ec2_instance_class", "instance_type":
				c.Aws.EC2.InstanceClass = value
			case "key_name":
				c.Aws.EC2.KeyName = value
			case "rke2_version":
				c.Config.Version = value
			case "rke2_channel":
				c.Config.Channel = value
			case "install_method":
				c.Config.InstallMethod = value
			case "install_mode":
				c.Config.InstallMode = value
			case "arch":
				c.Config.Arch = value
			case "server_flags":
				c.Config.ServerFlags = value
			case "worker_flags":
				c.Config.WorkerFlags = value
			case "datastore_type":
				c.Config.DataStore = value
			case "external_db":
				c.Config.ExternalDb = value
			case "external_db_version":
				c.Config.ExternalDbVersion = value
			case "db_group_name":
				c.Config.ExternalDbGroupName = value
			case "instance_class":
				c.Config.ExternalDbNodeType = value
			}
		}
	}

	return nil
}

// generatePublicKeyFromPrivate generates a public key from a private key using ssh-keygen
func generatePublicKeyFromPrivate(privateKeyPath, publicKeyPath string) error {
	// Use ssh-keygen to extract public key from private key
	cmd := exec.Command("ssh-keygen", "-y", "-f", privateKeyPath)
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to extract public key: %w", err)
	}

	// Write public key to file
	if err := os.WriteFile(publicKeyPath, output, 0644); err != nil {
		return fmt.Errorf("failed to write public key file: %w", err)
	}

	LogLevel("info", "Generated public key: %s", publicKeyPath)
	return nil
}

// updateMainTfModuleSource updates main.tf to use the correct infrastructure module based on INFRA_MODULE env var
func updateMainTfModuleSource(mainTfPath string) error {
	infraModule := os.Getenv("INFRA_MODULE")
	if infraModule == "" {
		infraModule = "aws"
	}

	LogLevel("info", "Using infrastructure module: %s", infraModule)

	// Read current main.tf content
	content, err := os.ReadFile(mainTfPath)
	if err != nil {
		return fmt.Errorf("failed to read main.tf: %w", err)
	}

	contentStr := string(content)

	// Update module source to use add.vsphere branch and correct module path
	placeholder := "placeholder-for-remote-module"
	fmoralBranch := "add.vsphere"
	modulePath := infraModule + "/modules/cluster_nodes"

	srcModule := fmt.Sprintf("github.com/fmoral2/qa-infra-automation//tofu/%s?ref=%s", modulePath, fmoralBranch)
	contentStr = strings.ReplaceAll(contentStr, placeholder, srcModule)

	// Write updated content back to file
	if err := os.WriteFile(mainTfPath, []byte(contentStr), 0644); err != nil {
		return fmt.Errorf("failed to write updated main.tf: %w", err)
	}

	// switch infraModule {
	// case "aws":
	// 	// Update to use add.vsphere branch for AWS module
	// 	newSource := fmt.Sprintf("github.com/fmoral2/qa-infra-automation//tofu/%s?ref=%s", awsModulePath, fmoralBranch)
	// 	contentStr = strings.ReplaceAll(contentStr, placeholder, newSource)
	// 	LogLevel("info", "Updated AWS module source to add.vsphere branch: %s", newSource)

	// case "vsphere":
	// 	// Update module source to point to vSphere module on add.vsphere branch
	// 	newSource := fmt.Sprintf("github.com/fmoral2/qa-infra-automation//tofu/%s?ref=%s", vsphereModulePath, fmoralBranch)
	// 	contentStr = strings.ReplaceAll(contentStr, placeholder, newSource)
	// 	LogLevel("info", "Updated module source to vSphere: %s", newSource)

	// 	contentStr = strings.ReplaceAll(contentStr, awsProvider, vsphereProvider)
	// 	LogLevel("debug", "Updated terraform providers to use vSphere provider")

	// default:
	// 	return fmt.Errorf("unsupported infrastructure module: %s (supported: aws, vsphere)", infraModule)
	// }

	LogLevel("debug", "Successfully updated main.tf for %s infrastructure", infraModule)

	return nil
}
