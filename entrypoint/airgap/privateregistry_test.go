package airgap

import (
	"fmt"

	"github.com/rancher/distros-test-framework/pkg/testcase"

	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Test Airgap Cluster with Private Registry:", func() {
	It("Sets up private instances", func() {
		testcase.TestBuildPrivateCluster(cluster)
	})

	It("Deploys a private cluster", func() {
		testcase.TestAirgapPrivateRegistry(cluster, flags)
	})

	// Validate images available from private registry

	// Validate deployment, eg: cluster-ip

})

var _ = AfterEach(func() {
	if CurrentSpecReport().Failed() {
		fmt.Printf("\nFAILED! %s\n", CurrentSpecReport().FullText())
	} else {
		fmt.Printf("\nPASSED! %s\n", CurrentSpecReport().FullText())
	}
})
