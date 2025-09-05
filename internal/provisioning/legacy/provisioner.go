package legacy

import (
	"github.com/rancher/distros-test-framework/internal/provisioning/contract"
	"github.com/rancher/distros-test-framework/internal/resources"
)

type Provider struct {
	// dependências específicas do legacy (binários, paths, etc)
}

func New() *Provider { return &Provider{} }

func (p *Provider) Provision(cfg contract.InfraConfig, _ *contract.Cluster) (*contract.Cluster, error) {
	resources.LogLevel("info", "Start provisioning with legacy infrastructure for %s", cfg.Product)
	// ... lógica atual de provisioning legacy (antes: legacy.Provision)
	// retorne *contract.Cluster
	return &contract.Cluster{ /* ... */ }, nil
}

func (p *Provider) Destroy(c *contract.Cluster) error {
	// ... lógica atual de destroy legacy
	return nil
}
