package testcase

import (
	"fmt"
	"strings"

	. "github.com/onsi/gomega"
	"github.com/rancher/distros-test-framework/internal/pkg/testcase/support"
	"github.com/rancher/distros-test-framework/internal/provisioning/driver"
	"github.com/rancher/distros-test-framework/internal/resources"
)

func TestBuildCluster(cluster *driver.Cluster) {
	Expect(cluster.Status).To(Equal("cluster created"))
	Expect(resources.KubeConfigFile).ShouldNot(BeEmpty())
	Expect(cluster.ServerIPs).ShouldNot(BeEmpty())

	if strings.Contains(cluster.Config.DataStore, "etcd") {
		resources.LogLevel("info", "\nBackend: %v\n", cluster.Config.DataStore)
	} else {
		resources.LogLevel("info", "\nBackend: %v\n", cluster.Config.ExternalDb)
	}

	if cluster.Config.ExternalDb != "" && cluster.Config.DataStore == "external" {
		cmd := fmt.Sprintf("sudo grep \"datastore-endpoint\" /etc/rancher/%s/config.yaml", cluster.Config.Product)
		res, err := resources.RunCommandOnNode(cmd, cluster.ServerIPs[0])
		Expect(err).NotTo(HaveOccurred())
		Expect(res).Should(ContainSubstring(cluster.Config.ExternalDbEndpoint))
	}

	resources.LogLevel("info", "KUBECONFIG: ")
	err := resources.PrintFileContents(resources.KubeConfigFile)
	Expect(err).NotTo(HaveOccurred(), err)

	resources.LogLevel("info", "BASE64 ENCODED KUBECONFIG:")
	err = resources.PrintBase64Encoded(resources.KubeConfigFile)
	Expect(err).NotTo(HaveOccurred(), err)

	if cluster.Bastion.PublicIPv4Addr != "" {
		resources.LogLevel("info", "Bastion Node IP: %v", cluster.Bastion.PublicIPv4Addr)
	}
	resources.LogLevel("info", "Server Node IPs: %v", cluster.ServerIPs)

	support.LogAgentNodeIPs(cluster.NumAgents, cluster.AgentIPs, false)

	if cluster.Config.Product == "rke2" && cluster.NumWinAgents > 0 {
		support.LogAgentNodeIPs(cluster.NumWinAgents, cluster.WinAgentIPs, true)
	}
}
