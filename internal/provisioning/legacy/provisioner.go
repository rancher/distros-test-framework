package legacy

import (
	"github.com/rancher/distros-test-framework/internal/provisioning/driver"
)

// Provisioner implements the driver.Provisioner interface for legacy.
type Provisioner struct{}

func New() *Provisioner { return &Provisioner{} }

func (p *Provisioner) Provision(cfg *driver.InfraConfig) (*driver.Cluster, error) {
	return p.provision(cfg.Product, cfg.Module)
}

func (p *Provisioner) Destroy(product, module string) (string, error) {
	return p.destroy(product, module)
}
