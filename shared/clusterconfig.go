package shared

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gruntwork-io/terratest/modules/terraform"

	"github.com/rancher/distros-test-framework/config"
	"github.com/rancher/distros-test-framework/pkg/customflag"
)

var (
	once    sync.Once
	cluster *Cluster
)

type Cluster struct {
	Status        string
	ServerIPs     []string
	AgentIPs      []string
	WinAgentIPs   []string
	NumWinAgents  int
	NumServers    int
	NumAgents     int
	NumBastion    int
	FQDN          string
	Config        clusterConfig
	Aws           AwsConfig
	BastionConfig bastionConfig
	NodeOS        string
}

type AwsConfig struct {
	AccessKeyID     string
	SecretAccessKey string
	Region          string
	EC2
}

type EC2 struct {
	AccessKey        string
	AwsUser          string
	Ami              string
	VolumeSize       string
	InstanceClass    string
	Subnets          string
	AvailabilityZone string
	SgId             string
	KeyName          string
}

type clusterConfig struct {
	RenderedTemplate string
	ExternalDb       string
	DataStore        string
	Product          string
	Arch             string
	Version          string
	ServerFlags      string
}

type bastionConfig struct {
	PublicIPv4Addr string
	PublicDNS      string
}

type Node struct {
	Name              string
	Status            string
	Roles             string
	Version           string
	InternalIP        string
	ExternalIP        string
	OperationalSystem string
}

type Pod struct {
	NameSpace      string
	Name           string
	Ready          string
	Status         string
	Restarts       string
	Age            string
	IP             string
	Node           string
	NominatedNode  string
	ReadinessGates string
}

// ClusterConfig returns a singleton cluster with all terraform config and vars.
func ClusterConfig(envCfg *config.Env) *Cluster {
	once.Do(func() {
		var err error
		cluster, err = newCluster(envCfg.Product, envCfg.Module)
		if err != nil {
			LogLevel("error", "error getting cluster: %w\n", err)
			if customflag.ServiceFlag.Destroy {
				LogLevel("info", "\nmoving to start destroy operation\n")
				status, destroyErr := DestroyCluster(envCfg)
				if destroyErr != nil {
					LogLevel("error", "error destroying cluster: %w\n", destroyErr)
					os.Exit(1)
				}
				if status != "cluster destroyed" {
					LogLevel("error", "cluster not destroyed: %s\n", status)
					os.Exit(1)
				}
			}
			os.Exit(1)
		}
	})

	return cluster
}

func addClusterFromKubeConfig(nodes []Node) (*Cluster, error) {
	// if it is configureSSH() call then return the cluster with only aws key/user.
	if nodes == nil {
		return &Cluster{
			Aws: AwsConfig{
				EC2: EC2{
					AccessKey: os.Getenv("access_key"),
					AwsUser:   os.Getenv("aws_user"),
				},
			},
		}, nil
	}

	var serverIPs, agentIPs []string

	for i := range nodes {
		if nodes[i].Roles == "<none>" && nodes[i].Roles != "control-plane" {
			agentIPs = append(agentIPs, nodes[i].ExternalIP)
		} else {
			serverIPs = append(serverIPs, nodes[i].ExternalIP)
		}
	}

	return &Cluster{
		Status:     "cluster created",
		ServerIPs:  serverIPs,
		AgentIPs:   agentIPs,
		NumAgents:  len(agentIPs),
		NumServers: len(serverIPs),
		Aws: AwsConfig{
			Region: os.Getenv("region"),
			EC2: EC2{
				AccessKey:        os.Getenv("access_key"),
				AwsUser:          os.Getenv("aws_user"),
				Ami:              os.Getenv("aws_ami"),
				VolumeSize:       os.Getenv("volume_size"),
				InstanceClass:    os.Getenv("ec2_instance_class"),
				Subnets:          os.Getenv("subnets"),
				AvailabilityZone: os.Getenv("availability_zone"),
				SgId:             os.Getenv("sg_id"),
				KeyName:          os.Getenv("key_name"),
			},
		},
		Config: clusterConfig{
			Product:          os.Getenv("ENV_PRODUCT"),
			RenderedTemplate: os.Getenv("rendered_template"),
			DataStore:        os.Getenv("datastore_type"),
			ExternalDb:       os.Getenv("external_db"),
			Arch:             os.Getenv("arch"),
		},
		BastionConfig: bastionConfig{
			PublicIPv4Addr: os.Getenv("BASTION_IP"),
		},
		NodeOS: os.Getenv("node_os"),
	}, nil
}

