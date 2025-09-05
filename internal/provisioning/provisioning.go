package provisioning

import (
	"fmt"

	"github.com/rancher/distros-test-framework/internal/provisioning/contract"
	"github.com/rancher/distros-test-framework/internal/provisioning/legacy"
	"github.com/rancher/distros-test-framework/internal/provisioning/qainfra"
	"github.com/rancher/distros-test-framework/internal/resources"
)

// // ProvisionInfrastructure provisions infrastructure using the configured provider.
// // If the provider is not set, it will default to legacy.
// func ProvisionInfrastructure(infra qainfra.Config, c *Cluster) (*Cluster, error) {
// 	switch infra.Provisioner {
// 	case "legacy", "":
// 		resources.LogLevel("info", "Start provisioning with legacy infrastructure for %s", infra.Product)
//
// 		return legacy.Provision(infra.Product, infra.Module)
// 	case "qa-infra":
// 		resources.LogLevel("info", "Start provisioning with qa-infra infrastructure for %s", infra.Product)
//
// 		return qainfra.Provision(infra, c)
// 	default:
// 		return nil, fmt.Errorf("unknown infrastructure provider: %s", infra.Provisioner)
// 	}
// }

func ProvisionInfrastructure(infra contract.InfraConfig, c *contract.Cluster) (*contract.Cluster, error) {
	// default
	provKey := infra.Provisioner
	if provKey == "" {
		provKey = "legacy"
	}

	providers := map[string]contract.Provisioner{
		"legacy":   legacy.New(),
		"qa-infra": qainfra.New(),
	}

	p, ok := providers[provKey]
	if !ok {
		return nil, fmt.Errorf("unknown infrastructure provider: %s", provKey)
	}
	return p.Provision(infra, c)
}

func DestroyInfrastructure(infra contract.InfraConfig, c *contract.Cluster) error {
	provKey := infra.Provisioner
	if provKey == "" {
		provKey = "legacy"
	}
	providers := map[string]contract.Provisioner{
		"legacy":   legacy.New(),
		"qa-infra": qainfra.New(),
	}
	p, ok := providers[provKey]
	if !ok {
		return fmt.Errorf("unknown infrastructure provider: %s", provKey)
	}

	return p.Destroy(c)
}
