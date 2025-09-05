package legacy

import (
	"os"
	"sync"
	"testing"

	"github.com/gruntwork-io/terratest/modules/terraform"

	"github.com/rancher/distros-test-framework/internal/pkg/customflag"
	"github.com/rancher/distros-test-framework/internal/provisioning"
	"github.com/rancher/distros-test-framework/internal/resources"
)

var (
	once    sync.Once
	cluster *provisioning.Cluster
)

// ClusterConfig returns a singleton cluster with all terraform config and vars.
func ClusterConfig(product, module string) *provisioning.Cluster {
	once.Do(func() {
		var err error
		cluster, err = newCluster(product, module)
		if err != nil {
			resources.LogLevel("error", "error getting cluster: %w\n", err)
			if customflag.ServiceFlag.Destroy {
				resources.LogLevel("info", "\nmoving to start destroy operation\n")
				status, destroyErr := destroyLegacyInfra(product, module)
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

func destroyLegacyInfra(product string, module string) (interface{}, interface{}) {
	terraformOptions, _, err := setTerraformOptions(product, module)
	if err != nil {
		return "", err
	}
	terraform.Destroy(&testing.T{}, terraformOptions)

	return "cluster destroyed", nil
}
