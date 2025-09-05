package assert

import (
	"fmt"
	"strings"

	"github.com/rancher/distros-test-framework/internal/resources"

	. "github.com/onsi/gomega"
)

// CheckComponentCmdHost runs a command on the host and asserts that the value received contains the specified substring.
//
// You can send multiple asserts from a cmd but all of them must be true.
//
// Need to send KubeconfigFile.
func CheckComponentCmdHost(cmd string, asserts ...string) error {
	if cmd == "" {
		return fmt.Errorf("cmd: %s should not be sent empty", cmd)
	}
	Eventually(func() error {
		res, err := resources.RunCommandHost(cmd)
		cleanRes := resources.CleanString(res)

		Expect(err).ToNot(HaveOccurred())
		for _, assert := range asserts {
			if assert == "" {
				return fmt.Errorf("assert: %s should not be sent empty", assert)
			}

			if !strings.Contains(cleanRes, assert) {
				return fmt.Errorf("expected substring %q not found in result %q", assert, res)
			}

			resources.LogLevel("info", "Result: %s\nMatched with: %s\n", res, assert)
		}

		return nil
	}, "420s", "5s").Should(Succeed())

	return nil
}
