package testcase

import (
	"fmt"

	"github.com/rancher/distros-test-framework/factory"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestClusterResetRestore() {
	cluster := factory.ClusterConfig(GinkgoT())
	accessKey := cluster.Config.AwsS3Config.AccessKey
	secretKey := cluster.Config.AwsS3Config.SecretAccessKey

	fmt.Println(accessKey)
	fmt.Println(secretKey)

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
	shared.LogLevel("info", "cluster reset successful")

	deleteDataDirectories(cluster)
	shared.LogLevel("info", "data directories deleted")
	startServer(cluster)
	shared.LogLevel("info", "%s-service started", cluster.Config.Product)
}
