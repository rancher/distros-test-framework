package testcase

import (
	"errors"
	"fmt"

	"github.com/rancher/distros-test-framework/internal/pkg/aws"
	"github.com/rancher/distros-test-framework/internal/pkg/k8s"
	"github.com/rancher/distros-test-framework/internal/resources"

	"github.com/rancher/distros-test-framework/internal/provisioning/driver"

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
func TestUpgradeClusterManual(cluster *driver.Cluster, k8sClient *k8s.Client, version string) error {
	resources.LogLevel("info", "Upgrading cluster manually to version: %s", version)

	if version == "" {
		return resources.ReturnLogError("please provide a non-empty version or commit to upgrade to")
	}
	resources.PrintClusterState()

	if cluster.NumServers == 0 && cluster.NumAgents == 0 {
		return resources.ReturnLogError("no nodes found to upgrade")
	}

	// Initialize aws client in case reboot is needed for slemicro
	resources.LogLevel("debug", "Testing Node OS: %s", cluster.NodeOS)
	awsClient := getAwsClient(cluster)

	// Upgrades server nodes sequentially
	if cluster.NumServers > 0 {
		for _, ip := range cluster.ServerIPs {
			if err := upgradeProduct(awsClient, cluster, server, version, ip); err != nil {
				resources.LogLevel("error", "error upgrading %s %s: %v", server, ip, err)
				return err
			}
		}
	}

	// Upgrades agent nodes sequentially
	if cluster.NumAgents > 0 {
		for _, ip := range cluster.AgentIPs {
			if err := upgradeProduct(awsClient, cluster, agent, version, ip); err != nil {
				resources.LogLevel("error", "error upgrading %s %s: %v", agent, ip, err)
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
func runUpgradeCommand(cluster *driver.Cluster, nodeType, installType, ip string) error {
	upgradeCommand := resources.GetInstallCmd(cluster, installType, nodeType)
	resources.LogLevel("info", "Upgrading %s %s: %s", ip, nodeType, upgradeCommand)

	if _, err := resources.RunCommandOnNode(upgradeCommand, ip); err != nil {
		resources.LogLevel("error", "error running cmd on %s %s: %v", nodeType, ip, err)
		return err
	}

	return nil
}

// upgradeProduct upgrades a node server or agent type to the specified version.
func upgradeProduct(awsClient *aws.Client, cluster *driver.Cluster, nodeType, installType, ip string) error {
	nodeOS := cluster.NodeOS
	product := cluster.Config.Product

	err := runUpgradeCommand(cluster, nodeType, installType, ip)
	if err != nil {
		return err
	}

	if nodeOS == "slemicro" {
		rebootNodeAndWait(awsClient, ip)
	}

	actions := []resources.ServiceAction{
		{Service: product, Action: restart, NodeType: nodeType, ExplicitDelay: 0},
		{Service: product, Action: status, NodeType: nodeType, ExplicitDelay: 120},
	}

	if product == "rke2" {
		ms := resources.NewManageService(5, 30)
		output, serviceCmdErr := ms.ManageService(ip, actions)
		Expect(serviceCmdErr).To(BeNil(), "error running %s service %s on %s node: %s", product, restart, nodeType, ip)

		if output != "" {
			Expect(output).To(ContainSubstring("active "),
				fmt.Sprintf("error running %s service %s on %s node: %s", product, restart, nodeType, ip))
		}
	}

	if product == "k3s" {
		ms := resources.NewManageService(3, 10)
		var output string
		var svcCmdErr error
		if nodeOS == "slemicro" {
			sleActions := []resources.ServiceAction{
				{Service: product, Action: stop, NodeType: nodeType, ExplicitDelay: 30},
				{Service: product, Action: start, NodeType: nodeType, ExplicitDelay: 0},
				{Service: product, Action: status, NodeType: nodeType, ExplicitDelay: 120},
			}
			output, svcCmdErr = ms.ManageService(ip, sleActions)
		} else {
			k3sActions := []resources.ServiceAction{
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
