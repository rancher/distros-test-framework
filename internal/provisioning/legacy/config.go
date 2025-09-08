package legacy

import (
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/gruntwork-io/terratest/modules/terraform"

	"github.com/rancher/distros-test-framework/internal/pkg/customflag"
	"github.com/rancher/distros-test-framework/internal/provisioning/driver"
	"github.com/rancher/distros-test-framework/internal/resources"
)

var (
	once    sync.Once
	cluster *driver.Cluster
)

// ClusterConfig returns a singleton cluster with all terraform config and vars.
func (p *Provisioner) clusterConfig(product, module string) *driver.Cluster {
	resources.LogLevel("info", "Start provisioning with legacy infrastructure for %s", product)

	once.Do(func() {
		var err error
		cluster, err = p.newCluster(product, module)
		if err != nil {
			resources.LogLevel("error", "error getting cluster: %w\n", err)
			if customflag.ServiceFlag.Destroy {
				resources.LogLevel("info", "\nmoving to start destroy operation\n")
				status, destroyErr := p.destroyLegacyInfra(product, module)
				if destroyErr != nil {
					resources.LogLevel("error", "error destroying cluster: %w\n", destroyErr)
					os.Exit(1)
				}
				if status != "cluster destroyed" {
					resources.LogLevel("error", "cluster not destroyed: %s\n", status)
					os.Exit(1)
				}
			}
			os.Exit(1)
		}
	})

	return cluster
}

func (*Provisioner) destroyLegacyInfra(product, module string) (string, error) {
	terraformOptions, _, err := setTerraformOptions(product, module)
	if err != nil {
		return "", fmt.Errorf("error setting terraform options for destroying cluster resources: %w", err)
	}
	terraform.Destroy(&testing.T{}, terraformOptions)

	return "cluster destroyed", nil
}
