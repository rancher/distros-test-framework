//go:build tarball

package airgap

import (
	"fmt"

	"github.com/rancher/distros-test-framework/internal/pkg/assert"
	"github.com/rancher/distros-test-framework/internal/pkg/testcase"
	"github.com/rancher/distros-test-framework/internal/pkg/testcase/support"

	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Test Airgap cluster using Tarball Method:", Ordered, func() {
	BeforeAll(func() {
		support.BuildAirgapCluster(cluster)
	})

	It("Installs product on airgapped nodes", func() {
		testcase.TestTarball(cluster, flags)
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

	// TODO: Validate deployment, eg: cluster-ip
})

var _ = AfterEach(func() {
	if CurrentSpecReport().Failed() {
		fmt.Printf("\nFAILED! %s\n", CurrentSpecReport().FullText())
	} else {
		fmt.Printf("\nPASSED! %s\n", CurrentSpecReport().FullText())
	}
})
