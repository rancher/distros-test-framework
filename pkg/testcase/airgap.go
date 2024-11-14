package testcase

import (
	"fmt"
	"os"
	"strings"

	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/gomega"
)

var token string

const (
	PrivateRegistry       = "private_registry"
	SystemDefaultRegistry = "system_default_registry"
)

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

func installOnServers(cluster *shared.Cluster) {
	serverFlags := os.Getenv("server_flags")
	if !strings.Contains(serverFlags, "system-default-registry") {
		serverFlags += "\nsystem-default-registry: " + cluster.BastionConfig.PublicDNS
	}

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
	if !strings.Contains(agentFlags, "system-default-registry") {
		agentFlags += "\nsystem-default-registry: " + cluster.BastionConfig.PublicDNS
	}

	for idx, agentIP := range cluster.AgentIPs {
		shared.LogLevel("info", "Installing %v on agent-%v...", cluster.Config.Product, idx+1)
		cmd := fmt.Sprintf(
			"sudo chmod +x install_product.sh; "+
				`sudo ./install_product.sh "%v" "%v" "%v" "agent" "%v" "%v"`,
			cluster.Config.Product, cluster.ServerIPs[0], token, agentIP, agentFlags)
		_, err := shared.CmdForPrivateNode(cluster, cmd, agentIP)
		Expect(err).To(BeNil(), err)
	}
}
