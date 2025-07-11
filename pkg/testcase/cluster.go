package testcase

import (
	"fmt"
	"strings"
	"time"

	"github.com/rancher/distros-test-framework/pkg/assert"
	"github.com/rancher/distros-test-framework/pkg/testcase/support"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/gomega"
)

func TestBuildCluster(cluster *shared.Cluster) {
	Expect(cluster.Status).To(Equal("cluster created"))
	Expect(shared.KubeConfigFile).ShouldNot(BeEmpty())
	Expect(cluster.ServerIPs).ShouldNot(BeEmpty())

	if strings.Contains(cluster.Config.DataStore, "etcd") {
		shared.LogLevel("info", "\nBackend: %v\n", cluster.Config.DataStore)
	} else {
		shared.LogLevel("info", "\nBackend: %v\n", cluster.Config.ExternalDb)
	}

	if cluster.Config.ExternalDb != "" && cluster.Config.DataStore == "external" {
		cmd := fmt.Sprintf("sudo grep \"datastore-endpoint\" /etc/rancher/%s/config.yaml", cluster.Config.Product)
		res, err := shared.RunCommandOnNode(cmd, cluster.ServerIPs[0])
		Expect(err).NotTo(HaveOccurred())
		Expect(res).Should(ContainSubstring(cluster.Config.RenderedTemplate))
	}

	shared.LogLevel("info", "KUBECONFIG: ")
	err := shared.PrintFileContents(shared.KubeConfigFile)
	Expect(err).NotTo(HaveOccurred(), err)

	shared.LogLevel("info", "BASE64 ENCODED KUBECONFIG:")
	err = shared.PrintBase64Encoded(shared.KubeConfigFile)
	Expect(err).NotTo(HaveOccurred(), err)

	if cluster.BastionConfig.PublicIPv4Addr != "" {
		shared.LogLevel("info", "Bastion Node IP: %v", cluster.BastionConfig.PublicIPv4Addr)
	}
	shared.LogLevel("info", "Server Node IPs: %v", cluster.ServerIPs)

	support.LogAgentNodeIPs(cluster.NumAgents, cluster.AgentIPs, false)

	if cluster.Config.Product == "rke2" {
		support.LogAgentNodeIPs(cluster.NumWinAgents, cluster.WinAgentIPs, true)
	}
}

func TestCreateK3KCluster(cluster *shared.Cluster) {
	scpErr := shared.RunScp(
		cluster, cluster.ServerIPs[0],
		[]string{shared.BasePath() + "/modules/install/k3kcli_install.sh"},
		[]string{"/var/tmp/k3kcli_install.sh"},
	)
	Expect(scpErr).NotTo(HaveOccurred(), scpErr)

	cmd := "sudo cp /var/tmp/k3kcli_install.sh . && sudo chmod +x k3kcli_install.sh && "
	cmd += fmt.Sprintf(`sudo ./k3kcli_install.sh "%v" "linux"`, "v0.3.3")
	res, err := shared.RunCommandOnNode(cmd, cluster.ServerIPs[0])
	Expect(err).NotTo(HaveOccurred(), err)
	shared.LogLevel("info", "%v", res)

	preCmd := "export PATH=$PATH:/var/lib/rancher/rke2/bin:/opt/bin:/usr/local/bin && "
	checkK3KPod := preCmd + "kubectl get pods -n k3k-system --kubeconfig=/etc/rancher/rke2/rke2.yaml"
	checkErr := assert.ValidateOnNode(cluster.ServerIPs[0], checkK3KPod, "Running")
	Expect(checkErr).NotTo(HaveOccurred(), checkErr)

	clusterNS := "mdk3k-ns1"
	clusterName := "mdcluster1"
	cmd = preCmd + "KUBECONFIG=/etc/rancher/rke2/rke2.yaml " +
		"k3kcli cluster create " +
		"--namespace " + clusterNS +
		" --persistence-type ephemeral " +
		"--version v1.33.1-k3s1 " +
		"--mode virtual " +
		clusterName
	shared.LogLevel("debug", "Cmd: %v", cmd)
	res, err = shared.RunCommandOnNode(cmd, cluster.ServerIPs[0])
	Expect(err).NotTo(HaveOccurred(), err)
	shared.LogLevel("info", "\n%s", res)

	shared.LogLevel("info", "Waiting for 2 minutes for the k3k cluster to be up...")
	time.Sleep(2 * time.Minute)

	shared.LogLevel("info", "\nDisplaying cluster info post k3k create...")
	cmd = preCmd + "kubectl get nodes,pods -A -o wide " +
		"--kubeconfig=/etc/rancher/rke2/rke2.yaml"
	shared.LogLevel("debug", "\nCmd: %s", cmd)
	res, err = shared.RunCommandOnNode(cmd, cluster.ServerIPs[0])
	Expect(err).NotTo(HaveOccurred(), err)
	shared.LogLevel("info", "\n%s", res)

	shared.LogLevel("info", "\nChecking pods in namespace: %s", clusterNS)
	checkK3KCluster := preCmd + "kubectl get pods -n " + clusterNS + " --kubeconfig=/etc/rancher/rke2/rke2.yaml"
	checkErr = assert.ValidateOnNode(cluster.ServerIPs[0], checkK3KCluster, "Running")
	Expect(checkErr).NotTo(HaveOccurred(), checkErr)

	shared.LogLevel("info", "\nDisplaying nodes and pods in k3k cluster: %s", clusterName)
	cmd = preCmd + "kubectl get nodes,pods -A -o wide " +
		" --kubeconfig=" + clusterNS + "-" + clusterName + "-kubeconfig.yaml"
	shared.LogLevel("debug", "\nCmd: %s", cmd)
	res, err = shared.RunCommandOnNode(cmd, cluster.ServerIPs[0])
	Expect(err).NotTo(HaveOccurred(), err)
	shared.LogLevel("info", "\n%s", res)
}
