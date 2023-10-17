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
	cluster := factory.AddCluster(GinkgoT())
	expectedNodeCount := cluster.NumServers + cluster.NumAgents

	if cluster.Config.Product == "rke2" {
		expectedNodeCount += cluster.NumWinAgents
	}

	Eventually(func(g Gomega) {
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
	}, "1500s", "20s").Should(Succeed())

	fmt.Println("\n\nCluster nodes:")
	_, err := shared.GetNodes(true)
	Expect(err).NotTo(HaveOccurred())
}

func TestKillall(product, ip string) {
	_, _ = RunCommandOnNode(fmt.Sprintf("%s*-killall.sh", product), ip)
}

func StopServer(product, ip string) {
	_, _ = RunCommandOnNode(fmt.Sprintf("sudo systemctl stop %s*", product), ip)
}

func StartServer(product, ip string) {
	_, _ = RunCommandOnNode(fmt.Sprintf("sudo systemctl start %s*", product), ip)
}

// Deletes the Database Directories
func DeleteDatabaseDirectories(product, ip string) {
	_, _ = RunCommandOnNode(fmt.Sprintf("sudo rm -rf /var/lib/rancher/%s*/server/db", product), ip)
}
