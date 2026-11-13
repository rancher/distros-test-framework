package qainfra

import (
	"fmt"
	"strings"
)

const nestedMarker = "# __NESTED_VIRTUALIZATION_MODULE_ARGS__"

func injectNestedVirtualization(content, provider, value string) (string, error) {
	if value != "" && value != "false" && value != "true" {
		return "", fmt.Errorf("QA_INFRA_WORKER_NESTED must be true or false, got %q", value)
	}
	arg := ""
	if value == "true" {
		if provider != "aws" || !strings.Contains(content, nestedMarker) {
			return "", fmt.Errorf("nested virtualization requires the AWS module template with %s", nestedMarker)
		}
		arg = "worker_nested_virtualization = true"
	}

	return strings.ReplaceAll(content, nestedMarker, arg), nil
}