// newCluster creates a new cluster and returns his values from terraform config and vars.
func newCluster(product, module string) (*Cluster, error) {
	c := &Cluster{}
	t := &testing.T{}

	localServersIps := os.Getenv("LOCAL_SERVER_IPS")
	localAgentIps := os.Getenv("LOCAL_AGENT_IPS")

	if localServersIps == "" {
		terraformOptions, varDir, err := setTerraformOptions(product, module)
		if err != nil {
			return nil, err
		}

		numServers, err := strconv.Atoi(terraform.GetVariableAsStringFromVarFile(
			t, varDir, "no_of_server_nodes"))
		if err != nil {
			return nil, fmt.Errorf("error getting no_of_server_nodes from var file: %w", err)
		}
		numAgents, err := strconv.Atoi(terraform.GetVariableAsStringFromVarFile(
			t, varDir, "no_of_worker_nodes"))
		if err != nil {
			return nil, fmt.Errorf("error getting no_of_worker_nodes from var file: %w", err)
		}
		numBastion, err := terraform.GetVariableAsStringFromVarFileE(t, varDir, "no_of_bastion_nodes")
		if err != nil {
			LogLevel("debug", "no_of_bastion_nodes not found in tfvars")
		} else {
			c.NumBastion, _ = strconv.Atoi(numBastion)
		}

		LogLevel("debug", "Applying Terraform config and Creating cluster\n")
		_, err = terraform.InitAndApplyE(t, terraformOptions)
		if err != nil {
			return nil, fmt.Errorf("\nTerraform apply Failed: %w", err)
		}
		LogLevel("debug", "Applying Terraform config completed!\n")

		if os.Getenv("split_roles") == "true" {
			LogLevel("debug", "Checking and adding split roles...")
			numServers, err = addSplitRole(t, varDir, numServers)
			if err != nil {
				return nil, err
			}
		}
		c.NumServers = numServers
		c.NumAgents = numAgents

		LogLevel("debug", "Loading TF Configs...")
		c, err = loadTFconfig(t, c, product, module, varDir, terraformOptions)
		if err != nil {
			return nil, err
		}
		c.Status = "cluster created"
		LogLevel("debug", "Cluster has been created successfully...")
	} else {
		LogLevel("info", "Using local cluster mode with provided IPs...")
		var err error
		c, err = createLocalCluster(product, module, localServersIps, localAgentIps)
		if err != nil {
			return nil, fmt.Errorf("error creating local cluster: %w", err)
		}

		LogLevel("info", "Running install on provided IPs...")
		err = installClusterNodes(c)
		if err != nil {
			return nil, fmt.Errorf("error installing cluster nodes: %w", err)
		}

		c.Status = "cluster created"
		LogLevel("info", "Local cluster setup completed successfully...")
	}

	return c, nil
}

// createLocalCluster creates a cluster configuration using local IPs and loads config from tfvars
func createLocalCluster(product, module, serverIpsStr, agentIpsStr string) (*Cluster, error) {
	var serverIPs, agentIPs []string

	if serverIpsStr != "" {
		serverIPs = strings.Split(strings.TrimSpace(serverIpsStr), ",")
		for i := range serverIPs {
			serverIPs[i] = strings.TrimSpace(serverIPs[i])
		}
	}

	if agentIpsStr != "" {
		agentIPs = strings.Split(strings.TrimSpace(agentIpsStr), ",")
		for i := range agentIPs {
			agentIPs[i] = strings.TrimSpace(agentIPs[i])
		}
	}

	LogLevel("info", "Local cluster mode: %d servers, %d agents", len(serverIPs), len(agentIPs))

	cluster := &Cluster{
		ServerIPs:  serverIPs,
		AgentIPs:   agentIPs,
		NumServers: len(serverIPs),
		NumAgents:  len(agentIPs),
		Config: clusterConfig{
			Product: product,
		},
	}

	// Load all configuration from tfvars file
	t := &testing.T{}
	_, varDir, err := setTerraformOptions(product, module)
	if err != nil {
		return nil, fmt.Errorf("error setting terraform options: %w", err)
	}

	LogLevel("info", "Loading configuration from %s.tfvars...", product)
	err = loadClusterConfigFromTfvars(t, varDir, cluster)
	if err != nil {
		return nil, fmt.Errorf("error loading configuration from tfvars: %w", err)
	}

	return cluster, nil
}

