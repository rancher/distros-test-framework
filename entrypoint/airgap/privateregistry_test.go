package airgap

import (
	"fmt"

	"github.com/rancher/distros-test-framework/pkg/assert"
	"github.com/rancher/distros-test-framework/pkg/testcase"

	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Test Airgap Cluster with Private Registry:", func() {
	It("Creates bastion and private nodes", func() {
		testcase.TestBuildPrivateCluster(cluster)
	})

	It("Installs product on private nodes", func() {
		testcase.TestPrivateRegistry(cluster, flags)
	})

	It("Validates Private Nodes", func() {
		testcase.TestPrivateNodeStatus(
			cluster,
			assert.NodeAssertReadyStatus(),
			nil,
		)
	})

	It("Validates Private Pods", func() {
		testcase.TestPrivatePodStatus(
			cluster,
			assert.PodAssertRestart(),
			assert.PodAssertReady())
	})

	It("Displays cluster details", func() {
		testcase.DisplayClusterInfo(cluster)
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
