package driver

type InfraConfig struct {
	ProvisionerModule string
	ProvisionerType   string
	QAInfraProvider   string
	ResourceName      string
	Product           string
	Module            string
	InstallVersion    string

	NodeOS string
	CNI    string

	Cluster          *Cluster
	InfraProvisioner *InfraProvisionerConfig
}

// InfraProvisioner implements the Provider interface for qainfra automation compatibility.
type InfraProvisioner struct{}

type InfraProvisionerConfig struct {
	Workspace   string
	UniqueID    string
	IsContainer bool

	RootDir        string
	TFNodeSource   string
	TempDir        string
	KubeconfigPath string

	Inventory
	Ansible
	Terraform
	OpenTofuOutputs

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

type OpenTofuOutputs struct {
	KubeAPIHost string
	FQDN        string
}
