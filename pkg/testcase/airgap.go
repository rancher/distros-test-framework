package testcase

import (
	"fmt"

	"github.com/rancher/distros-test-framework/factory"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestBuildPrivateCluster(g GinkgoTInterface) {
	cluster := factory.ClusterConfig(g)
	Expect(cluster.Status).To(Equal("cluster created"))
	//Expect(shared.KubeConfigFile).ShouldNot(BeEmpty())
	Expect(cluster.ServerIPs).ShouldNot(BeEmpty())

	// fmt.Println("\nKUBECONFIG:")
	// err := shared.PrintFileContents(shared.KubeConfigFile)
	// Expect(err).NotTo(HaveOccurred(), err)

	// fmt.Println("BASE64 ENCODED KUBECONFIG:")
	// err = shared.PrintBase64Encoded(shared.KubeConfigFile)
	// Expect(err).NotTo(HaveOccurred(), err)

	if cluster.GeneralConfig.BastionIP != "" {
		fmt.Println("\nBastion Node IP:", cluster.GeneralConfig.BastionIP)
	}
	fmt.Println("\nServer Node IPs:", cluster.ServerIPs)

	checkAndPrintAgentNodeIPs(cluster.NumAgents, cluster.AgentIPs, false)

	if cluster.Config.Product == "rke2" {
		checkAndPrintAgentNodeIPs(cluster.NumWinAgents, cluster.WinAgentIPs, true)
	}
}

func TestAirgapPrivateRegistry() {
	
}