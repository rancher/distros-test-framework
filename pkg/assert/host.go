package assert

import (
	"fmt"
	"strings"

	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/gomega"
)

// CheckComponentCmdHost runs a command on the host and asserts that the value
// received contains the specified substring
// you can send multiple asserts from a cmd but all of them must be true
//
// need to send KubeconfigFile
func CheckComponentCmdHost(cmd string, asserts ...string) error {
	if cmd == "" {
		return fmt.Errorf("cmd: %s should not be sent empty", cmd)
	}
	Eventually(func() error {
		res, err := shared.RunCmdHost(cmd)
		Expect(err).ToNot(HaveOccurred())
		for _, assert := range asserts {
			if assert == "" {
				return fmt.Errorf("assert: %s should not be sent empty", assert)
			}
			if !strings.Contains(res, assert) {
				return fmt.Errorf("expected substring %q not found in result %q", assert, res)
			}

			fmt.Println("\nResult:", res+"\nMatched with:\n", assert)
		}
		return nil
	}, "420s", "5s").Should(Succeed())

	return nil
}