// loadClusterConfigFromTfvars loads complete configuration from tfvars files
func loadClusterConfigFromTfvars(t *testing.T, varDir string, c *Cluster) error {
	if region, err := terraform.GetVariableAsStringFromVarFileE(t, varDir, "region"); err == nil {
		c.Aws.Region = region
	}

	// Load AWS credentials from environment (standard AWS env vars)
	c.Aws.AccessKeyID = os.Getenv("AWS_ACCESS_KEY_ID")
	c.Aws.SecretAccessKey = os.Getenv("AWS_SECRET_ACCESS_KEY")

	// Load EC2 configuration from tfvars
	accessKey, err := terraform.GetVariableAsStringFromVarFileE(t, varDir, "access_key")
	if err == nil {
		c.Aws.EC2.AccessKey = accessKey
	}

	if awsUser, err := terraform.GetVariableAsStringFromVarFileE(t, varDir, "aws_user"); err == nil {
		c.Aws.EC2.AwsUser = awsUser
	}
	if ami, err := terraform.GetVariableAsStringFromVarFileE(t, varDir, "aws_ami"); err == nil {
		c.Aws.EC2.Ami = ami
	}
	if volumeSize, err := terraform.GetVariableAsStringFromVarFileE(t, varDir, "volume_size"); err == nil {
		c.Aws.EC2.VolumeSize = volumeSize
	}
	if instanceClass, err := terraform.GetVariableAsStringFromVarFileE(t, varDir, "ec2_instance_class"); err == nil {
		c.Aws.EC2.InstanceClass = instanceClass
	}
	if subnets, err := terraform.GetVariableAsStringFromVarFileE(t, varDir, "subnets"); err == nil {
		c.Aws.EC2.Subnets = subnets
	}
	if availabilityZone, err := terraform.GetVariableAsStringFromVarFileE(t, varDir, "availability_zone"); err == nil {
		c.Aws.EC2.AvailabilityZone = availabilityZone
	}
	if sgId, err := terraform.GetVariableAsStringFromVarFileE(t, varDir, "sg_id"); err == nil {
		c.Aws.EC2.SgId = sgId
	}
	if keyName, err := terraform.GetVariableAsStringFromVarFileE(t, varDir, "key_name"); err == nil {
		c.Aws.EC2.KeyName = keyName
	}

	// Load cluster configuration from tfvars
	if nodeOs, err := terraform.GetVariableAsStringFromVarFileE(t, varDir, "node_os"); err == nil {
		c.NodeOS = nodeOs
	}
	if arch, err := terraform.GetVariableAsStringFromVarFileE(t, varDir, "arch"); err == nil {
		c.Config.Arch = arch
	}
	if serverFlags, err := terraform.GetVariableAsStringFromVarFileE(t, varDir, "server_flags"); err == nil {
		c.Config.ServerFlags = serverFlags
	}
	if datastore, err := terraform.GetVariableAsStringFromVarFileE(t, varDir, "datastore_type"); err == nil {
		c.Config.DataStore = datastore
	}
	if externalDb, err := terraform.GetVariableAsStringFromVarFileE(t, varDir, "external_db"); err == nil {
		c.Config.ExternalDb = externalDb
	}
	if externalDbVersion, err := terraform.GetVariableAsStringFromVarFileE(t, varDir, "external_db_version"); err == nil {
		LogLevel("debug", "External DB version: %s", externalDbVersion)
	}
	if version, err := terraform.GetVariableAsStringFromVarFileE(t, varDir, "install_version"); err == nil {
		c.Config.Version = version
	}

	LogLevel("info", "Configuration:")
	LogLevel("info", "  Node OS: %s", c.NodeOS)
	LogLevel("info", "  Architecture: %s", c.Config.Arch)
	LogLevel("info", "  AWS User: %s", c.Aws.EC2.AwsUser)
	LogLevel("info", "  Access Key: %s", c.Aws.EC2.AccessKey)
	LogLevel("info", "  Datastore: %s", c.Config.DataStore)
	if c.Config.Version != "" {
		LogLevel("info", "  Version: %s", c.Config.Version)
	}
	if c.Config.ServerFlags != "" {
		LogLevel("info", "  Server Flags: %s", c.Config.ServerFlags)
	}

	return nil
}

