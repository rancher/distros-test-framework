package testcase

import (
	"github.com/rancher/distros-test-framework/factory"
	"github.com/rancher/distros-test-framework/pkg/customflag"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestDaemonset(deployWorkload bool) {
	var allNodes []shared.Node
	var cpNodes string
	var err error

	if deployWorkload {
		_, err = shared.ManageWorkload(
			"create",
			"daemonset.yaml",
			customflag.ServiceFlag.ClusterConfig.Arch.String(),
		)
		Expect(err).NotTo(HaveOccurred(),
			"Daemonset manifest not deployed")
	}
	nodes, _ := shared.ParseNodes(false)
	pods, _ := shared.ParsePods(false)

	cluster := factory.GetCluster(GinkgoT())
	for _, ip := range cluster.ServerIPs {
		cpNodes, err = shared.RunCommandOnNode("cat /tmp/.control-plane", ip)
		Expect(err).NotTo(HaveOccurred())
	}

	if cpNodes != "true" {
		for _, node := range nodes {
			allNodes = append(allNodes, node)
		}
	}

	Eventually(func(g Gomega) int {
		return shared.CountOfStringInSlice("test-daemonset", pods)
	}, "420s", "5s").Should(Equal(len(allNodes)),
		"Daemonset pod count does not match node count")
}
