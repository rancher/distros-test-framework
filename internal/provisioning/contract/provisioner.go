package contract

import ()

type Provisioner interface {
	Provision(cfg InfraConfig, c *Cluster) (*Cluster, error)
	Destroy(c *Cluster) error
}
