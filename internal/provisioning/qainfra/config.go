package qainfra

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rancher/distros-test-framework/internal/provisioning"
	"github.com/rancher/distros-test-framework/internal/resources"
)

type ProvisioningStep func(*InfraProvisionerConfig) error

var (
	qaCfg                  *InfraProvisionerConfig
	qaCluster              *provisioning.Cluster
	qaOnce                 sync.Once
	defaultContainerKeyDir = "/go/src/github.com/rancher/distros-test-framework"
)

func addQAInfraEnv(infra Config, c *provisioning.Cluster) *InfraProvisionerConfig {
	var err error
	qaOnce.Do(func() {
		qaCfg, err = loadQAInfra(infra, c)
		if err != nil {
			os.Exit(1)
		}
	})

	return qaCfg
}

// loadQAInfra creates a configuration for qa-infra provisioning.
func loadQAInfra(infra Config, c *provisioning.Cluster) (*InfraProvisionerConfig, error) {
	workspace := fmt.Sprintf("dsf-%s", time.Now().Format("20060102150405"))
	uniqueID := time.Now().Format("0102-1504")

	// Determine the root directory for operations
	var defaultKeyDir string
	_, callerFilePath, _, _ := runtime.Caller(0)
	defaultKeyDir = filepath.Join(filepath.Dir(callerFilePath), "..", "..")

	isContainer := false
	if resources.IsRunningInContainer() {
		isContainer = true
		defaultKeyDir = defaultContainerKeyDir
	}

	// create directory paths.
	nodeSource := filepath.Join(defaultKeyDir, "tmp", fmt.Sprintf("qa-infra-tofu-%s", workspace))
	tempDir := filepath.Join(defaultKeyDir, "tmp/qa-infra-ansible")

	// build ansible directory path.
	var ansiblePath string
	switch strings.ToLower(infra.Product) {
	case "k3s":
		ansiblePath = "ansible/k3s/default"
	case "rke2":
		ansiblePath = "ansible/rke2/default"
	default:
		return nil, fmt.Errorf("unsupported product: %s", infra.Product)
	}

	ansibleDir := filepath.Join(tempDir, ansiblePath)

	// Use SSH config from the passed infra configuration
	sshUser := infra.SSHUser
	sshKeyPath := infra.SSHKeyPath

	// Adjust SSH key path for container environment
	if isContainer && sshKeyPath != "" {
		// In container, SSH keys are typically mounted to a standard location
		sshKeyPath = defaultContainerKeyDir + "/shared/config/.ssh/aws_key.pem"
		resources.LogLevel("info", "Container detected: Using container SSH key path: %s", sshKeyPath)
	}

	resources.LogLevel("info", "SSH Configuration - User: %s, KeyPath: %s", sshUser, sshKeyPath)

	qaInfraConfig := &InfraProvisionerConfig{
		Workspace:      workspace,
		UniqueID:       uniqueID,
		Product:        strings.ToLower(infra.Product),
		InstallVersion: infra.InstallVersion,
		QAInfraModule:  infra.QAInfraModule,
		SSHConfig: provisioning.SSHConfig{
			User:    sshUser,
			KeyPath: sshKeyPath,
		},
		RootDir:    defaultKeyDir,
		NodeSource: nodeSource,
		TempDir:    tempDir,
		Ansible: Ansible{
			Dir:  ansibleDir,
			Path: ansiblePath,
		},
		Terraform: Terraform{
			TFVarsPath: filepath.Join(nodeSource, "vars.tfvars"),
			MainTfPath: filepath.Join(nodeSource, "main.tf"),
		},
		KubeconfigPath: filepath.Join(ansibleDir, "kubeconfig.yaml"),

		IsContainer: isContainer,

		// todo get those from the vars.tfvars
		AirgapSetup: false,
		ProxySetup:  false,
	}

	if c != nil {
		if err := loadQAInfraTFVars(nodeSource, c); err != nil {
			resources.LogLevel("warn", "Failed to load complete configuration: %v", err)
		}
	}

	resources.LogLevel("debug", "Created QA infra configuration:\n%+v", qaInfraConfig)

	return qaInfraConfig, nil
}

// NewInfraClusterConfig returns a singleton cluster configuration for qa-infra
func NewInfraClusterConfig(infraConfig *InfraProvisionerConfig, fqdn string) *provisioning.Cluster {
	qaOnce.Do(func() {
		var err error
		qaCluster, err = buildClusterConfig(infraConfig, fqdn)
		if err != nil {
			resources.LogLevel("error", "error building qa-infra cluster config: %v", err)
			os.Exit(1)
		}
	})

	return qaCluster
}

