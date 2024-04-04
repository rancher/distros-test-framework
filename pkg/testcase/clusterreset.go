package testcase

import (
	"fmt"

	"github.com/rancher/distros-test-framework/factory"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestClusterReset() {
	killall()
	stopServer()

	product, err := shared.Product()
	Expect(err).NotTo(HaveOccurred())

	g := GinkgoT()
	cluster := factory.ClusterConfig(g)
	ip := cluster.ServerIPs[0]

	var resetCmd string
	var res string
	var resetCmdErr error

	resetCmd = fmt.Sprintf("sudo %s server --cluster-reset", product)

	switch product {
	case "k3s":
		res, resetCmdErr = shared.RunCommandOnNode(resetCmd, ip)
		Expect(resetCmdErr).NotTo(HaveOccurred())
		Expect(res).To(ContainSubstring("Managed etcd cluster"))
		Expect(res).To(ContainSubstring("has been reset"))
	case "rke2":
		_, resetCmdErr = shared.RunCommandOnNode(resetCmd, ip)
		Expect(resetCmdErr).To(HaveOccurred())
		Expect(resetCmdErr.Error()).To(ContainSubstring("Managed etcd cluster"))
		Expect(resetCmdErr.Error()).To(ContainSubstring("has been reset"))
	default:
		shared.LogLevel("error", "unsupported product: %s", product)
		g.Fail()
	}

	deleteDataDirectories()
	startServer()
}

func killall() {
	product, err := shared.Product()
	Expect(err).NotTo(HaveOccurred())

	g := GinkgoT()
	cluster := factory.ClusterConfig(g)

	for i := len(cluster.ServerIPs) - 1; i > 0; i-- {

		killallCmd := fmt.Sprintf("sudo %s-killall.sh", product)
		_, err := shared.RunCommandOnNode(killallCmd, cluster.ServerIPs[i])
		Expect(err).NotTo(HaveOccurred())
	}

	switch product {
	case "k3s":
		res, _ := shared.RunCommandHost("kubectl get nodes --kubeconfig=" + shared.KubeConfigFile)
		Expect(res).To(ContainSubstring("refused"))
	case "rke2":
		res, _ := shared.RunCommandHost("kubectl get nodes --kubeconfig=" + shared.KubeConfigFile)
		Expect(res).To(SatisfyAny(ContainSubstring("timed out"), ContainSubstring("refused")))
	default:
		shared.LogLevel("error", "unsupported product: %s", product)
		g.Fail()
	}
}

func stopServer() {
	product, err := shared.Product()
	Expect(err).NotTo(HaveOccurred())

	g := GinkgoT()
	cluster := factory.ClusterConfig(g)

	_, stopErr := shared.ManageService(product, "stop", "server", []string{cluster.ServerIPs[0]})
	Expect(stopErr).NotTo(HaveOccurred())

	_, statusErr := shared.ManageService(product, "status", "server", []string{cluster.ServerIPs[0]})
	Expect(statusErr).To(HaveOccurred())
	statusRes := statusErr.Error()
	Expect(statusRes).To(SatisfyAny(ContainSubstring("failed"), ContainSubstring("inactive")))
}

func startServer() {
	product, err := shared.Product()
	Expect(err).NotTo(HaveOccurred())
	g := GinkgoT()
	cluster := factory.ClusterConfig(g)

	_, startErr := shared.ManageService(product, "start", "server", cluster.ServerIPs)
	Expect(startErr).NotTo(HaveOccurred())
}

func deleteDataDirectories() {
	product, err := shared.Product()
	Expect(err).NotTo(HaveOccurred())

	g := GinkgoT()
	cluster := factory.ClusterConfig(g)

	for i := len(cluster.ServerIPs) - 1; i > 0; i-- {

		deleteCmd := fmt.Sprintf("sudo rm -rf /var/lib/rancher/%s/server/db", product)
		_, deleteErr := shared.RunCommandOnNode(deleteCmd, cluster.ServerIPs[i])
		Expect(deleteErr).NotTo(HaveOccurred())

		checkDirCmd := fmt.Sprintf("sudo -i ls -l /var/lib/rancher/%s/server/db", product)
		_, checkDirErr := shared.RunCommandOnNode(checkDirCmd, cluster.ServerIPs[i])

		Expect(checkDirErr.Error()).To(ContainSubstring("No such file or directory"))
	}
}
