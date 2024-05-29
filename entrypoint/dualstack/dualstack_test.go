package dualstack

import (
	"fmt"

	"github.com/rancher/distros-test-framework/pkg/assert"
	"github.com/rancher/distros-test-framework/pkg/testcase"

	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Test:", func() {
	It("Start Up with no issues", func() {
		testcase.TestBuildCluster(cluster)
	})

	It("Validate Nodes", func() {
		testcase.TestNodeStatus(
			cluster,
			assert.NodeAssertReadyStatus(),
			nil,
		)
	})

	It("Validate Pods", func() {
		testcase.TestPodStatus(
			cluster,
			assert.PodAssertRestart(),
			assert.PodAssertReady(),
			assert.PodAssertStatus(),
		)
	})

	It("Validate Ingress Service in Dual-Stack", func() {
		testcase.TestIngressDualStack(cluster, false)
	})

	It("Validate NodePort Service in Dual-Stack", func() {
		testcase.TestNodePort(cluster, false)
	})

	It("Validate ClusterIPs in CIDR range in Dual-Stack", func() {
		testcase.TestClusterIPsInCIDRRange(cluster, true)
	})

	It("Validate Single and Dual-Stack IPFamilies in Dual-Stack", func() {
		testcase.TestIPFamiliesDualStack(true)
	})

	// https://github.com/k3s-io/k3s/issues/10053
	It("[k3s/10053] Validates Ingress after Pod Restart when Network Policies are present", func() {
		testcase.TestIngressWithPodRestartAndNetPol(true)
	})
})

var _ = AfterEach(func() {
	if CurrentSpecReport().Failed() {
		fmt.Printf("\nFAILED! %s\n", CurrentSpecReport().FullText())
	} else {
		fmt.Printf("\nPASSED! %s\n", CurrentSpecReport().FullText())
	}
})
