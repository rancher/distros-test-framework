package testcase

import (
	"fmt"

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
	}
	log.Infof("Server Node IPs: %v", cluster.ServerIPs)

	checkAndPrintAgentNodeIPs(cluster.NumAgents, cluster.AgentIPs, false)

	if cluster.Config.Product == "rke2" {
		checkAndPrintAgentNodeIPs(cluster.NumWinAgents, cluster.WinAgentIPs, true)
	}
}

func TestAirgapPrivateRegistry(cluster *shared.Cluster, flags *customflag.FlagConfig) {
	// Setting up bastion
	helper.SetupBastion(cluster, flags)
	helper.CopyAssetsOnNodes(cluster)
	
	// Setting up private instances
	var token string
	for idx, serverIP := range cluster.ServerIPs {
		if idx == 0 {
			log.Infof("Installing %v on server node-1...", cluster.Config.Product)
			// ./install_product.sh product serverIP token nodeType privateIP ipv6IP flags
			cmd := fmt.Sprintf(
				"sudo chmod +x install_product.sh ; " +
				"sudo ./install_product.sh %v \"\" \"\" \"server\" \"%v\"",
				cluster.Config.Product, serverIP)
			_, err := helper.CmdForPrivateNode(cluster, cmd, serverIP)
			Expect(err).To(BeNil())
		
			// get token
			cmd = fmt.Sprintf("sudo cat /var/lib/rancher/%v/server/token", cluster.Config.Product)
			token, err = helper.CmdForPrivateNode(cluster, cmd, serverIP)
			Expect(err).To(BeNil())
			Expect(token).NotTo(BeEmpty())
			log.Info("token: ",token)
		}

		if idx > 0 {
			log.Infof("Installing %v on server node-%v...", cluster.Config.Product, idx+1)
			cmd := fmt.Sprintf(
				"sudo chmod +x install_product.sh ; " +
				"sudo ./install_product.sh %v \"%v\" \"%v\" \"server\" \"%v\"",
				cluster.Config.Product, cluster.ServerIPs[0], token, serverIP)
			res, err := helper.CmdForPrivateNode(cluster, cmd, serverIP)
			log.Info(res)
			Expect(err).To(BeNil())
		}
	}

	for idx, agentIP := range cluster.AgentIPs {
		log.Infof("Installing %v on agent node-%v", cluster.Config.Product, idx+1)
		// ./install_product.sh product serverIP token nodeType privateIP ipv6IP flags
		cmd := fmt.Sprintf(
			"sudo chmod +x install_product.sh ; " +
			"sudo ./install_product.sh %v \"%v\" \"%v\" \"agent\" \"%v\"",
			cluster.Config.Product, cluster.ServerIPs[0], token, agentIP)
		_, err := helper.CmdForPrivateNode(cluster, cmd, agentIP)
		Expect(err).To(BeNil())
	}
	
	// display nodes,pods
	cmd := fmt.Sprintf(
		"PATH=$PATH:/var/lib/rancher/%[1]v/bin:/opt/%[1]v/bin; " + 
		"KUBECONFIG=/etc/rancher/%[1]v/%[1]v.yaml ", 
		cluster.Config.Product)
	cmd += "kubectl get nodes,pods -A -o wide"
	
	clusterInfo, err := helper.CmdForPrivateNode(cluster, cmd, cluster.ServerIPs[0])
	Expect(err).To(BeNil())

	log.Infoln(clusterInfo)
}


