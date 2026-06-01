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
	timeouts ...string,
) {
	expectedNodeCount := cluster.NumServers + cluster.NumAgents
	if cluster.Config.Product == "rke2" {
		expectedNodeCount += cluster.NumWinAgents
	}

	timeout := "600s"
	if len(timeouts) > 0 && timeouts[0] != "" {
		timeout = timeouts[0]
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
	}, timeout, "10s").Should(BeTrue(), func() string {
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
	timeouts ...string,
) {
	expectedNodeCount := cluster.NumServers + cluster.NumAgents

	if cluster.Config.Product == "rke2" {
		expectedNodeCount += cluster.NumWinAgents
	}

	timeout := "600s"
	if len(timeouts) > 0 && timeouts[0] != "" {
		timeout = timeouts[0]
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
	}, timeout, "10s").Should(BeTrue())
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

// TestNodeCPUThreshold fails when any node exceeds the provided CPU percentage.
func TestNodeCPUThreshold(maxCPUPercent int, applyWorkload, deleteWorkload bool, timeouts ...string) {
	var workloadErr error
	if applyWorkload {
		resources.LogLevel("info", "Deploying test metrics-server workload...")
		workloadErr = resources.ManageWorkload("apply", "metrics-server.yaml")
		Expect(workloadErr).To(BeNil())
		resources.LogLevel("info", "Test metrics-server workload deployed successfully")
	}

	resources.LogLevel("info", "Verifying test metrics-server workload pod is running...")
	cmd := "kubectl get pods -n test-metrics-server --kubeconfig=" + resources.KubeConfigFile + " | grep metrics-server"
	err := assert.ValidateOnHost(cmd, "Running")
	Expect(err).To(BeNil())
	resources.LogLevel("info", "Test metrics-server workload pod is running")

	timeout := "120s"
	if len(timeouts) > 0 && timeouts[0] != "" {
		timeout = timeouts[0]
	}
	resources.LogLevel("info", "Querying node CPU usage with 'kubectl top node --no-headers'...")

	Eventually(func(g Gomega) bool {
		topNodeCmd := "kubectl top node --kubeconfig=" + resources.KubeConfigFile + " --no-headers"
		res, err := resources.RunCommandHost(topNodeCmd)
		g.Expect(err).To(BeNil())
		g.Expect(strings.TrimSpace(res)).NotTo(Equal(""), "kubectl top node returned no data")
		overThreshold, checkErr := resources.CheckNodeCPUThreshold(maxCPUPercent, res)
		g.Expect(checkErr).To(BeNil())
		g.Expect(overThreshold).To(BeEmpty(), "Found nodes above %d%% CPU utilization: %v\nFull output:\n%s",
			maxCPUPercent, overThreshold, res)

		return true
	}, timeout, "10s").Should(BeTrue(), "CPU usage on one or more nodes exceeded %d%% threshold", maxCPUPercent)

	if deleteWorkload {
		resources.LogLevel("info", "Cleaning up test metrics-server workload...")
		workloadErr = resources.ManageWorkload("delete", "metrics-server.yaml")
		Expect(workloadErr).To(BeNil())
		resources.LogLevel("info", "Test workload cleaned up successfully")
	}
}
