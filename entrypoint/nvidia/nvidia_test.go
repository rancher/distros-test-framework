package nvidia

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
			assert.PodAssertReady())
	})

	It("Validate Nvidia GPU functionality", func() {
		testcase.TestNvidiaGPUFunctionality(cluster, flags.Nvidia.Version)
	})

	It("Validate Nvidia Unprivileged Pod", func() {
		testcase.TestNvidiaUnprivilegedPod()
	})

	It("Validate Nvidia Privileged Pod", func() {
		testcase.TestNvidiaPrivilegedPod()
	})

	It("Validate Nodes after nvidia test", func() {
		testcase.TestNodeStatus(
			cluster,
			assert.NodeAssertReadyStatus(),
			nil,
		)
	})

	It("Validate Pods after nvidia test", func() {
		testcase.TestPodStatus(
			cluster,
			nil,
			assert.PodAssertReady())
	})
})

var _ = AfterEach(func() {
	if CurrentSpecReport().Failed() {
		fmt.Printf("\nFAILED! %s\n", CurrentSpecReport().FullText())
	} else {
		fmt.Printf("\nPASSED! %s\n", CurrentSpecReport().FullText())
	}
})
