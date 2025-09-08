package provisioning

import (
	"fmt"

	"github.com/rancher/distros-test-framework/internal/provisioning/driver"
	"github.com/rancher/distros-test-framework/internal/provisioning/legacy"
	"github.com/rancher/distros-test-framework/internal/provisioning/qainfra"
)

func ProvisionInfrastructure(infra *driver.InfraConfig) (*driver.Cluster, error) {
	provisioner := infra.ProvisionerModule
	if provisioner == "" {
		provisioner = "legacy"
	}

	providers := map[string]driver.Provisioner{
		"legacy":  legacy.New(),
		"qainfra": qainfra.New(),
	}

	prov, ok := providers[provisioner]
	if !ok {
		return nil, fmt.Errorf("unknown infrastructure provider: %s", provisioner)
	}

	return prov.Provision(infra)
}

func DestroyInfrastructure(provisioner, product, module string) (string, error) {
	if provisioner == "" {
		provisioner = "legacy"
	}

	provisioners := map[string]driver.Provisioner{
		"legacy":  legacy.New(),
		"qainfra": qainfra.New(),
	}

	prov, ok := provisioners[provisioner]
	if !ok {
		return "", fmt.Errorf("unknown infrastructure provider: %s", provisioner)
	}

	return prov.Destroy(product, module)
}
