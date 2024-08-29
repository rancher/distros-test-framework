package testcase

import (
	"fmt"
	"os"

	"github.com/rancher/distros-test-framework/pkg/customflag"
	"github.com/rancher/distros-test-framework/pkg/helper"
	"github.com/rancher/distros-test-framework/pkg/logger"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/gomega"
)

var log = logger.AddLogger()

func TestBuildPrivateCluster(cluster *shared.Cluster) {
	Expect(cluster.Status).To(Equal("cluster created"))
	Expect(cluster.ServerIPs).ShouldNot(BeEmpty())

	if cluster.GeneralConfig.BastionIP != "" {
		log.Infof("Bastion Node IP: %v", cluster.GeneralConfig.BastionIP)
		log.Infof("Bastion Node DNS: %v", cluster.GeneralConfig.BastionDNS)
	}
	log.Infof("Server Node IPs: %v", cluster.ServerIPs)

	checkAndPrintAgentNodeIPs(cluster.NumAgents, cluster.AgentIPs, false)

	if cluster.Config.Product == "rke2" {
		checkAndPrintAgentNodeIPs(cluster.NumWinAgents, cluster.WinAgentIPs, true)
	}
}

func TestPrivateRegistry(cluster *shared.Cluster, flags *customflag.FlagConfig) {
	serverFlags := os.Getenv("server_flags")
	agentFlags := os.Getenv("worker_flags")
	helper.SetupBastion(cluster, flags)
	helper.CopyAssetsOnNodes(cluster)
	var token string
	for idx, serverIP := range cluster.ServerIPs {
		if idx == 0 {
			log.Infof("Installing %v on server node-1...", cluster.Config.Product)
			cmd := fmt.Sprintf(
				"sudo chmod +x install_product.sh; "+
					`sudo ./install_product.sh "%v" "" "" "server" "%v" "%v"`,
				cluster.Config.Product, serverIP, serverFlags)
			_, err := helper.CmdForPrivateNode(cluster, cmd, serverIP)
			Expect(err).To(BeNil())

			cmd = fmt.Sprintf("sudo cat /var/lib/rancher/%v/server/token", cluster.Config.Product)
			token, err = helper.CmdForPrivateNode(cluster, cmd, serverIP)
			Expect(err).To(BeNil())
			Expect(token).NotTo(BeEmpty())
			log.Info("token: ", token)
		}
		if idx > 0 {
			log.Infof("Installing %v on server node-%v...", cluster.Config.Product, idx+1)
			cmd := fmt.Sprintf(
				"sudo chmod +x install_product.sh; "+
					`sudo ./install_product.sh "%v" "%v" "%v" "server" "%v" "%v"`,
				cluster.Config.Product, cluster.ServerIPs[0], token, serverIP, serverFlags)
			_, err := helper.CmdForPrivateNode(cluster, cmd, serverIP)
			Expect(err).To(BeNil())
		}
	}

	for idx, agentIP := range cluster.AgentIPs {
		log.Infof("Installing %v on agent node-%v", cluster.Config.Product, idx+1)
		cmd := fmt.Sprintf(
			"sudo chmod +x install_product.sh; "+
				`sudo ./install_product.sh "%v" "%v" "%v" "agent" "%v" "%v"`,
			cluster.Config.Product, cluster.ServerIPs[0], token, agentIP, agentFlags)
		_, err := helper.CmdForPrivateNode(cluster, cmd, agentIP)
		Expect(err).To(BeNil())
	}

	log.Infof("Bastion login: ssh -i %v.pem %v@%v",
		cluster.AwsEc2.KeyName, cluster.AwsEc2.AwsUser,
		cluster.GeneralConfig.BastionIP)
	cmd := fmt.Sprintf(
		"PATH=$PATH:/var/lib/rancher/%[1]v/bin:/opt/%[1]v/bin; "+
			"KUBECONFIG=/etc/rancher/%[1]v/%[1]v.yaml ",
		cluster.Config.Product)
	cmd += "kubectl get nodes,pods -A -o wide"
	log.Info("Running command in private server-1: " + cmd)
	clusterInfo, err := helper.CmdForPrivateNode(cluster, cmd, cluster.ServerIPs[0])
	Expect(err).To(BeNil())
	log.Info("\n\n" + clusterInfo)
}
