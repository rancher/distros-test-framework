package shared

import (
	"fmt"
	"os"
	"strconv"
	"sync"
	"testing"

	"github.com/gruntwork-io/terratest/modules/terraform"

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
	TestConfig    testConfig
	SSH           SSHConfig
}

type AwsConfig struct {
	AccessKeyID      string
	SecretAccessKey  string
	Region           string
	VPCID            string
	AvailabilityZone string
	SgId             string
	Subnets          string
	EC2
}

type SSHConfig struct {
	KeyPath string
	PubKey  string
	PrivKey string
	KeyName string
	User    string
}

type EC2 struct {
	Ami           string
	VolumeSize    string
	VolumeType    string
	InstanceClass string
	KeyName       string
}

type clusterConfig struct {
	DataStore           string
	Product             string
	Channel             string
	InstallMethod       string
	InstallMode         string
	Arch                string
	Version             string
	ServerFlags         string
	WorkerFlags         string
	ExternalDbEndpoint  string
	ExternalDb          string
	ExternalDbVersion   string
	ExternalDbGroupName string
	ExternalDbNodeType  string
	SplitRoles          splitRolesConfig
}

type splitRolesConfig struct {
	Add                bool
	NumServers         int
	ControlPlaneOnly   int
	ControlPlaneWorker int
	EtcdOnly           int
	EtcdCP             int
	EtcdWorker         int
}

