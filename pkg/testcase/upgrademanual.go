package testcase

import (
	"errors"
	"fmt"
	"time"

	"github.com/rancher/distros-test-framework/pkg/aws"
	"github.com/rancher/distros-test-framework/pkg/k8s"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/gomega"
)

const (
	server  = "server"
	status  = "status"
	restart = "restart"
	stop    = "stop"
	start   = "start"
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
	shared.LogLevel("debug", "Testing Node OS: %s", cluster.NodeOS)
	awsClient := getAwsClient(cluster)

	// Upgrades server nodes sequentially
	if cluster.NumServers > 0 {
		for _, ip := range cluster.ServerIPs {
			if err := upgradeProduct(awsClient, cluster, server, version, ip); err != nil {
				shared.LogLevel("error", "error upgrading %s %s: %v", server, ip, err)
				return err
			}
		}
	}

	// Upgrades agent nodes sequentially
	if cluster.NumAgents > 0 {
		for _, ip := range cluster.AgentIPs {
			if err := upgradeProduct(awsClient, cluster, agent, version, ip); err != nil {
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

// nodeType can be server or agent.
// installType can be version or commit.
func runUpgradeCommand(cluster *shared.Cluster, nodeType, installType, ip string) error {
	upgradeCommand := shared.GetInstallCmd(cluster, installType, nodeType)
	shared.LogLevel("info", "Upgrading %s %s: %s", ip, nodeType, upgradeCommand)

	if _, err := shared.RunCommandOnNode(upgradeCommand, ip); err != nil {
		shared.LogLevel("error", "error running cmd on %s %s: %v", nodeType, ip, err)
		return err
	}

	return nil
}

// upgradeProduct upgrades a node server or agent type to the specified version.
func upgradeProduct(awsClient *aws.Client, cluster *shared.Cluster, nodeType, installType, ip string) error {
	nodeOS := cluster.NodeOS
	product := cluster.Config.Product

	err := runUpgradeCommand(cluster, nodeType, installType, ip)
	if err != nil {
		return err
	}

	if nodeOS == "slemicro" {
		// rebootNodeAndWait(awsClient, ip)
		restart, err := shared.RunCommandOnNode("sudo reboot", ip)
		if err != nil {
			shared.LogLevel("error", "error rebooting %s node %s: %v", nodeType, ip, err)
			return fmt.Errorf("error rebooting %s node %s: %w", nodeType, ip, err)
		}
		shared.LogLevel("info", "Reboot command output for %s node %s: %s", nodeType, ip, restart)

		time.Sleep(120 * time.Second)
		sshErr := shared.WaitForSSHReady(ip)
		if sshErr != nil {
			shared.LogLevel("error", "error waiting for SSH to be ready on %s node %s: %v", nodeType, ip, sshErr)
			return fmt.Errorf("error waiting for SSH to be ready on %s node %s: %w", nodeType, ip, sshErr)
		}
	}

	actions := []shared.ServiceAction{
		{Service: product, Action: restart, NodeType: nodeType, ExplicitDelay: 0},
		{Service: product, Action: status, NodeType: nodeType, ExplicitDelay: 120},
	}

	if product == "rke2" {
		ms := shared.NewManageService(5, 30)
		output, serviceCmdErr := ms.ManageService(ip, actions)
		Expect(serviceCmdErr).To(BeNil(), "error running %s service %s on %s node: %s", product, restart, nodeType, ip)

		if output != "" {
			Expect(output).To(ContainSubstring("active "),
				fmt.Sprintf("error running %s service %s on %s node: %s", product, restart, nodeType, ip))
		}
	}

	if product == "k3s" {
		ms := shared.NewManageService(3, 10)
		var output string
		var svcCmdErr error
		if nodeOS == "slemicro" {
			sleActions := []shared.ServiceAction{
				{Service: product, Action: stop, NodeType: nodeType, ExplicitDelay: 30},
				{Service: product, Action: start, NodeType: nodeType, ExplicitDelay: 0},
				{Service: product, Action: status, NodeType: nodeType, ExplicitDelay: 120},
			}
			output, svcCmdErr = ms.ManageService(ip, sleActions)
		} else {
			k3sActions := []shared.ServiceAction{
				{Service: product, Action: status, NodeType: nodeType, ExplicitDelay: 30},
			}
			output, svcCmdErr = ms.ManageService(ip, k3sActions)
		}
		Expect(svcCmdErr).To(BeNil(), "error running %s service %s on %s node: %s", product, status, nodeType, ip)

		if output != "" {
			Expect(output).To(ContainSubstring("active "),
				fmt.Sprintf("error running %s service %s on %s node: %s", product, status, nodeType, ip))
		}
	}

	return nil
}
