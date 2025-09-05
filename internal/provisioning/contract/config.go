package contract

type InfraConfig struct {
	Provisioner    string
	ResourceName   string
	Product        string
	Module         string
	InstallVersion string
	TFVars         string
	QAInfraModule  string
	SSHKeyPath     string
	SSHUser        string
	*InfraProvisionerConfig
	Cluster *Cluster
}

// InfraProvisioner implements the Provider interface for qa-infra automation
type InfraProvisioner struct{}

// InfraProvisionerConfig holds comprehensive configuration for qa-infra provisioning
type InfraProvisionerConfig struct {
	Workspace      string
	UniqueID       string
	Product        string
	InstallVersion string
	IsContainer    bool
	QAInfraModule  string
	SSHConfig      provisioning.SSHConfig

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

type Inventory struct {
	Path string
}

type Terraform struct {
	TFVarsPath string
	MainTfPath string
}
