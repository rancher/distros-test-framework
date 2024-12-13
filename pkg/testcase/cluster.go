package testcase

import (
	"fmt"
	"strings"

	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/gomega"
)

func TestBuildCluster(cluster *shared.Cluster) {
	Expect(cluster.Status).To(Equal("cluster created"))
	Expect(shared.KubeConfigFile).ShouldNot(BeEmpty())
	Expect(cluster.ServerIPs).ShouldNot(BeEmpty())

	if strings.Contains(cluster.Config.DataStore, "etcd") {
		shared.LogLevel("info", "Backend: "+cluster.Config.DataStore)
	} else {
		shared.LogLevel("info", "Backend: "+cluster.Config.ExternalDb)
	}

	if cluster.Config.ExternalDb != "" && cluster.Config.DataStore == "external" {
		cmd := fmt.Sprintf("sudo grep \"datastore-endpoint\" /etc/rancher/%s/config.yaml", cluster.Config.Product)
		res, err := shared.RunCommandOnNode(cmd, cluster.ServerIPs[0])
		Expect(err).NotTo(HaveOccurred())
		Expect(res).Should(ContainSubstring(cluster.Config.RenderedTemplate))
	}

	shared.LogLevel("info", "KUBECONFIG: ")
	err := shared.PrintFileContents(shared.KubeConfigFile)
	Expect(err).NotTo(HaveOccurred(), err)

	shared.LogLevel("info", "BASE64 ENCODED KUBECONFIG:")
	err = shared.PrintBase64Encoded(shared.KubeConfigFile)
	Expect(err).NotTo(HaveOccurred(), err)

	if cluster.BastionConfig.PublicIPv4Addr != "" {
		shared.LogLevel("info", "Bastion Node IP: %v", cluster.BastionConfig.PublicIPv4Addr)
	}
	shared.LogLevel("info", "Server Node IPs: %v", cluster.ServerIPs)

	checkAndPrintAgentNodeIPs(cluster.NumAgents, cluster.AgentIPs, false)

	if cluster.Config.Product == "rke2" {
		checkAndPrintAgentNodeIPs(cluster.NumWinAgents, cluster.WinAgentIPs, true)
	}
}

// TestDisplayClusterDetails used to display cluster details.
func TestDisplayClusterDetails() {
	_, err := shared.GetNodes(true)
	Expect(err).NotTo(HaveOccurred())

	_, err = shared.GetPods(true)
	Expect(err).NotTo(HaveOccurred())
}

// checkAndPrintAgentNodeIPs Prints out the Agent node IPs.
func checkAndPrintAgentNodeIPs(agentNum int, agentIPs []string, isWindows bool) {
	info := "Agent Node IPs:"

	if isWindows {
		info = "Windows " + info
	}

	if agentNum > 0 {
		Expect(agentIPs).ShouldNot(BeEmpty())
		shared.LogLevel("info", info+"  %v", agentIPs)
	} else {
		Expect(agentIPs).Should(BeEmpty())
	}
}
