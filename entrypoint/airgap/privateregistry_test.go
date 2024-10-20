//go:build privateregistry

package airgap

import (
	"fmt"

	"github.com/rancher/distros-test-framework/pkg/assert"
	"github.com/rancher/distros-test-framework/pkg/testcase"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Test Airgap cluster with Private Registry:", Ordered, func() {
	It("Creates bastion and airgapped nodes", func() {
		testcase.TestBuildAirgapCluster(cluster)
	})

	It("Installs product on airgapped nodes", func() {
		testcase.TestPrivateRegistry(cluster, flags)
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

	AfterAll(func() {
		shared.DisplayAirgapClusterDetails(cluster)
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
