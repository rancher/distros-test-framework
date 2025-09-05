package provisioning

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/rancher/distros-test-framework/internal/logging"
	"github.com/rancher/distros-test-framework/internal/resources"
)

// LegacyProvider implements the Provider interface for legacy terraform modules
type LegacyProvider struct{}

// InfraProvisioner defines the interface for infrastructure providers
type InfraProvisioner interface {
	Provision(config Config, c *resources.Cluster) (*resources.Cluster, error)
	Destroy(product, module string) (string, error)
}

// ProvisionInfrastructure provisions infrastructure using the configured provider.
// If the provider is not set, it will default to legacy.
func ProvisionInfrastructure(infra Config, c *resources.Cluster) (*resources.Cluster, error) {
	switch infra.Provisioner {
	case "legacy", "":
		resources.LogLevel("info", "Start provisioning with legacy infrastructure for %s", infra.Product)
		provisioner := &LegacyProvider{}
		return provisioner.Provision(infra, c)
	case "qa-infra":
		logging.LogLevel("info", "Start provisioning with qa-infra infrastructure for %s", infra.Product)
		provisioner := &QAInfraProvider{}
		return provisioner.Provision(infra, c)
	default:
		return nil, fmt.Errorf("unknown infrastructure provider: %s", infra.Provisioner)
	}
}
