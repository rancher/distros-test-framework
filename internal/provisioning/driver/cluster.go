package driver

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
