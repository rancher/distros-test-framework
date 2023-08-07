package testcase

import (
	"fmt"
	"strings"

	"github.com/rancher/distros-test-framework/factory"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestBuildCluster(g GinkgoTInterface) {
	var err error

	cluster := factory.GetCluster(g)
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
	fmt.Println(
		"\nServer Node IPS:", cluster.ServerIPs,
		"\nAgent Node IPS:", cluster.AgentIPs,
		"\nWindows Agent Node IPS:", cluster.WinAgentIPs,
	)

	if cluster.NumAgents > 0 {
		Expect(cluster.AgentIPs).ShouldNot(BeEmpty())
	} else {
		Expect(cluster.AgentIPs).Should(BeEmpty())
	}

	if cluster.NumWinAgents > 0 {
		Expect(cluster.WinAgentIPs).ShouldNot(BeEmpty())
	} else {
		Expect(cluster.WinAgentIPs).Should(BeEmpty())
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
