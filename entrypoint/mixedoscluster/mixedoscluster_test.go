package mixedoscluster

import (
	"fmt"

	"github.com/rancher/distros-test-framework/pkg/assert"
	"github.com/rancher/distros-test-framework/pkg/testcase"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Test: Mixed OS Cluster", func() {

	It("Starts Up with no issues", func() {
		testcase.TestBuildCluster(GinkgoT())
	})

	It("Validates Node", func() {
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

	It("Validates internode connectivity over the vxlan tunnel", func() {
		testcase.TestInternodeConnectivityMixedOS()
	})

	It("Validates cluster by running sonobuoy mixed OS plugin", func() {
		testcase.TestSonobuoyMixedOS(sonobuoyVersion, true)
		shared.ManageWorkload("delete", "",
			"pod_client.yaml","windows_app_deployment.yaml")
	})
})

var _ = AfterEach(func() {
	if CurrentSpecReport().Failed() {
		fmt.Printf("\nFAILED! %s\n", CurrentSpecReport().FullText())
	} else {
		fmt.Printf("\nPASSED! %s\n", CurrentSpecReport().FullText())
	}
})