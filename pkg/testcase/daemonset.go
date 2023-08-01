package testcase

import (
	"github.com/rancher/distros-test-framework/pkg/customflag"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/gomega"
)

func TestDaemonset(deployWorkload bool) {
	if deployWorkload {
		_, err := shared.ManageWorkload(
			"create",
			"daemonset.yaml",
			customflag.ServiceFlag.ClusterConfig.Arch.String(),
		)
		Expect(err).NotTo(HaveOccurred(),
			"Daemonset manifest not deployed")
	}
	nodes, _ := shared.ParseNodes(false)
	pods, _ := shared.ParsePods(false)

	var allNodes []shared.Node
	for _, node := range nodes {
		if !(node.Roles == "control-plane,master" || node.Roles == "etcd") {
			allNodes = append(allNodes, node)
		}
	}
	Eventually(func(g Gomega) int {
		return shared.CountOfStringInSlice("test-daemonset", pods)
	}, "420s", "5s").Should(Equal(len(allNodes)),
		"Daemonset pod count does not match node count")
}
