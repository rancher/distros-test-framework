package testcase

import (
	"fmt"

	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/gomega"
)

func TestClusterReset(cluster *shared.Cluster) {
	killall(cluster)
	shared.LogLevel("info", "%s-service killed", cluster.Config.Product)

	ms := shared.NewManageService(5, 5)
	stopServer(cluster, ms)
	shared.LogLevel("info", "%s-service stopped", cluster.Config.Product)

	productLocationCmd, findErr := shared.FindPath(cluster.Config.Product, cluster.ServerIPs[0])
	Expect(findErr).NotTo(HaveOccurred())
	resetCmd := fmt.Sprintf("sudo %s server --cluster-reset", productLocationCmd)

	shared.LogLevel("info", "running cluster reset on server %s\n", cluster.ServerIPs[0])

	clusterReset(cluster, resetCmd)
	shared.LogLevel("info", "cluster reset successful. Waiting cluster to sync after reset")

	deleteDataDirectories(cluster)
	shared.LogLevel("info", "data directories deleted")

	restartServer(cluster, ms)
	shared.LogLevel("info", "%s-service restarted", cluster.Config.Product)
}

func clusterReset(cluster *shared.Cluster, resetCmd string) {
	var (
		resetRes    string
		resetCmdErr error
	)

	resetRes, resetCmdErr = shared.RunCommandOnNode(resetCmd, cluster.ServerIPs[0])
	shared.LogLevel("debug", "Cluster reset command output: %s", resetRes)
	shared.LogLevel("debug", "Cluster reset command error: %v", resetCmdErr)
	Expect(resetCmdErr).NotTo(HaveOccurred())
	Expect(resetRes).To(ContainSubstring("Managed etcd cluster"))
	Expect(resetRes).To(ContainSubstring("has been reset"))
}

func killall(cluster *shared.Cluster) {
	killallLocationCmd, findErr := shared.FindPath(cluster.Config.Product+"-killall.sh", cluster.ServerIPs[0])
	shared.LogLevel("debug", "Found killall script at: %s", killallLocationCmd)
	Expect(findErr).NotTo(HaveOccurred())

	for i := len(cluster.ServerIPs) - 1; i >= 0; i-- {
		_, err := shared.RunCommandOnNode("sudo  "+killallLocationCmd, cluster.ServerIPs[i])
		Expect(err).NotTo(HaveOccurred())
	}

	// since we have killed the server, the kubectl command will fail.
	res, _ := shared.RunCommandHost("kubectl get nodes --kubeconfig=" + shared.KubeConfigFile)
	Expect(res).To(SatisfyAny(ContainSubstring("timed out"), ContainSubstring("refused")))
}

func stopServer(cluster *shared.Cluster, ms *shared.ManageService) {
	action := shared.ServiceAction{
		Service:  cluster.Config.Product,
		Action:   "stop",
		NodeType: "server",
	}
	_, stopErr := ms.ManageService(cluster.ServerIPs[0], []shared.ServiceAction{action})
	Expect(stopErr).NotTo(HaveOccurred())

	// due to the stop command, this should fail.
	cmd := fmt.Sprintf("sudo systemctl status %s-server", cluster.Config.Product)
	_, err := shared.RunCommandOnNode(cmd, cluster.ServerIPs[0])
	Expect(err).To(HaveOccurred())
}

func restartServer(cluster *shared.Cluster, ms *shared.ManageService) {
	var startFirst []string
	var startLast []string

	for _, serverIP := range cluster.ServerIPs {
		if serverIP == cluster.ServerIPs[0] {
			startFirst = append(startFirst, serverIP)

			continue
		}
		startLast = append(startLast, serverIP)
	}

	action := shared.ServiceAction{
		Service:  cluster.Config.Product,
		Action:   "restart",
		NodeType: "server",
	}
	for _, ip := range startFirst {
		_, startErr := ms.ManageService(ip, []shared.ServiceAction{action})
		Expect(startErr).NotTo(HaveOccurred())
	}

	for _, ip := range startLast {
		_, startLastErr := ms.ManageService(ip, []shared.ServiceAction{action})
		Expect(startLastErr).NotTo(HaveOccurred())
	}
}

func deleteDataDirectories(cluster *shared.Cluster) {
	for i := len(cluster.ServerIPs) - 1; i > 0; i-- {
		deleteCmd := fmt.Sprintf("sudo rm -rf /var/lib/rancher/%s/server/db", cluster.Config.Product)
		_, deleteErr := shared.RunCommandOnNode(deleteCmd, cluster.ServerIPs[i])
		Expect(deleteErr).NotTo(HaveOccurred())

		checkDirCmd := fmt.Sprintf("sudo -i ls -l /var/lib/rancher/%s/server/db", cluster.Config.Product)
		_, checkDirErr := shared.RunCommandOnNode(checkDirCmd, cluster.ServerIPs[i])

		if checkDirErr != nil {
			Expect(checkDirErr.Error()).To(ContainSubstring("No such file or directory"))
		}
	}
}
