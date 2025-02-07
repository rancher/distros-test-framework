package ipv6only

import (
	"fmt"

	"github.com/rancher/distros-test-framework/pkg/assert"
	"github.com/rancher/distros-test-framework/pkg/testcase"
	"github.com/rancher/distros-test-framework/pkg/testcase/support"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Test ipv6 only cluster:", Ordered, func() {
	BeforeAll(func() {
		support.BuildIPv6OnlyCluster(cluster)
	})

	It("Install product on ipv6 only nodes", func() {
		testcase.TestIPv6Only(cluster, awsClient)
	})

	It("Validates Nodes", func() {
		testcase.TestAirgapClusterNodeStatus(
			cluster,
			assert.NodeAssertReadyStatus(),
			nil,
		)
	})

	It("Validates Pods", func() {
		testcase.TestAirgapClusterPodStatus(
			cluster,
			assert.PodAssertRestart(),
			assert.PodAssertReady())
	})

	AfterAll(func()  {
		shared.DisplayAirgapClusterDetails(cluster)
	})
})

var _ = AfterEach(func() {
	if CurrentSpecReport().Failed() {
		fmt.Printf("\nFAILED! %s\n\n", CurrentSpecReport().FullText())
	} else {
		fmt.Printf("\nPASSED! %s\n\n", CurrentSpecReport().FullText())
	}
})
