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
		resultChan <- fmt.Errorf("mismatched length commands x expected values:"+
			" %s x %s", cmds, expectedValues)
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
		for _, testMap := range testCombination.Run {

			cmds := strings.Split(testMap.Cmd, ",")
			expectedValues := strings.Split(testMap.ExpectedValue, ",")

			ipsToUse := ips

			if strings.Contains(testMap.Cmd, "etcd") {

				cmdToGetIps := fmt.Sprintf(`
				kubectl get node -A -o wide --kubeconfig="%s" \
				| grep 'etcd' | awk '{print $7}'
				`,
					shared.KubeConfigFile)

				var nodes []string
				nodeIps, err := shared.RunCommandHost(cmdToGetIps)
				if err != nil {
					return
				}

				n := strings.Split(nodeIps, "\n")
				for _, nodeIP := range n {
					if nodeIP != "" {
						nodes = append(nodes, nodeIP)
					}
				}

				ipsToUse = nodes
			}

			for _, ip := range ipsToUse {
				processCmds(resultChan, wg, ip, cmds, expectedValues)
			}
		}
	}
}

// processOnNode runs the test on the node calling ValidateOnNode.
func processOnNode(resultChan chan error, ip, cmd, expectedValue string) {
	var version string
	var err error

	if expectedValue == "" {
		err = fmt.Errorf("\nerror: expected value should be sent to node")
		resultChan <- err
		close(resultChan)
		return
	}

	product, err := shared.GetProduct()
	if err != nil {
		fmt.Println(err)
		resultChan <- err
		close(resultChan)
		return
	}

	version, err = shared.GetProductVersion(product)
	if err != nil {
		fmt.Println(err)
		resultChan <- err
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
			resultChan <- err
			close(resultChan)
			return
		}
	}
}

// processOnHost runs the test on the host calling ValidateOnHost.
func processOnHost(resultChan chan error, ip, cmd, expectedValue string) {
	var version string
	var err error

	if expectedValue == "" {
		err = fmt.Errorf("\nerror: expected value should be sent to host")
		resultChan <- err
		close(resultChan)
		return
	}

	kubeconfigFlag := " --kubeconfig=" + shared.KubeConfigFile
	fullCmd := shared.JoinCommands(cmd, kubeconfigFlag)

	product, err := shared.GetProduct()
	if err != nil {
		fmt.Println(err)
		resultChan <- err
		close(resultChan)
		return
	}

	version, err = shared.GetProductVersion(product)
	if err != nil {
		fmt.Println(err)
		resultChan <- err
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
		resultChan <- err
		close(resultChan)
		return
	}
}
