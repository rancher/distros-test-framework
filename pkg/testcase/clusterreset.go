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

	killall(cluster)
	shared.LogLevel("INFO", "%s-service killed", cluster.Config.Product)

	stopServer(cluster)
	shared.LogLevel("INFO", "%s-service stopped", cluster.Config.Product)

	productLocationCmd := fmt.Sprintf("sudo find / -type f -executable -name %s 2> /dev/null | sed 1q", cluster.Config.Product)
	productLocation, _ := shared.RunCommandOnNode(productLocationCmd, cluster.ServerIPs[0])
	Expect(productLocation).To(ContainSubstring(cluster.Config.Product))
	resetCmd := fmt.Sprintf("sudo %s server --cluster-reset", productLocation)
	shared.LogLevel("INFO", "running cluster reset on server %s\n", cluster.ServerIPs[0])

	if cluster.Config.Product == "k3s" {
		// k3s cluster reset output returns stdout channel
		resetRes, resetCmdErr := shared.RunCommandOnNode(resetCmd, cluster.ServerIPs[0])
		Expect(resetCmdErr).NotTo(HaveOccurred())
		Expect(resetRes).To(ContainSubstring("Managed etcd cluster"))
		Expect(resetRes).To(ContainSubstring("has been reset"))
	} else if cluster.Config.Product == "rke2" {
		// rke2 cluster reset output returns stderr channel
		_, resetCmdErr := shared.RunCommandOnNode(resetCmd, cluster.ServerIPs[0])
		Expect(resetCmdErr).To(HaveOccurred())
		Expect(resetCmdErr.Error()).To(ContainSubstring("Managed etcd cluster"))
		Expect(resetCmdErr.Error()).To(ContainSubstring("has been reset"))
	}
	shared.LogLevel("INFO", "cluster reset successful")

	deleteDataDirectories(cluster)
	shared.LogLevel("INFO", "data directories deleted")
	startServer(cluster)
	shared.LogLevel("INFO", "%s-service started", cluster.Config.Product)
}

func killall(cluster *factory.Cluster) {
	for i := len(cluster.ServerIPs) - 1; i > 0; i-- {
		killallLocationCmd := fmt.Sprintf("sudo find / -type f -executable -name %s-killall.sh 2> /dev/null | sed 1q", cluster.Config.Product)
		killallLocation, _ := shared.RunCommandOnNode(killallLocationCmd, cluster.ServerIPs[i])
		Expect(killallLocation).To(ContainSubstring(cluster.Config.Product))
		_, err := shared.RunCommandOnNode(fmt.Sprintf("sudo %s", killallLocation), cluster.ServerIPs[i])
		Expect(err).NotTo(HaveOccurred())
	}
	res, _ := shared.RunCommandHost("kubectl get nodes --kubeconfig=" + shared.KubeConfigFile)
	Expect(res).To(SatisfyAny(ContainSubstring("timed out"), ContainSubstring("refused")))
}

func stopServer(cluster *factory.Cluster) {
	_, stopErr := shared.ManageService(cluster.Config.Product, "stop", "server", []string{cluster.ServerIPs[0]})
	Expect(stopErr).NotTo(HaveOccurred())

	_, statusErr := shared.ManageService(cluster.Config.Product, "status", "server", []string{cluster.ServerIPs[0]})
	Expect(statusErr).To(HaveOccurred())
	statusRes := statusErr.Error()
	Expect(statusRes).To(SatisfyAny(ContainSubstring("failed"), ContainSubstring("inactive")))
}

func startServer(cluster *factory.Cluster) {
	_, startErr := shared.ManageService(cluster.Config.Product, "start", "server", cluster.ServerIPs)
	Expect(startErr).NotTo(HaveOccurred())
}

func deleteDataDirectories(cluster *factory.Cluster) {
	for i := len(cluster.ServerIPs) - 1; i > 0; i-- {

		deleteCmd := fmt.Sprintf("sudo rm -rf /var/lib/rancher/%s/server/db", cluster.Config.Product)
		_, deleteErr := shared.RunCommandOnNode(deleteCmd, cluster.ServerIPs[i])
		Expect(deleteErr).NotTo(HaveOccurred())

		checkDirCmd := fmt.Sprintf("sudo -i ls -l /var/lib/rancher/%s/server/db", cluster.Config.Product)
		_, checkDirErr := shared.RunCommandOnNode(checkDirCmd, cluster.ServerIPs[i])

		Expect(checkDirErr.Error()).To(ContainSubstring("No such file or directory"))
	}
}