// buildClusterConfig creates the final cluster configuration
func buildClusterConfig(infraConfig *InfraProvisionerConfig, fqdn string) (*provisioning.Cluster, error) {
	// Get node counts from environment
	serverCount, _ := strconv.Atoi(os.Getenv("NO_OF_SERVER_NODES"))
	agentCount, _ := strconv.Atoi(os.Getenv("NO_OF_WORKER_NODES"))

	// Get actual node IPs from Terraform state
	nodes, err := getAllNodesFromState(infraConfig.NodeSource)
	if err != nil {
		return nil, fmt.Errorf("failed to get node IPs from state: %w", err)
	}

	// Separate server and agent IPs based on node roles
	var serverIPs, agentIPs []string
	for _, node := range nodes {
		if strings.Contains(node.Role, "etcd") ||
			strings.Contains(node.Role, "cp") ||
			strings.Contains(node.Role, "control-plane") {
			serverIPs = append(serverIPs, node.PublicIP)
		} else if strings.Contains(node.Role, "worker") &&
			!strings.Contains(node.Role, "etcd") &&
			!strings.Contains(node.Role, "cp") {
			agentIPs = append(agentIPs, node.PublicIP)
		}
	}

	// Create cluster configuration
	cc := &provisioning.Cluster{
		Status:     "cluster created",
		ServerIPs:  serverIPs,
		AgentIPs:   agentIPs,
		NumServers: serverCount,
		NumAgents:  agentCount,
		FQDN:       fqdn,
		SSH: provisioning.SSHConfig{
			KeyPath: infraConfig.SSHConfig.KeyPath,
			User:    infraConfig.SSHConfig.User,
		},
		Config: provisioning.Config{
			Product: infraConfig.Product,
			Version: infraConfig.InstallVersion,
		},
	}

	return cc, nil
}

// loadQAInfraTFVars loads comprehensive cluster configuration from vars.tfvars
func loadQAInfraTFVars(nodeSource string, c *provisioning.Cluster) error {
	varsFile := filepath.Join(nodeSource, "vars.tfvars")

	// Load from qa-infra vars.tfvars
	if err := loadVarsFromFile(c, varsFile); err != nil {
		return fmt.Errorf("failed to load qa-infra vars: %w", err)
	}

	// todo: finishing adding all data to accommodate all tests.
	if c.Config.DataStore == "" {
		c.Config.DataStore = "etcd"
	}

	resources.LogLevel("info", "Cluster configuration loaded from vars.tfvars\n%+v", c)

	return nil
}

// loadVarsFromFile loads variables from a vars.tfvars file into cluster config.
func loadVarsFromFile(c *provisioning.Cluster, filename string) error {
	// those come from environment not from the file.
	c.Aws.AccessKeyID = os.Getenv("AWS_ACCESS_KEY_ID")
	c.Aws.SecretAccessKey = os.Getenv("AWS_SECRET_ACCESS_KEY")

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
				cleanValue := strings.Split(value, "\"")[0]
				cleanValue = strings.Split(cleanValue, "#")[0]
				cleanValue = strings.TrimSpace(cleanValue)
				privateKeyPath := strings.TrimSuffix(cleanValue, ".pub")
				c.SSH.KeyPath = privateKeyPath
			case "aws_ssh_user":
				c.SSH.User = value
			case "aws_ami":
				c.Aws.EC2.Ami = value
			case "volume_size":
				c.Aws.EC2.VolumeSize = value
			case "ec2_instance_class", "instance_type":
				c.Aws.EC2.InstanceClass = value
			case "aws_volume_type":
				c.Aws.EC2.VolumeType = value
			case "aws_volume_size":
				c.Aws.EC2.VolumeSize = value
			case "airgap_setup":
				// todo get those from the vars.tfvars
			case "proxy_setup":
				// todo get those from the vars.tfvars
			}
		}
	}

	return nil
}

// Placeholder implementations for the provisioning steps
func setupDirectories(config *InfraProvisionerConfig) error {
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

// runCmdWithTimeout executes a command with a specific timeout
func runCmdWithTimeout(dir string, timeout time.Duration, name string, args ...string) error {
	timeoutCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(timeoutCtx, name, args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()

	resources.LogLevel("info", "Running with %v timeout: %s %v (in %s)", timeout, name, args, dir)
	err := cmd.Run()

	if errors.Is(timeoutCtx.Err(), context.DeadlineExceeded) {
		return fmt.Errorf("command timed out after %v: %s %v", timeout, name, args)
	}

	return err
}

// runCmd executes a command with proper logging
func runCmd(dir, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()

	resources.LogLevel("info", "Running: %s %v (in %s)", name, args, dir)
	return cmd.Run()
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

	resources.LogLevel("debug", "Updated vars.tfvars: product=%s, uniquePrefix=%s", product, uniquePrefix)

	// Write back the updated content
	if err := os.WriteFile(varsFilePath, []byte(varsContent), 0644); err != nil {
		return fmt.Errorf("failed to write vars file: %w", err)
	}

	return nil
}
