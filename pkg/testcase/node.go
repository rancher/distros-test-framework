package testcase

import (
	"fmt"
	"strconv"
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
	}, timeout, "10s").Should(BeTrue(), func() string {
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
	}, timeout, "10s").Should(BeTrue())
}

func TestNodeMetricsServer(applyWorkload, deleteWorkload bool) {
	var workloadErr error
	if applyWorkload {
		workloadErr = shared.ManageWorkload("apply", "metrics-server.yaml")
		Expect(workloadErr).To(BeNil())
	}
	cmd := "kubectl get pods -n test-metrics-server --kubeconfig=" + shared.KubeConfigFile + " | grep metrics-server"
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

// TestNodeCPUUsageBelowThreshold fails when any node exceeds the provided CPU percentage.
func TestNodeCPUUsageBelowThreshold(maxCPUPercent int, applyWorkload, deleteWorkload bool, timeouts ...string) {
	var workloadErr error
	if applyWorkload {
		shared.LogLevel("info", "Deploying metrics-server...")
		workloadErr = shared.ManageWorkload("apply", "metrics-server.yaml")
		Expect(workloadErr).To(BeNil())
		shared.LogLevel("info", "metrics-server deployed successfully")
	}

	shared.LogLevel("info", "Verifying metrics-server pod is running...")
	cmd := "kubectl get pods -n test-metrics-server --kubeconfig=" + shared.KubeConfigFile + " | grep metrics-server"
	err := assert.ValidateOnHost(cmd, "Running")
	Expect(err).To(BeNil())
	shared.LogLevel("info", "metrics-server pod is running")

	timeout := "120s"
	if len(timeouts) > 0 && timeouts[0] != "" {
		timeout = timeouts[0]
	}

	Eventually(func(g Gomega) bool {
		shared.LogLevel("info", "Querying node CPU usage with 'kubectl top node --no-headers'...")
		topNodeCmd := "kubectl top node --kubeconfig=" + shared.KubeConfigFile + " --no-headers"
		res, err := shared.RunCommandHost(topNodeCmd)
		g.Expect(err).To(BeNil())
		g.Expect(strings.TrimSpace(res)).NotTo(Equal(""), "kubectl top node returned no data")
		shared.LogLevel("info", "kubectl top node query completed:\n%s", res)

		overThreshold, checkErr := checkNodeCPUThreshold(maxCPUPercent, res)
		g.Expect(checkErr).To(BeNil())
		g.Expect(overThreshold).To(BeEmpty(), "Found nodes above %d%% CPU utilization: %v\nFull output:\n%s", maxCPUPercent, overThreshold, res)

		return true
	}, timeout, "10s").Should(BeTrue(), "CPU usage on one or more nodes exceeded %d%% threshold", maxCPUPercent)

	if deleteWorkload {
		shared.LogLevel("info", "Cleaning up metrics-server...")
		workloadErr = shared.ManageWorkload("delete", "metrics-server.yaml")
		Expect(workloadErr).To(BeNil())
		shared.LogLevel("info", "metrics-server cleaned up successfully")
	}
}

func parseNodeCPUPercentages(output string) (map[string]int, error) {
	nodeCPU := make(map[string]int)

	minExpectedFields := 3
	nodeNameFieldIndex := 0
	cpuUsagePercentFieldIndex := 2

	lineCount := 0
	for _, rawLine := range strings.Split(output, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}

		lineCount++
		fields := strings.Fields(line)
		if len(fields) < minExpectedFields {
			shared.LogLevel("info", "  Skipping malformed line (expected %d+ fields): %q", minExpectedFields, line)
			continue
		}

		nodeName := fields[nodeNameFieldIndex]
		cpuPercentRaw := strings.TrimSuffix(fields[cpuUsagePercentFieldIndex], "%")
		cpuPercent, err := strconv.Atoi(cpuPercentRaw)
		if err != nil {
			shared.LogLevel("error", "Failed parsing CPU percent from line %q: %v", line, err)
			return nil, fmt.Errorf("failed parsing CPU percent from line %q: %w", line, err)
		}

		nodeCPU[nodeName] = cpuPercent
		shared.LogLevel("info", "  Parsed node %s with CPU usage %d%%", nodeName, cpuPercent)
	}

	shared.LogLevel("info", "Parsed %d lines total, extracted %d nodes", lineCount, len(nodeCPU))
	return nodeCPU, nil

}

// checkNodeCPUThreshold returns a list of nodes exceeding maxCPUPercent, or nil if all nodes pass.
func checkNodeCPUThreshold(maxCPUPercent int, output string) ([]string, error) {
	shared.LogLevel("info", "Parsing node CPU percentages...")
	nodeCPU, err := parseNodeCPUPercentages(output)
	if err != nil {
		return nil, err
	}
	if len(nodeCPU) == 0 {
		return nil, fmt.Errorf("expected at least one node in kubectl top node output")
	}
	shared.LogLevel("info", "Successfully parsed CPU data for %d nodes", len(nodeCPU))

	shared.LogLevel("info", "Checking if any nodes exceed %d%% CPU threshold...", maxCPUPercent)
	overThreshold := make([]string, 0)
	for nodeName, cpuPercent := range nodeCPU {
		shared.LogLevel("info", "  Node %s: %d%% CPU", nodeName, cpuPercent)
		if cpuPercent > maxCPUPercent {
			overThreshold = append(overThreshold, fmt.Sprintf("%s=%d%%", nodeName, cpuPercent))
		}
	}

	if len(overThreshold) == 0 {
		shared.LogLevel("info", "✓ All nodes are at or below %d%% CPU threshold", maxCPUPercent)
	}

	return overThreshold, nil
}
