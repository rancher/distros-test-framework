package driver

type Provisioner interface {
	Provision(cfg *InfraConfig) (*Cluster, error)
	Destroy(product, module string) (string, error)
}
