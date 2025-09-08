package ipv6only

import (
	"fmt"

	"github.com/rancher/distros-test-framework/internal/pkg/assert"
	"github.com/rancher/distros-test-framework/internal/pkg/testcase"
	"github.com/rancher/distros-test-framework/internal/pkg/testcase/support"

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
		testcase.TestNodeStatusUsingBastion(
			cluster,
			assert.NodeAssertReadyStatus(),
			nil,
		)
	})

	It("Validates Pods", func() {
		testcase.TestPodStatusUsingBastion(
			cluster,
			assert.PodAssertRestart(),
			assert.PodAssertReady())
	})

	AfterAll(func() {
		support.LogClusterInfoUsingBastion(cluster)
	})
})

var _ = AfterEach(func() {
	if CurrentSpecReport().Failed() {
		fmt.Printf("\nFAILED! %s\n\n", CurrentSpecReport().FullText())
	} else {
		fmt.Printf("\nPASSED! %s\n\n", CurrentSpecReport().FullText())
	}
})
