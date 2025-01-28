package testcase

import (
	"errors"
	"fmt"
	"time"

	"github.com/rancher/distros-test-framework/pkg/k8s"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/gomega"
)

const (
	server = "server"
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

	// Upgrades server nodes sequentially
	if cluster.NumServers > 0 {
		for _, ip := range cluster.ServerIPs {
			if err := upgradeProduct(cluster.Config.Product, server, version, ip); err != nil {
				shared.LogLevel("error", "error upgrading %s %s: %v", server, ip, err)
				return err
			}
		}
	}

	// Upgrades agent nodes sequentially
	if cluster.NumAgents > 0 {
		for _, ip := range cluster.AgentIPs {
			if err := upgradeProduct(cluster.Config.Product, agent, version, ip); err != nil {
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

// upgradeProduct upgrades a node server or agent type to the specified version.
func upgradeProduct(product, nodeType, installType, ip string) error {
	upgradeCommand := shared.GetInstallCmd(product, installType, nodeType)
	shared.LogLevel("info", "Upgrading %s %s: %s", ip, nodeType, upgradeCommand)
	if _, err := shared.RunCommandOnNode(upgradeCommand, ip); err != nil {
		shared.LogLevel("error", "error running cmd on %s %s: %v", nodeType, ip, err)
		return err
	}
	status := []shared.ServiceAction{
		{
			Service:  product,
			Action:   "status",
			NodeType: nodeType,
		},
	}

	if product == "rke2" {
		shared.LogLevel("info", "Waiting for 2 mins after installing upgrade...")
		time.Sleep(2 * time.Minute)
		ms := shared.NewManageService(3, 3)
		restart := []shared.ServiceAction{
			{
				Service:  product,
				Action:   "restart",
				NodeType: nodeType,
			},
		}
		output, err := ms.ManageService(ip, restart)
		if output != "" {
			Expect(output).To(ContainSubstring("active "),
				fmt.Sprintf("error starting %s service for %s node ip: %s", product, nodeType, ip))
		}
		Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("error starting %s service on %s", product, ip))

		shared.LogLevel("info", "Waiting for 3 mins after restarting service...")
		time.Sleep(3 * time.Minute)

		output, err = ms.ManageService(ip, status)
		if output != "" {
			Expect(output).To(ContainSubstring("active "),
				fmt.Sprintf("error checking status %s service for %s node ip: %s", product, nodeType, ip))
		}
		Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("error checking status %s service on %s", product, ip))
	}

	if product == "k3s" {
		shared.LogLevel("info", "Waiting for 1 mins after installing upgrade...")
		time.Sleep(1 * time.Minute)
		ms := shared.NewManageService(3, 1)
		output, err := ms.ManageService(ip, status)
		if output != "" {
			Expect(output).To(ContainSubstring("active "),
				fmt.Sprintf("error starting %s service for %s node ip: %s", product, nodeType, ip))
		}
		Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("error starting %s service on %s", product, ip))
	}

	return nil
}