// installClusterNodes installs the cluster on provided IPs using curl commands
func installClusterNodes(c *Cluster) error {
	if len(c.ServerIPs) == 0 {
		return fmt.Errorf("no server IPs provided")
	}

	token := "test"
	firstServerIP := c.ServerIPs[0]
	LogLevel("info", "Installing first server on %s...", firstServerIP)
	err := installFirstServer(c, firstServerIP, "test")
	if err != nil {
		return fmt.Errorf("failed to install first server: %w", err)
	}

	LogLevel("info", "Waiting for first server to be ready...")
	time.Sleep(30 * time.Second)

	if len(c.ServerIPs) > 1 {
		LogLevel("info", "Installing additional servers...")
		for i, serverIP := range c.ServerIPs[1:] {
			LogLevel("info", "Installing server %d on %s...", i+2, serverIP)
			err := installAdditionalServer(c, serverIP, firstServerIP, token)
			if err != nil {
				return fmt.Errorf("failed to install server %s: %w", serverIP, err)
			}
			time.Sleep(10 * time.Second)
		}
	}

	if len(c.AgentIPs) > 0 {
		LogLevel("info", "Installing agents...")
		for i, agentIP := range c.AgentIPs {
			LogLevel("info", "Installing agent %d on %s...", i+1, agentIP)
			err := installAgent(c, agentIP, firstServerIP, token)
			if err != nil {
				return fmt.Errorf("failed to install agent %s: %w", agentIP, err)
			}
			time.Sleep(5 * time.Second)
		}
	}

	LogLevel("info", "All cluster nodes installed successfully")
	return nil
}

// installFirstServer installs and configures the first server (bootstrap node)
func installFirstServer(c *Cluster, serverIP, token string) error {
	configDir := fmt.Sprintf("/etc/rancher/%s", c.Config.Product)
	configPath := fmt.Sprintf("%s/config.yaml", configDir)

	configContent := buildServerConfig(c, token, true)

	createDirCmd := fmt.Sprintf("sudo mkdir -p %s", configDir)

	createConfigCmd := fmt.Sprintf("sudo tee %s > /dev/null << 'EOF'\n%s\nEOF", configPath, configContent)

	installCmd := buildInstallCommand(c, true)

	commands := []string{createDirCmd, createConfigCmd, installCmd}

	for _, cmd := range commands {
		LogLevel("debug", "Executing on %s: %s", serverIP, cmd)
		_, err := RunCommandOnNode(cmd, serverIP)
		if err != nil {
			return fmt.Errorf("failed to execute command on %s: %w", serverIP, err)
		}
	}

	// Handle SLEMicro OS special requirements
	if isSLEMicro(c.NodeOS) {
		LogLevel("info", "SLEMicro detected, rebooting %s...", serverIP)

		rebootCmd := "sudo reboot"
		_, _ = RunCommandOnNode(rebootCmd, serverIP)

		LogLevel("info", "Waiting for %s to come back online after reboot...", serverIP)
		err := waitForNodeOnline(serverIP, 120) // Wait up to 2 minutes
		if err != nil {
			return fmt.Errorf("node %s did not come back online after reboot: %w", serverIP, err)
		}

		LogLevel("info", "Node %s is back online, enabling service...", serverIP)
	}

	enableCmd := fmt.Sprintf("sudo systemctl enable --now %s", c.Config.Product)

	LogLevel("debug", "Executing on %s: %s", serverIP, enableCmd)
	_, err := RunCommandOnNode(enableCmd, serverIP)
	if err != nil {
		return fmt.Errorf("failed to execute command on %s: %w", serverIP, err)
	}

	return nil
}