type testConfig struct {
	Tag string
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
func ClusterConfig(product, module string) *Cluster {
	once.Do(func() {
		var err error
		cluster, err = newCluster(product, module)
		if err != nil {
			LogLevel("error", "error getting cluster: %w\n", err)
			if customflag.ServiceFlag.Destroy {
				LogLevel("info", "\nmoving to start destroy operation\n")
				status, destroyErr := DestroyInfrastructure(product, module)
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

// newCluster creates a new cluster and returns his values from terraform config and vars.
func newCluster(product, module string) (*Cluster, error) {
	c := &Cluster{}
	t := &testing.T{}

	terraformOptions, varDir, err := setTerraformOptionsLegacy(product, module)
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
		c.NumBastion = 0
	} else {
		c.NumBastion, _ = strconv.Atoi(numBastion)
	}

	LogLevel("debug", "Applying Terraform config and Creating cluster\n")
	_, err = terraform.InitAndApplyE(t, terraformOptions)
	if err != nil {
		return nil, fmt.Errorf("\nTerraform apply Failed: %w", err)
	}
	LogLevel("debug", "Applying Terraform config completed!\n")

	splitRoles := terraform.GetVariableAsStringFromVarFile(t, varDir, "split_roles")
	if splitRoles == "true" || os.Getenv("split_roles") == "true" {
		LogLevel("debug", "Checking and adding split roles...")
		numServers, err = addSplitRole(t, &c.Config.SplitRoles, varDir, numServers)
		if err != nil {
			return nil, fmt.Errorf("error adding split roles: %w", err)
		}
	}
	c.NumServers = numServers
	c.NumAgents = numAgents

	LogLevel("debug", "Loading TF Configs...")
	c, err = loadTFconfig(t, c, product, module, varDir, terraformOptions)
	if err != nil {
		return nil, fmt.Errorf("error loading TF config: %w", err)
	}
	c.Status = "cluster created"
	LogLevel("debug", "Cluster has been created successfully...")

	return c, nil
}

//nolint:funlen // yep, but this makes more clear being one function.
func addClusterFromKubeConfig(nodes []Node) (*Cluster, error) {
	// if it is configureSSH() call then return the cluster with only key/user.
	if nodes == nil {
		return &Cluster{
			SSH: SSHConfig{
				KeyPath: os.Getenv("access_key"),
				// TODO: this should be added option to load any user, not only aws_user.
				User: os.Getenv("aws_user"),
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

	product := os.Getenv("ENV_PRODUCT")
	return &Cluster{
		Status:     "cluster created",
		ServerIPs:  serverIPs,
		AgentIPs:   agentIPs,
		NumAgents:  len(agentIPs),
		NumServers: len(serverIPs),
		SSH: SSHConfig{
			KeyPath: os.Getenv("access_key"),
			User:    os.Getenv("aws_user"),
		},

		Aws: AwsConfig{
			Region:           os.Getenv("region"),
			Subnets:          os.Getenv("subnets"),
			SgId:             os.Getenv("sg_id"),
			AvailabilityZone: os.Getenv("availability_zone"),
			EC2: EC2{

				Ami:           os.Getenv("aws_ami"),
				VolumeSize:    os.Getenv("volume_size"),
				InstanceClass: os.Getenv("ec2_instance_class"),
				KeyName:       os.Getenv("key_name"),
			},
		},
		Config: clusterConfig{
			Product:             product,
			Version:             getVersion(),
			ServerFlags:         getFlags("server"),
			WorkerFlags:         getFlags("worker"),
			Channel:             getChannel(product),
			InstallMethod:       os.Getenv("install_method"),
			InstallMode:         os.Getenv("install_mode"),
			DataStore:           os.Getenv("datastore_type"),
			ExternalDb:          os.Getenv("external_db"),
			ExternalDbVersion:   os.Getenv("external_db_version"),
			ExternalDbGroupName: os.Getenv("db_group_name"),
			ExternalDbNodeType:  os.Getenv("instance_class"),
			ExternalDbEndpoint:  os.Getenv("rendered_template"),
			Arch:                os.Getenv("arch"),
			SplitRoles: splitRolesConfig{
				Add: os.Getenv("split_roles") == "true",
				NumServers: parseEnvInt("etcd_only_nodes", 0) +
					parseEnvInt("etcd_cp_nodes", 0) +
					parseEnvInt("etcd_worker_nodes", 0) +
					parseEnvInt("cp_only_nodes", 0) +
					parseEnvInt("cp_worker_nodes", 0),
				ControlPlaneOnly:   parseEnvInt("cp_only_nodes", 0),
				ControlPlaneWorker: parseEnvInt("cp_worker_nodes", 0),
				EtcdOnly:           parseEnvInt("etcd_only_nodes", 0),
				EtcdCP:             parseEnvInt("etcd_cp_nodes", 0),
				EtcdWorker:         parseEnvInt("etcd_worker_nodes", 0),
			},
		},
		BastionConfig: bastionConfig{
			PublicIPv4Addr: os.Getenv("BASTION_IP"),
			PublicDNS:      os.Getenv("bastion_dns"),
		},
		NodeOS: os.Getenv("node_os"),
	}, nil
}

// getVersion retrieves the install version from environment variables.
// used for addClusterFromKubeConfig().
func getVersion() string {
	installVersion := os.Getenv("install_version")
	installVersionEnv := os.Getenv("INSTALL_VERSION")

	if installVersion != "" {
		return installVersion
	}

	if installVersionEnv != "" {
		return installVersionEnv
	}

	return ""
}

// getFlags retrieves the flags for a given node type from environment variables.
// used for addClusterFromKubeConfig().
func getFlags(nodeType string) string {
	flags := os.Getenv(nodeType + "_flags")
	flagsEnv := os.Getenv(nodeType + "_FLAGS")

	if flags != "" {
		return flags
	}

	if flagsEnv != "" {
		return flagsEnv
	}

	return ""
}

// getChannel retrieves the install channel for a given product from environment variables.
// used for addClusterFromKubeConfig().
func getChannel(product string) string {
	c := os.Getenv("install_channel")
	if c != "" {
		return c
	}

	channel := os.Getenv(product + "_channel")
	if channel != "" {
		return channel
	}

	return "testing"
}

// parseEnvInt helper that parses an environment variable as an integer.
// If the variable is not set or cannot be parsed, it returns the default value.
func parseEnvInt(key string, defaultValue int) int {
	if val := os.Getenv(key); val != "" {
		if value, err := strconv.Atoi(val); err == nil {
			return value
		}
	}

	return defaultValue
}

func addSplitRole(t *testing.T, sp *splitRolesConfig, varDir string, numServers int) (int, error) {
	etcdNodes, err := strconv.Atoi(terraform.GetVariableAsStringFromVarFile(
		t,
		varDir,
		"etcd_only_nodes",
	))
	if err != nil {
		return 0, fmt.Errorf("error getting etcd_only_nodes %w", err)
	}
	etcdCpNodes, err := strconv.Atoi(terraform.GetVariableAsStringFromVarFile(
		t,
		varDir,
		"etcd_cp_nodes",
	))
	if err != nil {
		return 0, fmt.Errorf("error getting etcd_cp_nodes %w", err)
	}
	etcdWorkerNodes, err := strconv.Atoi(terraform.GetVariableAsStringFromVarFile(
		t,
		varDir,
		"etcd_worker_nodes",
	))
	if err != nil {
		return 0, fmt.Errorf("error getting etcd_worker_nodes %w", err)
	}
	cpNodes, err := strconv.Atoi(terraform.GetVariableAsStringFromVarFile(
		t,
		varDir,
		"cp_only_nodes",
	))
	if err != nil {
		return 0, fmt.Errorf("error getting cp_only_nodes %w", err)
	}
	cpWorkerNodes, err := strconv.Atoi(terraform.GetVariableAsStringFromVarFile(
		t,
		varDir,
		"cp_worker_nodes",
	))
	if err != nil {
		return 0, fmt.Errorf("error getting cp_worker_nodes %w", err)
	}

	numServers = numServers + etcdNodes + etcdCpNodes + etcdWorkerNodes + cpNodes + cpWorkerNodes

	sp.Add = true
	sp.ControlPlaneOnly = cpNodes
	sp.EtcdOnly = etcdNodes
	sp.EtcdCP = etcdCpNodes
	sp.EtcdWorker = etcdWorkerNodes
	sp.ControlPlaneWorker = cpWorkerNodes
	sp.NumServers = numServers

	return numServers, nil
}
