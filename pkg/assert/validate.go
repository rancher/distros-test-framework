package assert

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/rancher/distros-test-framework/shared"
)

type testResult struct {
	Command   string
	Assertion string
	Result    string
}

var (
	Results []testResult
	mutex   sync.Mutex
)

// validate calls runAssertion for each cmd/assert pair.
func validate(exec func(string) (string, error), args ...string) error {
	if len(args) < 2 || len(args)%2 != 0 {
		return shared.ReturnLogError("should send even number of args")
	}

	timeout := time.After(120 * time.Second)
	ticker := time.NewTicker(5 * time.Second)

	for i := 0; i < len(args); i++ {
		cmd := args[i]
		if i+1 < len(args) {
			assert := args[i+1]
			i++
			if assert == "" || cmd == "" {
				return shared.ReturnLogError("should not send empty arg for assert:%s "+"and/or cmd:%s", assert, cmd)
			}

			err := runAssertion(cmd, assert, exec, ticker.C, timeout)
			if err != nil {
				return shared.ReturnLogError("error from runAssertion():\n %v\n", err)
			}
		}
	}

	return nil
}

// runAssertion runs a command and asserts that the value received against his respective command.
func runAssertion(
	cmd, assert string,
	exec func(string) (string, error),
	ticker <-chan time.Time,
	timeout <-chan time.Time,
) error {
	var (
		res   string
		err   error
		retry int
	)

	for {
		select {
		case <-timeout:
			timeoutErr := shared.ReturnLogError("timeout reached for command:\n%s\n"+
				"Trying to assert with:\n%s\nExpected value: %s\n", cmd, res, assert)

			return timeoutErr
		case <-ticker:
			retry++
			res, err = exec(cmd)
			if err != nil {
				if retry == 0 || retry == 4 {
					shared.LogLevel("warn", "error from exec runAssertion: %v\n"+
						"Retrying command: %s\n on retry count: %d\n", err, cmd, retry)
				}
				if retry > 5 {
					return shared.ReturnLogError("error from exec runAssertion after 5 retries: %v\n", err)
				}

				continue
			}

			res = strings.TrimSpace(res)
			if strings.Contains(res, assert) {
				fmt.Printf("\nCommand:\n"+
					"%s"+
					"\n---------------------\nAssertion:\n"+
					"%s"+
					"\n----------------------\nMatched with result:\n%s\n", cmd, assert, res)
				addResult(cmd, assert, res)

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

func addResult(command, assertion, result string) {
	mutex.Lock()
	defer mutex.Unlock()

	Results = append(Results, testResult{Command: "\nCommand:\n" + command + "\n", Assertion: "\nAssertion:\n" +
		assertion + "\n", Result: "\nMatched with result:\n" + result + "\n"})
}