// installAdditionalServer installs and configures additional servers that join the first server
func installAdditionalServer(c *Cluster, serverIP, firstServerIP, token string) error {
	configDir := fmt.Sprintf("/etc/rancher/%s", c.Config.Product)
	configPath := fmt.Sprintf("%s/config.yaml", configDir)

	configContent := buildServerConfig(c, token, false)

	// Use correct port based on product
	port := "6443"
	if strings.ToLower(c.Config.Product) == "rke2" {
		port = "9345"
	}

	configContent += fmt.Sprintf("server: https://%s:%s\n", firstServerIP, port)

	createDirCmd := fmt.Sprintf("sudo mkdir -p %s", configDir)

	createConfigCmd := fmt.Sprintf("sudo tee %s > /dev/null << 'EOF'\n%s\nEOF", configPath, configContent)

	installCmd := buildInstallCommand(c, true)

	commands := []string{createDirCmd, createConfigCmd, installCmd}

	for _, cmd := range commands {
		LogLevel("debug", "Executing on %s: %s", serverIP, cmd)
		_, err := RunCommandOnNode(cmd, serverIP)
		if err != nil {
			return fmt.Errorf("failed to execute command on %s: %w", serverIP, err)
		}
	}

	// Handle SLEMicro OS special requirements
	if isSLEMicro(c.NodeOS) {
		LogLevel("info", "SLEMicro detected, rebooting %s...", serverIP)

		rebootCmd := "sudo reboot"
		_, _ = RunCommandOnNode(rebootCmd, serverIP)

		LogLevel("info", "Waiting for %s to come back online after reboot...", serverIP)
		err := waitForNodeOnline(serverIP, 120) // Wait up to 2 minutes
		if err != nil {
			return fmt.Errorf("node %s did not come back online after reboot: %w", serverIP, err)
		}

		LogLevel("info", "Node %s is back online, enabling service...", serverIP)
	}

	enableCmd := fmt.Sprintf("sudo systemctl enable --now %s", c.Config.Product)
	LogLevel("debug", "Executing on %s: %s", serverIP, enableCmd)
	_, err := RunCommandOnNode(enableCmd, serverIP)
	if err != nil {
		return fmt.Errorf("failed to execute command on %s: %w", serverIP, err)
	}

	return nil
}

// installAgent installs and configures agents that join the cluster
func installAgent(c *Cluster, agentIP, firstServerIP, token string) error {
	configDir := fmt.Sprintf("/etc/rancher/%s", c.Config.Product)
	configPath := fmt.Sprintf("%s/config.yaml", configDir)

	configContent := buildAgentConfig(c, token, firstServerIP)

	createDirCmd := fmt.Sprintf("sudo mkdir -p %s", configDir)

	createConfigCmd := fmt.Sprintf("sudo tee %s > /dev/null << 'EOF'\n%s\nEOF", configPath, configContent)

	installCmd := buildInstallCommand(c, false)
	commands := []string{createDirCmd, createConfigCmd, installCmd}

	for _, cmd := range commands {
		LogLevel("debug", "Executing on %s: %s", agentIP, cmd)
		_, err := RunCommandOnNode(cmd, agentIP)
		if err != nil {
			return fmt.Errorf("failed to execute command on %s: %w", agentIP, err)
		}
	}

	// Handle SLEMicro OS special requirements
	if isSLEMicro(c.NodeOS) {
		LogLevel("info", "SLEMicro detected, rebooting %s...", agentIP)

		rebootCmd := "sudo reboot"
		_, _ = RunCommandOnNode(rebootCmd, agentIP)

		LogLevel("info", "Waiting for %s to come back online after reboot...", agentIP)
		err := waitForNodeOnline(agentIP, 120)
		if err != nil {
			return fmt.Errorf("node %s did not come back online after reboot: %w", agentIP, err)
		}

		LogLevel("info", "Node %s is back online, enabling service...", agentIP)
	}

	enableCmd := fmt.Sprintf("sudo systemctl enable --now %s-agent", c.Config.Product)
	LogLevel("debug", "Executing on %s: %s", agentIP, enableCmd)
	_, err := RunCommandOnNode(enableCmd, agentIP)
	if err != nil {
		return fmt.Errorf("failed to execute command on %s: %w", agentIP, err)
	}

	return nil
}

