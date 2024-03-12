package testcase

import (
	"fmt"

	"github.com/rancher/distros-test-framework/factory"
	"github.com/rancher/distros-test-framework/pkg/assert"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// TestNodeStatus test the status of the nodes in the cluster using 2 custom assert functions
func TestNodeStatus(
	nodeAssertReadyStatus assert.NodeAssertFunc,
	nodeAssertVersion assert.NodeAssertFunc,
) {
	cluster := factory.ClusterConfig(GinkgoT())
	expectedNodeCount := cluster.NumServers + cluster.NumAgents

	if cluster.Config.Product == "rke2" {
		expectedNodeCount += cluster.NumWinAgents
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
	}, "2500s", "10s").Should(BeTrue(), func() string {
		shared.LogLevel("error", "\ntimeout for nodes to be ready gathering journal logs...\n")
		logs := shared.GetJournalLogs("error", cluster.ServerIPs[0])

		if cluster.NumAgents > 0 {
			logs += shared.GetJournalLogs("error", cluster.AgentIPs[0])
		}

		return logs
	})

	fmt.Println("\n\nCluster nodes:")
	_, err := shared.GetNodes(true)
	Expect(err).NotTo(HaveOccurred())
}

func TestKillall() {
	product, err := shared.GetProduct()
	Expect(err).NotTo(HaveOccurred())

	g := GinkgoT()
	cluster := factory.AddCluster(g)

	for i := len(cluster.ServerIPs) - 1; i > 0; i-- {

		cmd := fmt.Sprintf("sudo %s-killall.sh", product)
		_, err := shared.RunCommandOnNode(cmd, cluster.ServerIPs[i])
		Expect(err).NotTo(HaveOccurred())
	}

	switch {
	case product == "k3s":
		res, _ := shared.RunCommandHost("kubectl get nodes --kubeconfig=" + shared.KubeConfigFile)
		Expect(res).To(ContainSubstring("refused"))
	case product == "rke2":
		res, _ := shared.RunCommandHost("kubectl get nodes --kubeconfig=" + shared.KubeConfigFile)
		Expect(res).To(ContainSubstring("timed out"))
	default:
		shared.ReturnLogError("unsupported product: %s\n", product)
	}
}

func StopServer() {
	product, err := shared.GetProduct()
	Expect(err).NotTo(HaveOccurred())
	g := GinkgoT()
	cluster := factory.AddCluster(g)

	switch {
	case product == "k3s":
		cmd := fmt.Sprintf("sudo systemctl stop %s", product)
		_, err = shared.RunCommandOnNode(cmd, cluster.ServerIPs[0])
		Expect(err).NotTo(HaveOccurred())
	case product == "rke2":
		cmd := fmt.Sprintf("sudo systemctl stop %s-server.service", product)
		_, err = shared.RunCommandOnNode(cmd, cluster.ServerIPs[0])
		Expect(err).NotTo(HaveOccurred())
	default:
		shared.ReturnLogError("unsupported product: %s\n", product)
	}

	switch {
	case product == "k3s":
		cmd := fmt.Sprintf("sudo systemctl status %s", product)
		_, err := shared.RunCommandOnNode(cmd, cluster.ServerIPs[0])
		if err != nil {
			res := err.Error()
			Expect(res).To(SatisfyAny(ContainSubstring("failed"), ContainSubstring("inactive")))
		}
	case product == "rke2":
		cmd := fmt.Sprintf("sudo systemctl status %s-server.service", product)
		_, err := shared.RunCommandOnNode(cmd, cluster.ServerIPs[0])
		if err != nil {
			res := err.Error()
			Expect(res).To(SatisfyAny(ContainSubstring("failed"), ContainSubstring("inactive")))
		}
	default:
		shared.ReturnLogError("unsupported product: %s\n", product)
	}
}

func StartServer() {
	product, err := shared.GetProduct()
	Expect(err).NotTo(HaveOccurred())
	g := GinkgoT()
	cluster := factory.AddCluster(g)
	for _, ip := range cluster.ServerIPs {

		switch {
		case product == "k3s":
			cmd := fmt.Sprintf("sudo systemctl start %s", product)
			_, err = shared.RunCommandOnNode(cmd, ip)
			Expect(err).NotTo(HaveOccurred())
		case product == "rke2":
			cmd := fmt.Sprintf("sudo systemctl start %s-server.service", product)
			_, err = shared.RunCommandOnNode(cmd, ip)
			Expect(err).NotTo(HaveOccurred())
		default:
			shared.ReturnLogError("unsupported product: %s\n", product)
		}
	}
}

func DeleteDatabaseDirectories() {
	product, err := shared.GetProduct()
	Expect(err).NotTo(HaveOccurred())
	g := GinkgoT()
	cluster := factory.AddCluster(g)
	for i := len(cluster.ServerIPs) - 1; i > 0; i-- {

		cmd1 := fmt.Sprintf("sudo rm -rf /var/lib/rancher/%s/server/db", product)
		_, err := shared.RunCommandOnNode(cmd1, cluster.ServerIPs[i])
		Expect(err).NotTo(HaveOccurred())

		cmd2 := fmt.Sprintf("sudo -i ls -l /var/lib/rancher/%s/server/db", product)
		res, err := shared.RunCommandOnNode(cmd2, cluster.ServerIPs[i])

		fmt.Println("Response: ", res)
		fmt.Println("Error: ", err)

		Expect(err.Error()).To(ContainSubstring("No such file or directory"))
	}
}
