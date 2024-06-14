package testcase

import (
	"fmt"
	"time"

	"github.com/rancher/distros-test-framework/factory"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/gomega"
)

func TestClusterReset(cluster *factory.Cluster) {
	killall(cluster)
	shared.LogLevel("info", "%s-service killed", cluster.Config.Product)

	stopServer(cluster)
	shared.LogLevel("info", "%s-service stopped", cluster.Config.Product)

	productLocationCmd := fmt.Sprintf("sudo find / -type f -executable -name %s "+
		"2> /dev/null | grep -v data | sed 1q", cluster.Config.Product)
	productLocation, _ := shared.RunCommandOnNode(productLocationCmd, cluster.ServerIPs[0])
	Expect(productLocation).To(ContainSubstring(cluster.Config.Product))
	resetCmd := fmt.Sprintf("sudo %s server --cluster-reset", productLocation)
	shared.LogLevel("info", "running cluster reset on server %s\n", cluster.ServerIPs[0])

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
	shared.LogLevel("info", "cluster reset successful. Waiting 60 seconds for cluster "+
		"to complete background processes after reset.")
	time.Sleep(60 * time.Second)

	deleteDataDirectories(cluster)
	shared.LogLevel("info", "data directories deleted")

	startServer(cluster)
	shared.LogLevel("info", "%s-service started. Waiting 60 seconds for nodes "+
		"and pods to sync after reset.", cluster.Config.Product)

	time.Sleep(60 * time.Second)
}

func killall(cluster *factory.Cluster) {
	for i := len(cluster.ServerIPs) - 1; i > 0; i-- {
		killallLocationCmd := fmt.Sprintf("sudo find / -type f -executable -name %s-killall.sh "+
			"2> /dev/null | grep -v data  | sed 1q", cluster.Config.Product)
		killallLocation, _ := shared.RunCommandOnNode(killallLocationCmd, cluster.ServerIPs[i])
		Expect(killallLocation).To(ContainSubstring(cluster.Config.Product))
		_, err := shared.RunCommandOnNode(fmt.Sprintf("sudo %s", killallLocation), cluster.ServerIPs[i])
		Expect(err).NotTo(HaveOccurred())
	}

	res, _ := shared.RunCommandHost("kubectl get nodes --kubeconfig=" + factory.KubeConfigFile)
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
	var startFirst []string
	var startLast []string
	for _, serverIP := range cluster.ServerIPs {
		if serverIP == cluster.ServerIPs[0] {
			startFirst = append(startFirst, serverIP)
			continue
		}
		startLast = append(startLast, serverIP)
	}

	_, startErr := shared.ManageService(cluster.Config.Product, "start", "server", startFirst)
	Expect(startErr).NotTo(HaveOccurred())
	time.Sleep(10 * time.Second)

	_, startLastErr := shared.ManageService(cluster.Config.Product, "start", "server", startLast)
	Expect(startLastErr).NotTo(HaveOccurred())
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