// buildServerConfig builds the config.yaml content for server nodes
func buildServerConfig(c *Cluster, token string, isFirstServer bool) string {
	var config strings.Builder

	config.WriteString(fmt.Sprintf("token: %s\n", token))
	config.WriteString("write-kubeconfig-mode: 644\n")

	if isFirstServer && strings.ToLower(c.Config.Product) == "k3s" {
		config.WriteString("cluster-init: true\n")
	}

	if c.Config.ServerFlags != "" {
		flags := strings.Fields(c.Config.ServerFlags)
		for _, flag := range flags {
			if strings.HasPrefix(flag, "--") {
				parts := strings.SplitN(flag[2:], "=", 2)
				if len(parts) == 2 {
					config.WriteString(fmt.Sprintf("%s: %s\n", parts[0], parts[1]))
				} else {
					config.WriteString(fmt.Sprintf("%s: true\n", parts[0]))
				}
			}
		}
	}

	if c.Config.DataStore != "" && c.Config.DataStore != "etcd" {
		config.WriteString(fmt.Sprintf("datastore-endpoint: %s\n", c.Config.ExternalDb))
	}

	return config.String()
}

// buildAgentConfig builds the config.yaml content for agent nodes
func buildAgentConfig(c *Cluster, token, serverIP string) string {
	var config strings.Builder

	config.WriteString(fmt.Sprintf("token: %s\n", token))

	// Use correct port based on product
	port := "6443"
	if strings.ToLower(c.Config.Product) == "rke2" {
		port = "9345"
	}

	config.WriteString(fmt.Sprintf("server: https://%s:%s\n", serverIP, port))

	return config.String()
}

// buildInstallCommand builds the curl installation command
func buildInstallCommand(c *Cluster, isServer bool) string {
	var cmd strings.Builder

	// Determine the installation URL based on product.
	var installURL string
	switch strings.ToLower(c.Config.Product) {
	case "k3s":
		installURL = "https://get.k3s.io"
	case "rke2":
		installURL = "https://get.rke2.io"
	default:
		installURL = "https://get.k3s.io"
	}

	cmd.WriteString(fmt.Sprintf("curl -sfL %s | ", installURL))
	if c.Config.Version != "" {
		cmd.WriteString(fmt.Sprintf("INSTALL_%s_VERSION=%s ",
			strings.ToUpper(c.Config.Product), c.Config.Version))
	}

	if strings.Contains(strings.ToLower(c.NodeOS), "micro") {
		cmd.WriteString(fmt.Sprintf("INSTALL_%s_SKIP_ENABLE=true ", strings.ToUpper(c.Config.Product)))
	}

	if !isServer {
		cmd.WriteString(fmt.Sprintf("INSTALL_%s_EXEC=\"agent\" ", strings.ToUpper(c.Config.Product)))
	}

	cmd.WriteString("sh -")

	return cmd.String()
}

// DestroyCluster destroys the cluster and returns it.
func DestroyCluster(cfg *config.Env) (string, error) {
	terraformOptions, _, err := setTerraformOptions(cfg.Product, cfg.Module)
	if err != nil {
		return "", err
	}
	terraform.Destroy(&testing.T{}, terraformOptions)

	return "cluster destroyed", nil
}

// isSLEMicro checks if the node OS is SLEMicro
func isSLEMicro(nodeOS string) bool {
	return strings.Contains(strings.ToLower(nodeOS), "slemicro") ||
		strings.Contains(strings.ToLower(nodeOS), "sle-micro")
}

// waitForNodeOnline waits for a node to come back online after reboot
func waitForNodeOnline(nodeIP string, timeoutSeconds int) error {
	LogLevel("debug", "Waiting for node %s to come online...", nodeIP)

	for i := 0; i < timeoutSeconds; i += 5 {
		// Try to execute a simple command to check if node is responsive
		_, err := RunCommandOnNode("echo 'online'", nodeIP)
		if err == nil {
			LogLevel("debug", "Node %s is online after %d seconds", nodeIP, i)
			return nil
		}

		LogLevel("debug", "Node %s not yet online, waiting... (%d/%d seconds)", nodeIP, i, timeoutSeconds)
		time.Sleep(5 * time.Second)
	}

	return fmt.Errorf("node %s did not come online within %d seconds", nodeIP, timeoutSeconds)
}
