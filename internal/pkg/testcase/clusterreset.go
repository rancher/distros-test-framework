package testcase

import (
	"fmt"

	"github.com/rancher/distros-test-framework/internal/provisioning/driver"
	"github.com/rancher/distros-test-framework/internal/resources"

	. "github.com/onsi/gomega"
)

func TestClusterReset(cluster *driver.Cluster) {
	killall(cluster)
	resources.LogLevel("info", "%s-service killed", cluster.Config.Product)

	ms := resources.NewManageService(5, 5)
	stopServer(cluster, ms)
	resources.LogLevel("info", "%s-service stopped", cluster.Config.Product)

	productLocationCmd, findErr := resources.FindPath(cluster.Config.Product, cluster.ServerIPs[0])
	Expect(findErr).NotTo(HaveOccurred())
	resetCmd := fmt.Sprintf("sudo %s server --cluster-reset", productLocationCmd)

	resources.LogLevel("info", "running cluster reset on server %s\n", cluster.ServerIPs[0])

	clusterReset(cluster, resetCmd)
	resources.LogLevel("info", "cluster reset successful. Waiting cluster to sync after reset")

	deleteDataDirectories(cluster)
	resources.LogLevel("info", "data directories deleted")

	restartServer(cluster, ms)
	resources.LogLevel("info", "%s-service restarted", cluster.Config.Product)
}

func clusterReset(cluster *driver.Cluster, resetCmd string) {
	var (
		resetRes    string
		resetCmdErr error
	)

	resetRes, resetCmdErr = resources.RunCommandOnNode(resetCmd, cluster.ServerIPs[0])
	switch cluster.Config.Product {
	case "rke2":
		// rke2 cluster reset output returns stderr channel
		Expect(resetRes).To(BeEmpty())
		Expect(resetCmdErr).To(HaveOccurred())
		Expect(resetCmdErr.Error()).To(ContainSubstring("Managed etcd cluster"))
		Expect(resetCmdErr.Error()).To(ContainSubstring("has been reset"))

	case "k3s":
		// k3s cluster reset output returns stdout channel
		Expect(resetCmdErr).NotTo(HaveOccurred())
		Expect(resetRes).To(ContainSubstring("Managed etcd cluster"))
		Expect(resetRes).To(ContainSubstring("has been reset"))
	}
}

func killall(cluster *driver.Cluster) {
	killallLocationCmd, findErr := resources.FindPath(cluster.Config.Product+"-killall.sh", cluster.ServerIPs[0])
	Expect(findErr).NotTo(HaveOccurred())

	for i := len(cluster.ServerIPs) - 1; i > 0; i-- {
		_, err := resources.RunCommandOnNode("sudo  "+killallLocationCmd, cluster.ServerIPs[i])
		Expect(err).NotTo(HaveOccurred())
	}

	// since we have killed the server, the kubectl command will fail.
	res, _ := resources.RunCommandHost("kubectl get nodes --kubeconfig=" + resources.KubeConfigFile)
	Expect(res).To(SatisfyAny(ContainSubstring("timed out"), ContainSubstring("refused")))
}

func stopServer(cluster *driver.Cluster, ms *resources.ManageService) {
	action := resources.ServiceAction{
		Service:  cluster.Config.Product,
		Action:   "stop",
		NodeType: "server",
	}
	_, stopErr := ms.ManageService(cluster.ServerIPs[0], []resources.ServiceAction{action})
	Expect(stopErr).NotTo(HaveOccurred())

	// due to the stop command, this should fail.
	cmd := fmt.Sprintf("sudo systemctl status %s-server", cluster.Config.Product)
	_, err := resources.RunCommandOnNode(cmd, cluster.ServerIPs[0])
	Expect(err).To(HaveOccurred())
}

func restartServer(cluster *driver.Cluster, ms *resources.ManageService) {
	var startFirst []string
	var startLast []string

	for _, serverIP := range cluster.ServerIPs {
		if serverIP == cluster.ServerIPs[0] {
			startFirst = append(startFirst, serverIP)

			continue
		}
		startLast = append(startLast, serverIP)
	}

	action := resources.ServiceAction{
		Service:  cluster.Config.Product,
		Action:   "restart",
		NodeType: "server",
	}
	for _, ip := range startFirst {
		_, startErr := ms.ManageService(ip, []resources.ServiceAction{action})
		Expect(startErr).NotTo(HaveOccurred())
	}

	for _, ip := range startLast {
		_, startLastErr := ms.ManageService(ip, []resources.ServiceAction{action})
		Expect(startLastErr).NotTo(HaveOccurred())
	}
}

func deleteDataDirectories(cluster *driver.Cluster) {
	for i := len(cluster.ServerIPs) - 1; i > 0; i-- {
		deleteCmd := fmt.Sprintf("sudo rm -rf /var/lib/rancher/%s/server/db", cluster.Config.Product)
		_, deleteErr := resources.RunCommandOnNode(deleteCmd, cluster.ServerIPs[i])
		Expect(deleteErr).NotTo(HaveOccurred())

		checkDirCmd := fmt.Sprintf("sudo -i ls -l /var/lib/rancher/%s/server/db", cluster.Config.Product)
		_, checkDirErr := resources.RunCommandOnNode(checkDirCmd, cluster.ServerIPs[i])

		if checkDirErr != nil {
			Expect(checkDirErr.Error()).To(ContainSubstring("No such file or directory"))
		}
	}
}
