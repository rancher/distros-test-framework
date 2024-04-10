package testcase

import (
	"fmt"

	"github.com/rancher/distros-test-framework/factory"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestClusterReset() {
	product, err := shared.Product()
	Expect(err).NotTo(HaveOccurred())

	killall()
	shared.LogLevel("INFO", "%s-service killed", product)

	stopServer()
	shared.LogLevel("INFO", "%s-service stopped", product)

	g := GinkgoT()
	cluster := factory.ClusterConfig(g)

	var (
		resetRes           string
		resetCmd           string
		resetCmdErr        error
		productLocation    string
		productLocationCmd string
		productLocationErr error
	)

	productLocationCmd = fmt.Sprintf("which %s", product)
	productLocation, productLocationErr = shared.RunCommandOnNode(productLocationCmd, cluster.ServerIPs[0])
	Expect(productLocationErr).NotTo(HaveOccurred())
	resetCmd = fmt.Sprintf("sudo %s server --cluster-reset", productLocation)
	shared.LogLevel("INFO", "running cluster reset on server %s\n", cluster.ServerIPs[0])

	if product == "k3s" {
		// k3s cluster reset output returns stdout channel
		resetRes, resetCmdErr = shared.RunCommandOnNode(resetCmd, cluster.ServerIPs[0])
		Expect(resetCmdErr).NotTo(HaveOccurred())
		Expect(resetRes).To(ContainSubstring("Managed etcd cluster"))
		Expect(resetRes).To(ContainSubstring("has been reset"))
	} else if product == "rke2" {
		// rke2 cluster reset output returns stderr channel
		_, resetCmdErr = shared.RunCommandOnNode(resetCmd, cluster.ServerIPs[0])
		Expect(resetCmdErr).To(HaveOccurred())
		Expect(resetCmdErr.Error()).To(ContainSubstring("Managed etcd cluster"))
		Expect(resetCmdErr.Error()).To(ContainSubstring("has been reset"))
	} else {
		shared.LogLevel("error", "unsupported product: %s", product)
		g.Fail()
	}
	shared.LogLevel("INFO", "cluster reset successful")

	deleteDataDirectories()
	shared.LogLevel("INFO", "data directories deleted")
	startServer()
	shared.LogLevel("INFO", "%s-service started", product)
}

func killall() {
	product, err := shared.Product()
	Expect(err).NotTo(HaveOccurred())

	var (
		productLocation    string
		productLocationCmd string
		productLocationErr error
	)

	g := GinkgoT()
	cluster := factory.ClusterConfig(g)
	for i := len(cluster.ServerIPs) - 1; i > 0; i-- {
		productLocationCmd = fmt.Sprintf("which %s", product)
		productLocation, productLocationErr = shared.RunCommandOnNode(productLocationCmd, cluster.ServerIPs[i])
		Expect(productLocationErr).NotTo(HaveOccurred())
		_, err := shared.RunCommandOnNode(fmt.Sprintf("sudo %s-killall.sh", productLocation), cluster.ServerIPs[i])
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
