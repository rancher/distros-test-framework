package testcase

import (
	"fmt"

	"github.com/rancher/distros-test-framework/factory"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestClusterReset() {
	cluster := factory.ClusterConfig(GinkgoT())

	killall()
	shared.LogLevel("INFO", "%s-service killed", cluster.Config.Product)

	stopServer()
	shared.LogLevel("INFO", "%s-service stopped", cluster.Config.Product)

	var (
		resetRes           string
		resetCmd           string
		resetCmdErr        error
		productLocation    string
		productLocationCmd string
		productLocationErr error
	)

	productLocationCmd = fmt.Sprintf("which %s", cluster.Config.Product)
	productLocation, productLocationErr = shared.RunCommandOnNode(productLocationCmd, cluster.ServerIPs[0])
	Expect(productLocationErr).NotTo(HaveOccurred())
	resetCmd = fmt.Sprintf("sudo %s server --cluster-reset", productLocation)
	shared.LogLevel("INFO", "running cluster reset on server %s\n", cluster.ServerIPs[0])

	if cluster.Config.Product == "k3s" {
		// k3s cluster reset output returns stdout channel
		resetRes, resetCmdErr = shared.RunCommandOnNode(resetCmd, cluster.ServerIPs[0])
		Expect(resetCmdErr).NotTo(HaveOccurred())
		Expect(resetRes).To(ContainSubstring("Managed etcd cluster"))
		Expect(resetRes).To(ContainSubstring("has been reset"))
	} else if cluster.Config.Product == "rke2" {
		// rke2 cluster reset output returns stderr channel
		_, resetCmdErr = shared.RunCommandOnNode(resetCmd, cluster.ServerIPs[0])
		Expect(resetCmdErr).To(HaveOccurred())
		Expect(resetCmdErr.Error()).To(ContainSubstring("Managed etcd cluster"))
		Expect(resetCmdErr.Error()).To(ContainSubstring("has been reset"))
	}
	shared.LogLevel("INFO", "cluster reset successful")

	deleteDataDirectories()
	shared.LogLevel("INFO", "data directories deleted")
	startServer()
	shared.LogLevel("INFO", "%s-service started", cluster.Config.Product)
}

func killall() {
	var (
		productLocation    string
		productLocationCmd string
		productLocationErr error
	)

	cluster := factory.ClusterConfig(GinkgoT())
	for i := len(cluster.ServerIPs) - 1; i > 0; i-- {
		productLocationCmd = fmt.Sprintf("which %s", cluster.Config.Product)
		productLocation, productLocationErr = shared.RunCommandOnNode(productLocationCmd, cluster.ServerIPs[i])
		Expect(productLocationErr).NotTo(HaveOccurred())
		_, err := shared.RunCommandOnNode(fmt.Sprintf("sudo %s-killall.sh", productLocation), cluster.ServerIPs[i])
		Expect(err).NotTo(HaveOccurred())
	}

	switch cluster.Config.Product {
	case "k3s":
		res, _ := shared.RunCommandHost("kubectl get nodes --kubeconfig=" + shared.KubeConfigFile)
		Expect(res).To(ContainSubstring("refused"))
	case "rke2":
		res, _ := shared.RunCommandHost("kubectl get nodes --kubeconfig=" + shared.KubeConfigFile)
		Expect(res).To(SatisfyAny(ContainSubstring("timed out"), ContainSubstring("refused")))
	default:
		shared.LogLevel("error", "unsupported product: %s", cluster.Config.Product)
		GinkgoT().Fail()
	}
}

func stopServer() {
	cluster := factory.ClusterConfig(GinkgoT())

	_, stopErr := shared.ManageService(cluster.Config.Product, "stop", "server", []string{cluster.ServerIPs[0]})
	Expect(stopErr).NotTo(HaveOccurred())

	_, statusErr := shared.ManageService(cluster.Config.Product, "status", "server", []string{cluster.ServerIPs[0]})
	Expect(statusErr).To(HaveOccurred())
	statusRes := statusErr.Error()
	Expect(statusRes).To(SatisfyAny(ContainSubstring("failed"), ContainSubstring("inactive")))
}

func startServer() {
	cluster := factory.ClusterConfig(GinkgoT())

	_, startErr := shared.ManageService(cluster.Config.Product, "start", "server", cluster.ServerIPs)
	Expect(startErr).NotTo(HaveOccurred())
}

func deleteDataDirectories() {
	cluster := factory.ClusterConfig(GinkgoT())
	for i := len(cluster.ServerIPs) - 1; i > 0; i-- {

		deleteCmd := fmt.Sprintf("sudo rm -rf /var/lib/rancher/%s/server/db", cluster.Config.Product)
		_, deleteErr := shared.RunCommandOnNode(deleteCmd, cluster.ServerIPs[i])
		Expect(deleteErr).NotTo(HaveOccurred())

		checkDirCmd := fmt.Sprintf("sudo -i ls -l /var/lib/rancher/%s/server/db", cluster.Config.Product)
		_, checkDirErr := shared.RunCommandOnNode(checkDirCmd, cluster.ServerIPs[i])

		Expect(checkDirErr.Error()).To(ContainSubstring("No such file or directory"))
	}
}
