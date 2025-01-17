package testcase

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rancher/distros-test-framework/pkg/customflag"
	"github.com/rancher/distros-test-framework/pkg/k8s"
	"github.com/rancher/distros-test-framework/shared"
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

	if cluster.NumServers > 0 {
		for _, ip := range cluster.ServerIPs {
			if err := upgradeProduct(k8sClient, cluster.Config.Product, server, version, ip); err != nil {
				return err
			}
			shared.LogLevel("info", "Checking pod status after restarting %v node: %v", server, ip)
			CheckPodStatus(cluster)
		}
	}

	if cluster.NumAgents > 0 {
		for _, ip := range cluster.AgentIPs {
			if err := upgradeProduct(k8sClient, cluster.Config.Product, agent, version, ip); err != nil {
				return err
			}
			shared.LogLevel("info", "Checking pod status after restarting %v node: %v", agent, ip)
			CheckPodStatus(cluster)
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
func upgradeProduct(k8sClient *k8s.Client, product, nodeType, installType, ip string) error {
	upgradeCommand := getInstallCmd(product, installType, nodeType)
	shared.LogLevel("info", "Upgrading %s %s: %s", ip, nodeType, upgradeCommand)
	if _, err := shared.RunCommandOnNode(upgradeCommand, ip); err != nil {
		shared.LogLevel("warn", "upgrading %s %s: %v", nodeType, ip, err)
		return err
	}
	shared.LogLevel("info", "Waiting 30s after installing upgrade...")
	time.Sleep(30 * time.Second)
	if product == "rke2" {
		shared.LogLevel("info", "Restarting %s service on %s node: %s", product, nodeType, ip)
		_, err := shared.ManageService(product, "restart", nodeType, []string{ip})
		if err != nil {
			return err
		}
		shared.LogLevel("info", "Waiting for 180s after restarting service")
		time.Sleep(180 * time.Second)
		shared.LogLevel("info", "Waiting for %v node to be ready: %v", nodeType, ip)
		err = k8sClient.WaitForNodeReady(ip)
		if err != nil {
			return err
		}
	}

	return nil
}

func getInstallCmd(product, installType, nodeType string) string {
	var installFlag string
	var installCmd string

	var channel = getChannel(product)

	if strings.HasPrefix(installType, "v") {
		installFlag = fmt.Sprintf("INSTALL_%s_VERSION=%s", strings.ToUpper(product), installType)
	} else {
		installFlag = fmt.Sprintf("INSTALL_%s_COMMIT=%s", strings.ToUpper(product), installType)
	}

	installCmd = fmt.Sprintf("curl -sfL https://get.%s.io | sudo %%s %%s sh -s - %s", product, nodeType)

	return fmt.Sprintf(installCmd, installFlag, channel)
}

func getChannel(product string) string {
	var defaultChannel = fmt.Sprintf("INSTALL_%s_CHANNEL=%s", strings.ToUpper(product), "stable")

	if customflag.ServiceFlag.Channel.String() != "" {
		return fmt.Sprintf("INSTALL_%s_CHANNEL=%s", strings.ToUpper(product),
			customflag.ServiceFlag.Channel.String())
	}

	return defaultChannel
}
