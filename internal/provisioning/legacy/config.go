package legacy

import (
	"os"
	"sync"
	"testing"

	"github.com/gruntwork-io/terratest/modules/terraform"

	"github.com/rancher/distros-test-framework/internal/pkg/customflag"
	"github.com/rancher/distros-test-framework/internal/resources"
)

// Cluster represents a Kubernetes cluster with all its configuration and state
type Cluster struct {
	Status       string
	ServerIPs    []string
	AgentIPs     []string
	WinAgentIPs  []string
	NumWinAgents int
	NumServers   int
	NumAgents    int
	NumBastion   int
	FQDN         string
	Config       Config
	Aws          AwsConfig
	Bastion      BastionConfig
	NodeOS       string
	TestConfig   testConfig
	SSH          SSHConfig
}

// AwsConfig holds AWS-specific configuration
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

// SSHConfig holds SSH connection configuration
type SSHConfig struct {
	KeyPath string
	PubKey  string
	PrivKey string
	KeyName string
	User    string
}

// EC2 holds EC2-specific configuration
type EC2 struct {
	Ami           string
	VolumeSize    string
	VolumeType    string
	InstanceClass string
	KeyName       string
}

// Config holds cluster configuration
type Config struct {
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

// splitRolesConfig holds split roles configuration
type splitRolesConfig struct {
	Add                bool
	NumServers         int
	ControlPlaneOnly   int
	ControlPlaneWorker int
	EtcdOnly           int
	EtcdCP             int
	EtcdWorker         int
}

// TestConfig holds test-specific configuration
type testConfig struct {
	Tag string
}

// BastionConfig holds bastion host configuration
type BastionConfig struct {
	PublicIPv4Addr string
	PublicDNS      string
}

// Node represents a cluster node
type Node struct {
	Name              string
	Status            string
	Roles             string
	Version           string
	InternalIP        string
	ExternalIP        string
	OperationalSystem string
}

// Pod represents a Kubernetes pod
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

var (
	once    sync.Once
	cluster *Cluster
)

// ClusterConfig returns a singleton cluster with all terraform config and vars.
func ClusterConfig(product, module string) *Cluster {
	once.Do(func() {
		var err error
		cluster, err = newCluster(product, module)
		if err != nil {
			resources.LogLevel("error", "error getting cluster: %w\n", err)
			if customflag.ServiceFlag.Destroy {
				resources.LogLevel("info", "\nmoving to start destroy operation\n")
				status, destroyErr := destroyLegacyInfra(product, module)
				if destroyErr != nil {
					resources.LogLevel("error", "error destroying cluster: %w\n", destroyErr)
					os.Exit(1)
				}
				if status != "cluster destroyed" {
					resources.LogLevel("error", "cluster not destroyed: %s\n", status)
					os.Exit(1)
				}
			}
			os.Exit(1)
		}
	})

	return cluster
}

func destroyLegacyInfra(product string, module string) (interface{}, interface{}) {
	terraformOptions, _, err := setTerraformOptions(product, module)
	if err != nil {
		return "", err
	}
	terraform.Destroy(&testing.T{}, terraformOptions)

	return "cluster destroyed", nil
}
