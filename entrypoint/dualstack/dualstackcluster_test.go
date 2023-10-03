package dualstack

import (
	"fmt"

	"github.com/rancher/distros-test-framework/pkg/assert"
	"github.com/rancher/distros-test-framework/pkg/testcase"

	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Test:", func() {
	It("Start Up with no issues", func() {
		testcase.TestBuildCluster(GinkgoT())
	})

	It("Validate Nodes", func() {
		testcase.TestNodeStatus(
			assert.NodeAssertReadyStatus(),
			nil,
		)
	})

	It("Validate Pods", func() {
		testcase.TestPodStatus(
			assert.PodAssertRestart(),
			assert.PodAssertReady(),
			assert.PodAssertStatus(),
		)
	})

	It("Validate Dual-Stack Ingress Service", func() {
		testcase.TestDualStackIngress(true)
	})

	It("Validate Dual-Stack NodePort Service", func() {
		testcase.TestDualStackNodePort(true)
	})

	It("Validate Dual-Stack ClusterIPs in CIDR range", func() {
		testcase.TestDualStackClusterIPsInCIDRRange(true)
	})

	It("Validate Single and Dual-Stack IPFamilies", func() {
		testcase.TestDualStackIPFamilies(true)
	})
})

var _ = AfterEach(func() {
	if CurrentSpecReport().Failed() {
		fmt.Printf("\nFAILED! %s\n", CurrentSpecReport().FullText())
	} else {
		fmt.Printf("\nPASSED! %s\n", CurrentSpecReport().FullText())
	}
})
