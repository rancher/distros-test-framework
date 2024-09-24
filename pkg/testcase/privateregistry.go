package testcase

import (
	"fmt"
	"os"

	"github.com/rancher/distros-test-framework/pkg/customflag"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/gomega"
)

var token string

func TestBuildAirgapCluster(cluster *shared.Cluster) {
	Expect(cluster.Status).To(Equal("cluster created"))
	Expect(cluster.ServerIPs).ShouldNot(BeEmpty())

	if cluster.BastionConfig.PublicIPv4Addr != "" {
		shared.LogLevel("info", "Bastion Public IP: "+cluster.BastionConfig.PublicIPv4Addr)
		shared.LogLevel("info", "Bastion Public DNS: "+cluster.BastionConfig.PublicDNS)
	}
	shared.LogLevel("info", "Server Node IPs: %v", cluster.ServerIPs)

	checkAndPrintAgentNodeIPs(cluster.NumAgents, cluster.AgentIPs, false)

	if cluster.Config.Product == "rke2" {
		checkAndPrintAgentNodeIPs(cluster.NumWinAgents, cluster.WinAgentIPs, true)
	}
}

func TestPrivateRegistry(cluster *shared.Cluster, flags *customflag.FlagConfig) {
	shared.LogLevel("info", "Setting bastion as private registry...")
	shared.SetupPrivateRegistry(cluster, flags)

	shared.LogLevel("info", "Copying assets on the airgap nodes...")
	shared.CopyAssetsOnNodes(cluster)

	shared.LogLevel("info", "Installing %v on airgap nodes...", cluster.Config.Product)
	installOnServers(cluster)
	installOnAgents(cluster)
}

func installOnServers(cluster *shared.Cluster) {
	serverFlags := os.Getenv("server_flags")
	for idx, serverIP := range cluster.ServerIPs {
		// Installing product on primary server aka server-1, saving the token.
		if idx == 0 {
			shared.LogLevel("info", "Installing %v on server-1...", cluster.Config.Product)
			cmd := fmt.Sprintf(
				"sudo chmod +x install_product.sh; "+
					`sudo ./install_product.sh "%v" "" "" "server" "%v" "%v"`,
				cluster.Config.Product, serverIP, serverFlags)
			_, err := shared.CmdForPrivateNode(cluster, cmd, serverIP)
			Expect(err).To(BeNil(), err)

			cmd = fmt.Sprintf("sudo cat /var/lib/rancher/%v/server/token", cluster.Config.Product)
			token, err = shared.CmdForPrivateNode(cluster, cmd, serverIP)
			Expect(err).To(BeNil(), err)
			Expect(token).NotTo(BeEmpty())
			shared.LogLevel("debug", "token: "+token)
		}

		// Installing product on additional servers.
		if idx > 0 {
			shared.LogLevel("info", "Installing %v on server-%v...", cluster.Config.Product, idx+1)
			cmd := fmt.Sprintf(
				"sudo chmod +x install_product.sh; "+
					`sudo ./install_product.sh "%v" "%v" "%v" "server" "%v" "%v"`,
				cluster.Config.Product, cluster.ServerIPs[0], token, serverIP, serverFlags)
			_, err := shared.CmdForPrivateNode(cluster, cmd, serverIP)
			Expect(err).To(BeNil(), err)
		}
	}
}

func installOnAgents(cluster *shared.Cluster) {
	agentFlags := os.Getenv("worker_flags")
	for idx, agentIP := range cluster.AgentIPs {
		shared.LogLevel("info", "Install %v on agent-%v...", cluster.Config.Product, idx+1)
		cmd := fmt.Sprintf(
			"sudo chmod +x install_product.sh; "+
				`sudo ./install_product.sh "%v" "%v" "%v" "agent" "%v" "%v"`,
			cluster.Config.Product, cluster.ServerIPs[0], token, agentIP, agentFlags)
		_, err := shared.CmdForPrivateNode(cluster, cmd, agentIP)
		Expect(err).To(BeNil(), err)
	}
}

// DisplayAirgapClusterDetails executes and prints kubectl get nodes,pods on bastion.
func DisplayAirgapClusterDetails(cluster *shared.Cluster) {
	shared.LogLevel("info", "Bastion login: ssh -i %v.pem %v@%v",
		cluster.AwsEc2.KeyName, cluster.AwsEc2.AwsUser,
		cluster.BastionConfig.PublicIPv4Addr)

	cmd := fmt.Sprintf(
		"PATH=$PATH:/var/lib/rancher/%[1]v/bin:/opt/%[1]v/bin; "+
			"KUBECONFIG=/etc/rancher/%[1]v/%[1]v.yaml ",
		cluster.Config.Product)
	cmd += "kubectl get nodes,pods -A -o wide"

	shared.LogLevel("info", "Display cluster details from airgap server-1: "+cmd)
	clusterInfo, err := shared.CmdForPrivateNode(cluster, cmd, cluster.ServerIPs[0])
	Expect(err).To(BeNil(), err)
	shared.LogLevel("info", "\n"+clusterInfo)
}
