package assert

import (
	"fmt"
	"strings"
	"time"

	"github.com/rancher/distros-test-framework/shared"
)

// validate calls runAssertion for each cmd/assert pair
func validate(exec func(string) (string, error), args ...string) error {
	if len(args) < 2 || len(args)%2 != 0 {
		return shared.ReturnLogError("should send even number of args")
	}

	errorsChan := make(chan error, len(args)/2)
	timeout := time.After(420 * time.Second)
	ticker := time.NewTicker(3 * time.Second)

	for i := 0; i < len(args); i++ {
		cmd := args[i]
		if i+1 < len(args) {
			assert := args[i+1]
			i++

			if assert == "" || cmd == "" {
				return shared.ReturnLogError("should not send empty arg for assert:%s "+
					"and/or cmd:%s",
					assert, cmd)
			}
			err := runAssertion(cmd, assert, exec, ticker.C, timeout, errorsChan)
			if err != nil {
				shared.LogLevel("error", "error from runAssertion():\n %s\n", err)
				close(errorsChan)
				return err
			}
		}
	}
	close(errorsChan)

	return nil
}

// runAssertion runs a command and asserts that the value received against his respective command
func runAssertion(
	cmd, assert string,
	exec func(string) (string, error),
	ticker <-chan time.Time,
	timeout <-chan time.Time,
	errorsChan chan<- error,
) error {
	for {
		res, err := exec(cmd)
		if err != nil {
			errorsChan <- err
			return fmt.Errorf("error from runCmd: %s\n %s", cmd, res)
		}

		select {
		case <-timeout:
			timeoutErr := shared.ReturnLogError("timeout reached for command:\n%s\n "+
				"Trying to assert with:\n %s",
				cmd, res)
			errorsChan <- timeoutErr
			return timeoutErr

		case <-ticker:
			if strings.Contains(res, assert) {
				fmt.Printf("\nCommand:\n"+
					"%s"+
					"\n---------------------\nAssertion:\n"+
					"%s"+
					"\n----------------------\nMatched with result:\n%s\n", cmd, assert, res)
				errorsChan <- nil
				return nil
			}
		}
	}
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

// ValidateOnNode runs an exec function on RunCommandOnNode and assert given is fulfilled.
// The last argument should be the assertion.
func ValidateOnNode(ip string, args ...string) error {
	exec := func(cmd string) (string, error) {
		return shared.RunCommandOnNode(cmd, ip)
	}
	return validate(exec, args...)
}
