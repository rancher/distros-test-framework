package sonobuoyconformance

import (
	"fmt"

	"github.com/rancher/distros-test-framework/internal/pkg/assert"
	"github.com/rancher/distros-test-framework/internal/pkg/testcase"

	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Sonobuoy Conformance Tests...", func() {
	It("Starts Up with no issues", func() {
		testcase.TestBuildCluster(cluster)
	})

	It("Validates Node", func() {
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

	It("Validates the releases conformance with upstream requirements", func() {
		testcase.TestConformance(flags.External.SonobuoyVersion)
	})
})

var _ = AfterEach(func() {
	if CurrentSpecReport().Failed() {
		fmt.Printf("\nFAILED! %s\n\n", CurrentSpecReport().FullText())
	} else {
		fmt.Printf("\nPASSED! %s\n\n", CurrentSpecReport().FullText())
	}
})
