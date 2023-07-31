package assert

import (
	"fmt"
	"strings"

	"github.com/rancher/distros-test-framework/lib/shared"

	. "github.com/onsi/gomega"
)

// CheckComponentCmdHost runs a command on the host and asserts that the value received contains the specified substring
//
// you can send multiple asserts from a cmd but all of them must be true
//
// need to send KubeconfigFile
func CheckComponentCmdHost(cmd string, asserts ...string) {
	Eventually(func() error {
		fmt.Println("Executing cmd: ", cmd)
		res, err := shared.RunCommandHost(cmd)
		if err != nil {
			return fmt.Errorf("error on RunCommandHost: %v", err)
		}

		for _, assert := range asserts {
			if !strings.Contains(res, assert) {
				return fmt.Errorf("expected substring %q not found in result %q", assert, res)
			}
			fmt.Println("Result:", res+"\nMatched with assert:\n", assert)
		}

		return nil
	}, "600s", "5s").Should(Succeed())
}

// ValidateOnHost runs an exec function on RunCommandHost and assert given is fulfilled.
// The last argument should be the assertion.
// Need to send kubeconfig file.
func ValidateOnHost(args ...string) error {
	exec := func(cmd string) (string, error) {
		return shared.RunCommandHost(cmd)
	}
	return validate(exec, args...)
}
