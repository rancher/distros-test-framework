package fixture

import (
	"fmt"

	"github.com/rancher/distros-test-framework/lib/assert"
	"github.com/rancher/distros-test-framework/lib/cluster"
	"github.com/rancher/distros-test-framework/lib/shared"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// TestNodeStatus test the status of the nodes in the cluster using 2 custom assert functions
func TestNodeStatus(
	nodeAssertReadyStatus assert.NodeAssertFunc,
	nodeAssertVersion assert.NodeAssertFunc,
	product string,
) {
	cluster := activity.GetCluster(GinkgoT(), product)
	fmt.Println("\nFetching node status")

	expectedNodeCount := cluster.NumServers + cluster.NumAgents
	Eventually(func(g Gomega) {
		nodes, err := shared.ParseNodes(false)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(len(nodes)).To(Equal(expectedNodeCount),
			"Number of nodes should match the spec")

		for _, node := range nodes {
			if nodeAssertReadyStatus != nil {
				nodeAssertReadyStatus(g, node)
			}
			if nodeAssertVersion != nil {
				nodeAssertVersion(g, node)

			}
		}
	}, "800s", "30s").Should(Succeed())

	_, err := shared.ParseNodes(true)
	Expect(err).NotTo(HaveOccurred())
}
