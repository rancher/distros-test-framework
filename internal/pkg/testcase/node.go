package testcase

import (
	"strings"

	"github.com/rancher/distros-test-framework/internal/pkg/assert"
	"github.com/rancher/distros-test-framework/internal/pkg/testcase/support"
	"github.com/rancher/distros-test-framework/internal/provisioning/driver"
	"github.com/rancher/distros-test-framework/internal/resources"

	. "github.com/onsi/gomega"
)

// TestNodeStatus test the status of the nodes in the cluster using 2 custom assert functions.
func TestNodeStatus(
	cluster *driver.Cluster,
	nodeAssertReadyStatus assert.NodeAssertFunc,
	nodeAssertVersion assert.NodeAssertFunc,
) {
	expectedNodeCount := cluster.NumServers + cluster.NumAgents
	if cluster.Config.Product == "rke2" {
		expectedNodeCount += cluster.NumWinAgents
	}

	Eventually(func(g Gomega) bool {
		nodes, err := resources.GetNodes(false)
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
		resources.LogLevel("error", "\nNodes are not in desired state")
		_, err := resources.GetNodes(true)
		Expect(err).NotTo(HaveOccurred())
		resources.LogLevel("info", "Journal logs from server node-1: %v\n", cluster.ServerIPs[0])
		logs := resources.GetJournalLogs("error", cluster.ServerIPs[0], cluster.Config.Product)
		resources.LogLevel("info", "Journal logs from agent node-1: %v\n", cluster.AgentIPs[0])
		if cluster.NumAgents > 0 {
			logs += resources.GetJournalLogs("error", cluster.AgentIPs[0], cluster.Config.Product)
		}

		return logs
	})

	_, err := resources.GetNodes(true)
	Expect(err).NotTo(HaveOccurred())
}

// TestNodeStatusUsingBastion test the status of the nodes in the private cluster using 2 custom assert functions.
func TestNodeStatusUsingBastion(
	cluster *driver.Cluster,
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
		nodes := resources.ParseNodes(nodeDetails)
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
		workloadErr = resources.ManageWorkload("apply", "metrics-server.yaml")
		Expect(workloadErr).To(BeNil())
	}
	cmd := "kubectl get pods -n test-metrics-server --kubeconfig=" + resources.KubeConfigFile + " | grep metrics-server"
	err := assert.ValidateOnHost(cmd, "Running")
	Expect(err).To(BeNil())

	topCmd := "kubectl top node --kubeconfig=" + resources.KubeConfigFile + " | grep CPU -A1 " +
		" && kubectl top pods -A --kubeconfig=" + resources.KubeConfigFile + " | grep CPU -A5 "
	res, err := resources.RunCommandHost(topCmd)
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
		workloadErr = resources.ManageWorkload("delete", "metrics-server.yaml")
		Expect(workloadErr).To(BeNil())
	}
}

func TestNodeStatusForK3k(
	host *driver.HostCluster,
	nodeAssertReadyStatus assert.NodeAssertFunc,
	nodeAssertVersion assert.NodeAssertFunc,
	kubeconfigFile string,
) {
	Eventually(func(g Gomega) bool {
		// TODO GetNodes will use generic kubectl that may not work for rke2. May need fix.
		resources.KubeConfigFile = kubeconfigFile
		nodes, err := resources.GetNodesForK3k(true, host.ServerIP, kubeconfigFile)
		g.Expect(err).NotTo(HaveOccurred())
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
		resources.LogLevel("error", "\nNodes are not in desired state")
		_, err := resources.GetNodesForK3k(true, host.ServerIP, kubeconfigFile)
		Expect(err).NotTo(HaveOccurred())
		resources.LogLevel("info", "Journal logs from server node-1: %v\n", host.ServerIP)
		logs := resources.GetJournalLogs("error", host.ServerIP, host.HostClusterType)

		return logs
	})
	_, err := resources.GetNodesForK3k(true, host.ServerIP, kubeconfigFile)
	Expect(err).NotTo(HaveOccurred())
}
