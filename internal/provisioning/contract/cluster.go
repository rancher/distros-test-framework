package contract

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
	TestConfig   TestConfig
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
	SplitRoles          SplitRolesConfig
}

// SplitRolesConfig holds split roles configuration
type SplitRolesConfig struct {
	Add                bool
	NumServers         int
	ControlPlaneOnly   int
	ControlPlaneWorker int
	EtcdOnly           int
	EtcdCP             int
	EtcdWorker         int
}

// TestConfig holds test-specific configuration
type TestConfig struct {
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
