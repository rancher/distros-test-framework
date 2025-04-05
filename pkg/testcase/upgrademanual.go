package testcase

import (
	"errors"
	"fmt"
	"os"

	"github.com/rancher/distros-test-framework/pkg/aws"
	"github.com/rancher/distros-test-framework/pkg/k8s"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/gomega"
)

const (
	server  = "server"
	status  = "status"
	restart = "restart"
)

// TestUpgradeClusterManual upgrades the cluster "manually".
func TestUpgradeClusterManual(cluster *shared.Cluster, k8sClient *k8s.Client, version string) error {
	shared.LogLevel("info", "Upgrading cluster manually to version: %s", version)

	if version == "" {
		return shared.ReturnLogError("please provide a non-empty version or commit to upgrade to")
	}
	shared.PrintClusterState()

	if cluster.NumServers == 0 && cluster.NumAgents == 0 {
		return shared.ReturnLogError("no nodes found to upgrade")
	}

	// Initialize aws client in case reboot is needed for slemicro
	nodeOS := os.Getenv("node_os")
	shared.LogLevel("debug", "Testing Node OS: %s", nodeOS)
	var awsClient *aws.Client
	var clientErr error
	if nodeOS == "slemicro" {
		awsClient, clientErr = aws.AddClient(cluster)
		Expect(clientErr).NotTo(HaveOccurred())
	}

	// Upgrades server nodes sequentially
	if cluster.NumServers > 0 {
		for _, ip := range cluster.ServerIPs {
			if err := upgradeProduct(awsClient, cluster.Config.Product, server, version, ip, nodeOS); err != nil {
				shared.LogLevel("error", "error upgrading %s %s: %v", server, ip, err)
				return err
			}
		}
	}

	// Upgrades agent nodes sequentially
	if cluster.NumAgents > 0 {
		for _, ip := range cluster.AgentIPs {
			if err := upgradeProduct(awsClient, cluster.Config.Product, agent, version, ip, nodeOS); err != nil {
				shared.LogLevel("error", "error upgrading %s %s: %v", agent, ip, err)
				return err
			}
		}
	}

	ok, err := k8sClient.CheckClusterHealth(0)
	if err != nil {
		return fmt.Errorf("error waiting for cluster to be ready: %w", err)
	}
	if !ok {
		return errors.New("cluster is not healthy")
	}

	return nil
}

func rebootInstances(awsClient *aws.Client, ip string) {
	serverInstanceID, getErr := awsClient.GetInstanceIDByIP(ip)
	Expect(getErr).NotTo(HaveOccurred())
	shared.LogLevel("debug", "Rebooting instance id: %s", serverInstanceID)
	rebootError := awsClient.RebootInstance(serverInstanceID)
	Expect(rebootError).NotTo(HaveOccurred())
}

// upgradeProduct upgrades a node server or agent type to the specified version.
func upgradeProduct(awsClient *aws.Client, product, nodeType, installType, ip, nodeOS string) error {
	upgradeCommand := shared.GetInstallCmd(product, installType, nodeType)
	shared.LogLevel("info", "Upgrading %s %s: %s", ip, nodeType, upgradeCommand)
	if _, err := shared.RunCommandOnNode(upgradeCommand, ip); err != nil {
		shared.LogLevel("error", "error running cmd on %s %s: %v", nodeType, ip, err)
		return err
	}

	if nodeOS == "slemicro" {
		rebootInstances(awsClient, ip)
	}

	actions := []shared.ServiceAction{
		{Service: product, Action: restart, NodeType: nodeType, ExplicitDelay: 180},
		{Service: product, Action: status, NodeType: nodeType, ExplicitDelay: 30},
	}

	if product == "rke2" {
		ms := shared.NewManageService(3, 30)
		output, err := ms.ManageService(ip, actions)
		if output != "" {
			Expect(output).To(ContainSubstring("active "),
				fmt.Sprintf("error running %s service %s on %s node: %s", product, restart, nodeType, ip))
		}
		if err != nil {
			return err
		}
	}

	if product == "k3s" {
		ms := shared.NewManageService(3, 10)
		output, err := ms.ManageService(ip, []shared.ServiceAction{actions[1]})
		if output != "" {
			Expect(output).To(ContainSubstring("active "),
				fmt.Sprintf("error running %s service %s on %s node: %s", product, status, nodeType, ip))
		}
		if err != nil {
			return err
		}
	}

	return nil
}
