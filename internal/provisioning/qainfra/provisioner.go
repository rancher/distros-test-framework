package qainfra

import (
	"fmt"
	"os"
	"strings"

	"github.com/rancher/distros-test-framework/internal/provisioning/driver"
	"github.com/rancher/distros-test-framework/internal/resources"
)

// Provisioner implements the driver.Provisioner interface for qainfra infrastructure.
type Provisioner struct{}

func New() *Provisioner { return &Provisioner{} }

func (p *Provisioner) Provision(cfg *driver.InfraConfig) (*driver.Cluster, error) {
	return p.provisionInfrastructure(cfg)
}

func (p *Provisioner) Destroy(product, module string) (string, error) {
	return p.destroyInfrastructure(product, module)
}

// executeInfraProvisioner selects and runs the infrastructure provisioner based on env var.
// Current support: OpenTofu (default).
func executeInfraProvisioner(config *driver.InfraConfig) error {
	provisioner := strings.ToLower(strings.TrimSpace(os.Getenv("INFRA_PROVISIONER")))
	if provisioner == "" {
		provisioner = "opentofu"
	}

	resources.LogLevel("info", "Using infra provisioner: %s", provisioner)

	switch provisioner {
	case "opentofu", "tofu", "terraform", "tf":
		return executeOpenTofuOperations(config)
	default:
		return fmt.Errorf("unsupported INFRA_PROVISIONER=%s (only 'opentofu' supported)", provisioner)
	}
}
