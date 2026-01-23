package driver

import (
	"fmt"
	"os"
)

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
	TestConfig   TestConfig
	SSH          SSHConfig
}

// AwsConfig holds AWS-specific configuration.
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

// SSHConfig holds SSH connection configuration.
type SSHConfig struct {
	PrivKeyPath string
	PubKeyPath  string
	User        string
	KeyName     string
}

// EC2 holds EC2-specific configuration.
type EC2 struct {
	Ami           string
	VolumeSize    string
	VolumeType    string
	InstanceClass string
}

// Config holds cluster configuration.
type Config struct {
	DataStore           string
	Product             string
	Module              string
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
	SplitRoles          SplitRolesConfig
}

// SplitRolesConfig holds split roles configuration.
type SplitRolesConfig struct {
	Add                bool
	NumServers         int
	ControlPlaneOnly   int
	ControlPlaneWorker int
	EtcdOnly           int
	EtcdCP             int
	EtcdWorker         int
}

// TestConfig holds test-specific configuration.
type TestConfig struct {
	Tag string
}

// BastionConfig holds bastion host configuration.
type BastionConfig struct {
	PublicIPv4Addr string
	PublicDNS      string
}

type HostCluster struct {
	ServerIP        string
	HostClusterType string
	// FQDN            string
	NodeOS         string
	KubeconfigPath string
	SSH            SSHConfig
	K3kcliVersion  string
}
type K3kCluster struct {
	Namespace      string
	Name           string
	KubeconfigPath string
}

type K3kClusterOptions struct {
	Description      string
	Mode             string
	StorageClassType string
	PersistenceType  string
	NoOfServers      int
	NoOfAgents       int
	ServerArgs       string
	ServiceCIDR      string
	UseValuesYAML    bool
	ValuesYAMLFile   string
	K3kCluster       K3kCluster
	K3SVersion       string
}

func (k3kCluster *K3kCluster) SetKubeconfigPath(host *HostCluster) {
	k3kCluster.KubeconfigPath = fmt.Sprintf("/home/%s/%s-%s-kubeconfig.yaml", host.SSH.User, k3kCluster.Namespace, k3kCluster.Name)
}

func (k3kCluster *K3kCluster) GetKubeconfigPath(host *HostCluster) string {
	if k3kCluster.KubeconfigPath == "" {
		k3kCluster.SetKubeconfigPath(host)
	}
	return k3kCluster.KubeconfigPath
}

func (k3kCluster *K3kCluster) SetNamespace(namespace string) {
	k3kCluster.Namespace = namespace
}

func (k3kCluster *K3kCluster) GetNamespace() string {
	if k3kCluster.Namespace == "" {
		k3kCluster.Namespace = os.Getenv("K3K_NAMESPACE")
		if k3kCluster.Namespace == "" {
			k3kCluster.SetNamespace("k3k-system")
		}
	}
	return k3kCluster.Namespace
}

func (host *HostCluster) getKubectlBin() string {
	var kubectlBin string
	if host.HostClusterType == "k3s" {
		kubectlBin = "kubectl"
	} else {
		kubectlBin = fmt.Sprintf("/var/lib/rancher/%s/bin/kubectl", host.HostClusterType)
	}
	return kubectlBin
}

func (host *HostCluster) GetKubectlPath() string {
	return fmt.Sprintf("%s --kubeconfig %s", host.getKubectlBin(), host.KubeconfigPath)
}

func (k3kCluster *K3kCluster) GetKubectlPath(host *HostCluster) string {
	return fmt.Sprintf("%s --kubeconfig %s", host.getKubectlBin(), k3kCluster.KubeconfigPath)
}
