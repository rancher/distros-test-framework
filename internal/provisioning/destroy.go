package provisioning

import (
	"fmt"
)

// DestroyInfrastructure destroys infrastructure using the configured provider
func DestroyInfrastructure(product, module string) (string, error) {
	switch provider {
	case "legacy", "":
		return destroyLegacy(product, module)
	case "qa-infra":
		return destroyQAInfra()
	default:
		return "", fmt.Errorf("unknown infrastructure provider: %s", provider)
	}
}
