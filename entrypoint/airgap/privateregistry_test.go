package airgap

import (
	"fmt"

	//"github.com/rancher/distros-test-framework/pkg/assert"
	"github.com/rancher/distros-test-framework/pkg/testcase"

	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Test:", func() {
	FIt("Sets up private instances", func() {
		testcase.TestBuildPrivateCluster(GinkgoT())
	})

	It("Deploys a private cluster", func() {
		testcase.TestAirgapPrivateRegistry()
	})

	// It("Validate Nodes", func() {
	// 	testcase.TestNodeStatus(
	// 		assert.NodeAssertReadyStatus(),
	// 		nil,
	// 	)
	// })

	// It("Validate Pods", func() {
	// 	testcase.TestPodStatus(
	// 		assert.PodAssertRestart(),
	// 		assert.PodAssertReady(),
	// 		assert.PodAssertStatus(),
	// 	)
	// })
})

var _ = AfterEach(func() {
	if CurrentSpecReport().Failed() {
		fmt.Printf("\nFAILED! %s\n", CurrentSpecReport().FullText())
	} else {
		fmt.Printf("\nPASSED! %s\n", CurrentSpecReport().FullText())
	}
})
