package template

import (
	"fmt"
	"strings"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	"github.com/rancher/distros-test-framework/pkg/assert"
	"github.com/rancher/distros-test-framework/shared"
)

// processCmds runs the tests per ips using processOnNode and processOnHost validation.
//
// it will spawn a go routine per testCombination and ip.
func processCmds(resultChan chan error, wg *sync.WaitGroup, ip string, cmds []string, expectedValues []string) {
	if len(cmds) != len(expectedValues) {
		resultChan <- shared.ReturnLogError("mismatched length commands x expected values:"+
			" %s x %s", cmds, expectedValues)
		return
	}

	if expectedValues[0] == "" || cmds[0] == "" {
		resultChan <- shared.ReturnLogError("error: command and/or expected value was not sent")
		close(resultChan)
		return
	}

	for i := range cmds {
		cmd := cmds[i]
		expectedValue := expectedValues[i]

		wg.Add(1)
		go func(ip string, cmd, expectedValue string) {
			defer wg.Done()
			defer GinkgoRecover()

			if strings.Contains(cmd, "kubectl") || strings.HasPrefix(cmd, "helm") {
				processOnHost(resultChan, ip, cmd, expectedValue)
			} else {
				processOnNode(resultChan, ip, cmd, expectedValue)
			}
		}(ip, cmd, expectedValue)
	}
}

func processTestCombination(resultChan chan error, wg *sync.WaitGroup, ips []string, testCombination RunCmd) {
	if testCombination.Run != nil {
		for _, ip := range ips {
			for _, testMap := range testCombination.Run {
				cmds := strings.Split(testMap.Cmd, ",")
				expectedValues := strings.Split(testMap.ExpectedValue, ",")
				processCmds(resultChan, wg, ip, cmds, expectedValues)
			}
		}
	}
}

// processOnNode runs the test on the node calling ValidateOnNode.
func processOnNode(resultChan chan error, ip, cmd, expectedValue string) {
	var version string
	var err error

	product, err := shared.GetProduct()
	if err != nil {
		resultChan <- shared.ReturnLogError("failed to get product: %v", err)
		close(resultChan)
		return
	}

	version, err = shared.GetProductVersion(product)
	if err != nil {
		resultChan <- shared.ReturnLogError("failed to get product version: %v", err)
		close(resultChan)
		return
	}

	fmt.Printf("\n---------------------\n"+
		"Version Checked: %s\n"+
		"IP Address: %s\n"+
		"Command Executed: %s\n"+
		"Execution Location: Node\n"+
		"Expected Value: %s\n---------------------\n",
		version, ip, cmd, expectedValue)

	cmds := strings.Split(cmd, ",")
	for _, c := range cmds {
		err = assert.ValidateOnNode(
			ip,
			c,
			expectedValue,
		)
		if err != nil {
			resultChan <- shared.ReturnLogError("failed to validate on node: %v", err)
			close(resultChan)
			return
		}
	}
}

// processOnHost runs the test on the host calling ValidateOnHost.
func processOnHost(resultChan chan error, ip, cmd, expectedValue string) {
	var version string
	var err error

	kubeconfigFlag := " --kubeconfig=" + shared.KubeConfigFile
	fullCmd := shared.JoinCommands(cmd, kubeconfigFlag)

	product, err := shared.GetProduct()
	if err != nil {
		resultChan <- shared.ReturnLogError("failed to get product: %v", err)
		close(resultChan)
		return
	}

	version, err = shared.GetProductVersion(product)
	if err != nil {
		resultChan <- shared.ReturnLogError("failed to get product version: %v", err)
		close(resultChan)
		return
	}

	fmt.Printf("\n---------------------\n"+
		"Version Checked: %s\n"+
		"IP Address: %s\n"+
		"Command Executed: %s\n"+
		"Execution Location: Host\n"+
		"Expected Value: %s\n---------------------\n",
		version, ip, cmd, expectedValue)

	err = assert.ValidateOnHost(
		fullCmd,
		expectedValue,
	)
	if err != nil {
		resultChan <- shared.ReturnLogError("failed to validate on host: %v", err)
		close(resultChan)
		return
	}
}
