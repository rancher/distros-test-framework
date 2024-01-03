package testcase

import (
	"fmt"
	"strings"

	"github.com/rancher/distros-test-framework/factory"
	"github.com/rancher/distros-test-framework/pkg/customflag"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestBuildCluster(g GinkgoTInterface) {
	cluster := factory.ClusterConfig(g)
	Expect(cluster.Status).To(Equal("cluster created"))
	Expect(shared.KubeConfigFile).ShouldNot(BeEmpty())
	Expect(cluster.ServerIPs).ShouldNot(BeEmpty())

	if strings.Contains(cluster.Config.DataStore, "etcd") {
		fmt.Println("Backend:", cluster.Config.DataStore)
	} else {
		fmt.Println("Backend:", cluster.Config.ExternalDb)
	}

	if cluster.Config.ExternalDb != "" && cluster.Config.DataStore == "external" {
		cmd := "grep \"datastore-endpoint\" /etc/systemd/system/k3s.service"
		res, err := shared.RunCommandOnNode(cmd, cluster.ServerIPs[0])
		Expect(err).NotTo(HaveOccurred())
		Expect(res).Should(ContainSubstring(cluster.Config.RenderedTemplate))

		etcd, err := shared.RunCommandHost("cat /var/lib/rancher/k3s/server/db/etcd/config",
			cluster.ServerIPs[0])
		// TODO: validate also after fix https://github.com/k3s-io/k3s/issues/8744
		Expect(etcd).Should(ContainSubstring(" No such file or directory"))
		Expect(err).To(HaveOccurred())
	}

	fmt.Println("\nKUBECONFIG:")
	err := shared.PrintFileContents(shared.KubeConfigFile)
	Expect(err).NotTo(HaveOccurred(), err)

	fmt.Println("BASE64 ENCODED KUBECONFIG:")
	err = shared.PrintBase64Encoded(shared.KubeConfigFile)
	Expect(err).NotTo(HaveOccurred(), err)

	if cluster.ExtraConfig.BastionIP != "" {
		fmt.Println("\nBastion Node IP:", cluster.ExtraConfig.BastionIP)
	}
	fmt.Println("\nServer Node IPs:", cluster.ServerIPs)

	checkAndPrintAgentNodeIPs(cluster.NumAgents, cluster.AgentIPs, false)

	if cluster.Config.Product == "rke2" {
		checkAndPrintAgentNodeIPs(cluster.NumWinAgents, cluster.WinAgentIPs, true)
	}
}

// TestSonobuoyMixedOS runs sonobuoy tests for mixed os cluster (linux + windows) node
func TestSonobuoyMixedOS(deleteWorkload bool) {
	sonobuoyVersion := customflag.ServiceFlag.ExternalFlag.SonobuoyVersion
	err := shared.SonobuoyMixedOS("install", sonobuoyVersion)
	Expect(err).NotTo(HaveOccurred())

	cmd := "sonobuoy run --kubeconfig=" + shared.KubeConfigFile +
		" --plugin my-sonobuoy-plugins/mixed-workload-e2e/mixed-workload-e2e.yaml" +
		" --aggregator-node-selector kubernetes.io/os:linux --wait"
	res, err := shared.RunCommandHost(cmd)
	Expect(err).NotTo(HaveOccurred(), "failed output: "+res)

	cmd = fmt.Sprintf("sonobuoy retrieve --kubeconfig=%s", shared.KubeConfigFile)
	testResultTar, err := shared.RunCommandHost(cmd)
	Expect(err).NotTo(HaveOccurred(), "failed cmd: "+cmd)

	cmd = fmt.Sprintf("sonobuoy results %s", testResultTar)
	res, err = shared.RunCommandHost(cmd)
	Expect(err).NotTo(HaveOccurred(), "failed cmd: "+cmd)
	Expect(res).Should(ContainSubstring("Plugin: mixed-workload-e2e\nStatus: passed\n"))

	if deleteWorkload {
		cmd = fmt.Sprintf("sonobuoy delete --all --wait --kubeconfig=%s", shared.KubeConfigFile)
		_, err = shared.RunCommandHost(cmd)
		Expect(err).NotTo(HaveOccurred(), "failed cmd: "+cmd)
		err = shared.SonobuoyMixedOS("delete", sonobuoyVersion)
		if err != nil {
			GinkgoT().Errorf("error: %v", err)
			return
		}
	}
}

// setHelper returns the cluster and product
func FetchCluster() (*factory.Cluster, error) {
	cluster := factory.ClusterConfig(GinkgoT())
	return cluster, nil
}

// checkAndPrintAgentNodeIPs Prints out the Agent node IPs
// agentNum		int			Number of agent nodes
// agentIPs		[]string	IP list of agent nodes
// isWindows 	bool 		Check for Windows enablement
func checkAndPrintAgentNodeIPs(agentNum int, agentIPs []string, isWindows bool) {
	info := "Agent Node IPs:"

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
