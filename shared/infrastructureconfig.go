package shared

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	qaCfg                  *QAInfraConfig
	qaCluster              *Cluster
	qaOnce                 sync.Once
	defaultContainerKeyDir = "/go/src/github.com/rancher/distros-test-framework"
)

type ProvisioningStep func(*QAInfraConfig) error

type InfraConfig struct {
	InfraProvider  string
	ResourceName   string
	Product        string
	Module         string
	InstallVersion string
	TFVars         string
	QAInfraModule  string
	SSHKeyPath     string
	SSHUser        string

	*QAInfraConfig
}

// QAInfraConfig holds comprehensive configuration for qa-infra provisioning
type QAInfraConfig struct {
	Workspace      string
	UniqueID       string
	Product        string
	InstallVersion string
	IsContainer    bool
	QAInfraModule  string
	SSHConfig      SSHConfig

	RootDir        string
	NodeSource     string
	TempDir        string
	KubeconfigPath string

	Inventory
	Ansible
	Terraform

	AirgapSetup bool
	ProxySetup  bool
}

type Ansible struct {
	Dir  string
	Path string
}

type Terraform struct {
	TFVarsPath string
	MainTfPath string
}

type Inventory struct {
	Path string
}

func addQAInfraEnv(infra InfraConfig, c *Cluster) *QAInfraConfig {
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
func loadQAInfra(infra InfraConfig, c *Cluster) (*QAInfraConfig, error) {
	workspace := fmt.Sprintf("dsf-%s", time.Now().Format("20060102150405"))
	uniqueID := time.Now().Format("0102-1504")

	// Determine the root directory for operations
	var defaultKeyDir string
	_, callerFilePath, _, _ := runtime.Caller(0)
	defaultKeyDir = filepath.Join(filepath.Dir(callerFilePath), "..")

	isContainer := false
	if isRunningInContainer() {
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

	// Use SSH config from the passed infra configuration (already loaded from env)
	sshUser := infra.QAInfraConfig.SSHConfig.User
	sshKeyPath := infra.QAInfraConfig.SSHConfig.KeyPath

	// Adjust SSH key path for container environment
	if isContainer && sshKeyPath != "" {
		// In container, SSH keys are typically mounted to a standard location
		sshKeyPath = defaultContainerKeyDir + "/shared/config/.ssh/aws_key.pem"
		LogLevel("info", "Container detected: Using container SSH key path: %s", sshKeyPath)
	}

	LogLevel("info", "SSH Configuration - User: %s, KeyPath: %s", sshUser, sshKeyPath)

	qaInfraConfig := &QAInfraConfig{
		Workspace:      workspace,
		UniqueID:       uniqueID,
		Product:        strings.ToLower(infra.Product),
		InstallVersion: infra.InstallVersion,
		QAInfraModule:  infra.QAInfraModule,
		SSHConfig: SSHConfig{
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
			LogLevel("warn", "Failed to load complete configuration: %v", err)
		}
	}

	LogLevel("debug", "Created QA infra configuration:\n%+v", qaInfraConfig)

	return qaInfraConfig, nil
}

// loadQAInfraConfig loads comprehensive cluster configuration from vars.tfvars
func loadQAInfraTFVars(nodeSource string, c *Cluster) error {
	varsFile := filepath.Join(nodeSource, "vars.tfvars")

	// Load from qa-infra vars.tfvars
	if err := loadVarsFromFile(c, varsFile); err != nil {
		return fmt.Errorf("failed to load qa-infra vars: %w", err)
	}

	// todo: finishing adding all data to accommated all tests.
	if c.Config.DataStore == "" {
		c.Config.DataStore = "etcd"
	}

	LogLevel("info", "Cluster configuration loaded from vars.tfvars\n%+v", c)

	return nil
}

// loadVarsFromFile loads variables from a vars.tfvars file into cluster config.
func loadVarsFromFile(c *Cluster, filename string) error {
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

// QAInfraClusterConfig returns a singleton cluster configuration for qa-infra
func QAInfraClusterConfig(infraConfig *QAInfraConfig, fqdn string) *Cluster {
	qaOnce.Do(func() {
		var err error
		qaCluster, err = buildClusterConfig(infraConfig, fqdn)
		if err != nil {
			LogLevel("error", "error building qa-infra cluster config: %v", err)
			os.Exit(1)
		}
	})

	return qaCluster
}

// buildClusterConfig creates the final cluster configuration
func buildClusterConfig(infraConfig *QAInfraConfig, fqdn string) (*Cluster, error) {

	// Get node counts from environment
	serverCount, _ := strconv.Atoi(os.Getenv("NO_OF_SERVER_NODES"))
	agentCount, _ := strconv.Atoi(os.Getenv("NO_OF_WORKER_NODES"))

	// Get actual node IPs from Terraform state
	nodes, err := getAllNodesFromState(infraConfig.NodeSource)
	if err != nil {
		return nil, fmt.Errorf("failed to get node IPs from state: %w", err)
	}
	fmt.Println("nodes", nodes)

	// Separate server and agent IPs based on node roles
	var serverIPs, agentIPs []string
	// for _, node := range nodes {
	// 	if strings.Contains(node.Role, "etcd") || strings.Contains(node.Role, "cp") || strings.Contains(node.Role, "control-plane") {
	// 		serverIPs = append(serverIPs, node.PublicIP)
	// 	} else if strings.Contains(node.Role, "worker") && !strings.Contains(node.Role, "etcd") && !strings.Contains(node.Role, "cp") {
	// 		agentIPs = append(agentIPs, node.PublicIP)
	// 	}
	// }

	// Create cluster configuration
	cc := &Cluster{
		Status:     "cluster created",
		ServerIPs:  serverIPs,
		AgentIPs:   agentIPs,
		NumServers: serverCount,
		NumAgents:  agentCount,
		FQDN:       fqdn,
		SSH: SSHConfig{
			KeyPath: infraConfig.SSHConfig.KeyPath,
			User:    infraConfig.SSHConfig.User,
		},
		Config: clusterConfig{
			Product: infraConfig.Product,
			Version: infraConfig.InstallVersion,
			// ServerFlags:   infraConfig.ServerFlags,
			// WorkerFlags:   infraConfig.WorkerFlags,
			// Channel:       infraConfig.Channel,
			// InstallMethod: infraConfig.InstallMethod,
			// InstallMode:   infraConfig.InstallMode,
			// DataStore:     infraConfig.DataStore,
			// Arch:          infraConfig.Arch,
		},
	}

	return cc, nil
}

func isRunningInContainer() bool {
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}

	if gopath := os.Getenv("GOPATH"); gopath == "/go" {
		return true
	}

	if wd, err := os.Getwd(); err == nil {
		if strings.HasPrefix(wd, "/go/src/github.com/rancher/distros-test-framework") {
			return true
		}
	}

	return false
}
