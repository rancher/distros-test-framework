package testcase

import (
	"strings"

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
	}, "600s", "10s").Should(BeTrue(), func() string {
		shared.LogLevel("error", "\nNodes are not in desired state")
		_, err := shared.GetNodes(true)
		Expect(err).NotTo(HaveOccurred())
		shared.LogLevel("info", "Journal logs from server node-1: %v\n", cluster.ServerIPs[0])
		logs := shared.GetJournalLogs("error", cluster.ServerIPs[0])
		shared.LogLevel("info", "Journal logs from agent node-1: %v\n", cluster.AgentIPs[0])
		if cluster.NumAgents > 0 {
			logs += shared.GetJournalLogs("error", cluster.AgentIPs[0])
		}

		return logs
	})

	_, err := shared.GetNodes(true)
	Expect(err).NotTo(HaveOccurred())
}

// TestNodeStatusUsingBastion test the status of the nodes in the private cluster using 2 custom assert functions.
func TestNodeStatusUsingBastion(
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
		nodeDetails, err = support.GetNodesViaBastion(cluster)
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
	}, "600s", "10s").Should(BeTrue())
}

func TestNodeMetricsServer(applyWorkload, deleteWorkload bool) {
	var workloadErr error
	if applyWorkload {
		workloadErr = shared.ManageWorkload("apply", "metrics-server.yaml")
		Expect(workloadErr).To(BeNil())
	}
	cmd := "kubectl get pods -n kube-system --kubeconfig=" + shared.KubeConfigFile + " | grep metrics-server"
	err := assert.ValidateOnHost(cmd, "Running")
	Expect(err).To(BeNil())

	topCmd := "kubectl top node --kubeconfig=" + shared.KubeConfigFile + " | grep CPU -A1 " +
		" && kubectl top pods -A --kubeconfig=" + shared.KubeConfigFile + " | grep CPU -A5 "
	res, err := shared.RunCommandHost(topCmd)
	Expect(err).To(BeNil())
	Expect(res).To(ContainSubstring("CPU"))
	Expect(res).To(ContainSubstring("MEMORY"))
	Expect(res).NotTo(Equal(""))
	Expect(res).NotTo(ContainSubstring("No resources found"))

	lines := strings.Split(res, "\n")
	dataLines := 0
	for _, line := range lines {
		if strings.Contains(line, "m") && strings.Contains(line, "Mi") {
			dataLines++
		}
		if strings.Contains(line, "g") && strings.Contains(line, "Gi") {
			dataLines++
		}
	}
	Expect(dataLines).To(BeNumerically(">", 0), "Expected to find lines with metric data")

	if deleteWorkload {
		workloadErr = shared.ManageWorkload("delete", "metrics-server.yaml")
		Expect(workloadErr).To(BeNil())
	}
}
