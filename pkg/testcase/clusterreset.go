package testcase

import (
	"fmt"

	"github.com/rancher/distros-test-framework/pkg/k8s"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/gomega"
)

func TestClusterReset(cluster *shared.Cluster, k8sClient *k8s.Client) {
	killall(cluster)
	shared.LogLevel("info", "%s-service killed", cluster.Config.Product)

	stopServer(cluster)
	shared.LogLevel("info", "%s-service stopped", cluster.Config.Product)

	productLocationCmd, findErr := shared.FindPath(cluster.Config.Product, cluster.ServerIPs[0])
	Expect(findErr).NotTo(HaveOccurred())
	resetCmd := fmt.Sprintf("sudo %s server --cluster-reset", productLocationCmd)
	shared.LogLevel("info", "running cluster reset on server %s\n", cluster.ServerIPs[0])

	clusterReset(cluster, resetCmd)
	shared.LogLevel("info", "cluster reset successful. Waiting cluster to sync after reset")

	deleteDataDirectories(cluster)
	shared.LogLevel("info", "data directories deleted")

	restartServer(cluster, k8sClient)
	shared.LogLevel("info", "%s-service started. Waiting 60 seconds for nodes "+
		"and pods to sync after reset.", cluster.Config.Product)

	ok, err := k8sClient.CheckClusterHealth(0)
	Expect(err).NotTo(HaveOccurred())
	Expect(ok).To(BeTrue())
}

func clusterReset(cluster *shared.Cluster, resetCmd string) {
	switch cluster.Config.Product {
	case "rke2":
		// rke2 cluster reset output returns stderr channel
		_, resetCmdErr := shared.RunCommandOnNode(resetCmd, cluster.ServerIPs[0])
		Expect(resetCmdErr).To(HaveOccurred())
		Expect(resetCmdErr.Error()).To(ContainSubstring("Managed etcd cluster"))
		Expect(resetCmdErr.Error()).To(ContainSubstring("has been reset"))

	case "k3s":
		// k3s cluster reset output returns stdout channel
		resetRes, resetCmdErr := shared.RunCommandOnNode(resetCmd, cluster.ServerIPs[0])
		shared.LogLevel("info", "cluster reset: %v", resetRes)
		Expect(resetCmdErr).NotTo(HaveOccurred())
		Expect(resetRes).To(ContainSubstring("Managed etcd cluster"))
		Expect(resetRes).To(ContainSubstring("has been reset"))
	}
}

func killall(cluster *shared.Cluster) {
	killallLocationCmd, findErr := shared.FindPath(cluster.Config.Product+"-killall.sh", cluster.ServerIPs[0])
	Expect(findErr).NotTo(HaveOccurred())

	for i := len(cluster.ServerIPs) - 1; i > 0; i-- {
		_, err := shared.RunCommandOnNode("sudo  "+killallLocationCmd, cluster.ServerIPs[i])
		Expect(err).NotTo(HaveOccurred())
	}

	res, _ := shared.RunCommandHost("kubectl get nodes --kubeconfig=" + shared.KubeConfigFile)
	Expect(res).To(SatisfyAny(ContainSubstring("timed out"), ContainSubstring("refused")))
}

func stopServer(cluster *shared.Cluster) {
	_, stopErr := shared.ManageService(cluster.Config.Product, "stop", "server", []string{cluster.ServerIPs[0]})
	Expect(stopErr).NotTo(HaveOccurred())

	_, statusErr := shared.ManageService(cluster.Config.Product, "status", "server", []string{cluster.ServerIPs[0]})
	Expect(statusErr).To(HaveOccurred())
	statusRes := statusErr.Error()
	Expect(statusRes).To(SatisfyAny(ContainSubstring("failed"), ContainSubstring("inactive")))
}

func restartServer(cluster *shared.Cluster, k8sClient *k8s.Client) {
	var startFirst []string
	var startLast []string

	for _, serverIP := range cluster.ServerIPs {
		if serverIP == cluster.ServerIPs[0] {
			startFirst = append(startFirst, serverIP)

			continue
		}
		startLast = append(startLast, serverIP)
	}

	_, startErr := shared.ManageService(cluster.Config.Product, "restart", "server", startFirst)
	Expect(startErr).NotTo(HaveOccurred())

	err := k8sClient.WaitForNodesReady(0)
	Expect(err).NotTo(HaveOccurred())

	_, startLastErr := shared.ManageService(cluster.Config.Product, "restart", "server", startLast)
	Expect(startLastErr).NotTo(HaveOccurred())
}

func deleteDataDirectories(cluster *shared.Cluster) {
	for i := len(cluster.ServerIPs) - 1; i > 0; i-- {
		deleteCmd := fmt.Sprintf("sudo rm -rf /var/lib/rancher/%s/server/db", cluster.Config.Product)
		_, deleteErr := shared.RunCommandOnNode(deleteCmd, cluster.ServerIPs[i])
		Expect(deleteErr).NotTo(HaveOccurred())

		checkDirCmd := fmt.Sprintf("sudo -i ls -l /var/lib/rancher/%s/server/db", cluster.Config.Product)
		_, checkDirErr := shared.RunCommandOnNode(checkDirCmd, cluster.ServerIPs[i])

		Expect(checkDirErr.Error()).To(ContainSubstring("No such file or directory"))
	}
}
