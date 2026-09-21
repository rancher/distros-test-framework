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

	// RunID identifies this provisioning run; RunDir holds its state and manifest.
	RunID  string
	RunDir string

	// QAInfra pins rancher/qa-infra-automation (or a fork) to one resolved commit.
	QAInfraRepo string
	QAInfraRef  string
	QAInfraSHA  string

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
