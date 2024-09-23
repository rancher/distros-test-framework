package testcase

import (
	"fmt"

	"github.com/rancher/distros-test-framework/pkg/assert"
	"github.com/rancher/distros-test-framework/pkg/testcase/support"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/gomega"
)

// TestNodeStatus test the status of the nodes in the cluster using 2 custom assert functions.
func TestNodeStatus(
	cluster *shared.Cluster,
	nodeAssertReadyStatus assert.NodeAssertFunc,
	nodeAssertVersion assert.NodeAssertFunc,
) {
	expectedNodeCount := cluster.NumServers + cluster.NumAgents

	if cluster.Config.Product == "rke2" {
		expectedNodeCount += cluster.NumWinAgents
	}

	Eventually(func(g Gomega) bool {
		nodes, err := shared.GetNodes(false)
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

		return true
	}, "3600s", "10s").Should(BeTrue(), func() string {
		shared.LogLevel("error", "\ntimeout for nodes to be ready; gathering journal logs...\n")
		logs := shared.GetJournalLogs("error", cluster.ServerIPs[0])

		if cluster.NumAgents > 0 {
			logs += shared.GetJournalLogs("error", cluster.AgentIPs[0])
		}

		return logs
	})

	_, err := shared.GetNodes(true)
	Expect(err).NotTo(HaveOccurred())
}

// TestPrivateNodeStatus test the status of the nodes in the private cluster using 2 custom assert functions.
func TestPrivateNodeStatus(
	cluster *shared.Cluster,
	nodeAssertReadyStatus assert.NodeAssertFunc,
	nodeAssertVersion assert.NodeAssertFunc,
) {
	expectedNodeCount := cluster.NumServers + cluster.NumAgents

	if cluster.Config.Product == "rke2" {
		expectedNodeCount += cluster.NumWinAgents
	}

	var nodeDetails string
	var err error
	Eventually(func(g Gomega) bool {
		nodeDetails, err = getPrivateNodes(cluster)
		Expect(err).To(BeNil())
		nodes := shared.ParseNodes(nodeDetails)
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

		return true
	}, "3600s", "10s").Should(BeTrue())
}

func getPrivateNodes(cluster *shared.Cluster) (nodeDetails string, err error) {
	cmd := fmt.Sprintf(
		"PATH=$PATH:/var/lib/rancher/%[1]v/bin:/opt/%[1]v/bin; "+
			"KUBECONFIG=/etc/rancher/%[1]v/%[1]v.yaml ",
		cluster.Config.Product)
	cmd += "kubectl get nodes -o wide --no-headers"
	nodeDetails, err = support.CmdForPrivateNode(cluster, cmd, cluster.ServerIPs[0])

	return nodeDetails, err
}
