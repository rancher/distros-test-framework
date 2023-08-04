package testcase

import (
	"fmt"

	"github.com/rancher/distros-test-framework/factory"
	"github.com/rancher/distros-test-framework/pkg/assert"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// TestNodeStatus test the status of the nodes in the cluster using 2 custom assert functions
func TestNodeStatus(
	nodeAssertReadyStatus assert.NodeAssertFunc,
	nodeAssertVersion assert.NodeAssertFunc,
) {
	cluster := factory.GetCluster(GinkgoT())
	expectedNodeCount := cluster.NumServers + cluster.NumAgents 

	if cluster.ProductType == "rke2" {
		expectedNodeCount += cluster.NumWinAgents
	}

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
	}, "1200s", "10s").Should(Succeed())

	fmt.Println("\n\nCluster nodes:")
	_, err := shared.ParseNodes(true)
	Expect(err).NotTo(HaveOccurred())
}
