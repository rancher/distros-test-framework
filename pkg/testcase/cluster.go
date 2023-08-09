package testcase

import (
	"fmt"
	"strings"

	"github.com/rancher/distros-test-framework/factory"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var arch string

func TestBuildCluster(g GinkgoTInterface) {
	var err error

	cluster := factory.GetCluster(g)
	arch = cluster.ArchType
	Expect(cluster.Status).To(Equal("cluster created"))
	Expect(shared.KubeConfigFile).ShouldNot(BeEmpty())
	Expect(cluster.ServerIPs).ShouldNot(BeEmpty())

	if cluster.ProductType == "k3s" {
		if strings.Contains(cluster.ClusterType, "etcd") {
			fmt.Println("Backend:", cluster.ClusterType)
		} else {
			fmt.Println("Backend:", cluster.ExternalDb)
		}
	
		if cluster.ExternalDb != "" && cluster.ClusterType == "" {
			for i := 0; i > len(cluster.ServerIPs); i++ {
				cmd := "grep \"datastore-endpoint\" /etc/systemd/system/k3s.service"
				res, err := shared.RunCommandOnNode(cmd, cluster.ServerIPs[0])
				Expect(err).NotTo(HaveOccurred())
				Expect(res).Should(ContainSubstring(cluster.RenderedTemplate))
			}
		}
	}

	fmt.Println("\nKubeconfig file:\n")
	err = shared.PrintFileContents(shared.KubeConfigFile)
	if err != nil {
		return
	}

	fmt.Println("Base64 Encoded Kubeconfig file:\n")
	err = shared.PrintBase64Encoded(shared.KubeConfigFile)
	if err != nil {
		return
	}

	fmt.Println("Server Node IPS:", cluster.ServerIPs)

	checkAndPrintAgentNodeIPs(cluster.NumAgents, cluster.AgentIPs, false)

	if cluster.ProductType == "rke2" {
		checkAndPrintAgentNodeIPs(cluster.NumWinAgents, cluster.WinAgentIPs, true)
	}	
}

// TestSonobuoyMixedOS runs sonobuoy tests for mixed os cluster (linux + windows) node
func TestSonobuoyMixedOS(version string, delete bool) {
	err := shared.SonobuoyMixedOS("install", version)
	if err != nil {
		fmt.Println(err)
		return
	}

	cmd := "sonobuoy run --kubeconfig=" + shared.KubeConfigFile +
		" --plugin my-sonobuoy-plugins/mixed-workload-e2e/mixed-workload-e2e.yaml" + 
		" --aggregator-node-selector kubernetes.io/os:linux --wait"
	res, err := shared.RunCommandHost(cmd)
	Expect(err).NotTo(HaveOccurred(), "failed output: " + res)

	cmd = fmt.Sprintf("sonobuoy retrieve --kubeconfig=%s",shared.KubeConfigFile)
	testResultTar, err := shared.RunCommandHost(cmd)
	Expect(err).NotTo(HaveOccurred(), "failed cmd: "+ cmd)

	cmd = fmt.Sprintf("sonobuoy results %s",testResultTar)
	res, err = shared.RunCommandHost(cmd)
	Expect(err).NotTo(HaveOccurred(), "failed cmd: "+ cmd)
	Expect(res).Should(ContainSubstring("Plugin: mixed-workload-e2e\nStatus: passed\n"))

	if delete {
		cmd = fmt.Sprintf("sonobuoy delete --all --wait --kubeconfig=%s", shared.KubeConfigFile)
		res, err = shared.RunCommandHost(cmd)
		Expect(err).NotTo(HaveOccurred(), "failed cmd: "+ cmd)
		err := shared.SonobuoyMixedOS("cleanup", version)
		if err != nil {
			fmt.Println(err)
			return
		}
	}
}

// checkAndPrintAgentNodeIPs Prints out the Agent node IPs
// agentNum		int			Number of agent nodes
// agentIPs		[]string	IP list of agent nodes 
// isWindows 	bool 		Check for Windows enablement
func checkAndPrintAgentNodeIPs(agentNum int, agentIPs []string, isWindows bool) {
	info := "Agent Node IPS:"

	if isWindows {
		info = "Windows " + info
	}

	if agentNum > 0 {
		Expect(agentIPs).ShouldNot(BeEmpty())
		fmt.Println(info, agentIPs)
	} else {
		Expect(agentIPs).Should(BeEmpty())
	}
}
