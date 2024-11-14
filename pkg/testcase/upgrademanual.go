package testcase

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/rancher/distros-test-framework/pkg/customflag"
	"github.com/rancher/distros-test-framework/pkg/k8s"
	"github.com/rancher/distros-test-framework/shared"
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
		if err := upgradeServer(*k8sClient, cluster.Config.Product, version, cluster.ServerIPs); err != nil {
			return err
		}
	}

	if cluster.NumAgents > 0 {
		if err := upgradeAgent(*k8sClient, cluster.Config.Product, version, cluster.AgentIPs); err != nil {
			return err
		}
	}

	return nil
}

// upgradeProduct upgrades a node server or agent type to the specified version.
func upgradeProduct(k8sClient k8s.Client, product, nodeType, installType string, ips []string) error {
	var wg sync.WaitGroup
	errCh := make(chan error, len(ips))

	upgradeCommand := getInstallCmd(product, installType, nodeType)

	for _, ip := range ips {
		wg.Add(1)
		go func(ip, upgradeCommand string) {
			defer wg.Done()

			shared.LogLevel("info", fmt.Sprintf("Upgrading %s %s: %s", ip, nodeType, upgradeCommand))

			if _, err := shared.RunCommandOnNode(upgradeCommand, ip); err != nil {
				shared.LogLevel("warn", fmt.Sprintf("upgrading %s %s: %v", nodeType, ip, err))
				errCh <- err
				return
			}

			shared.LogLevel("info", "Restarting %s: %s", nodeType, ip)
			err := shared.RestartCluster(product, ip)
			if err != nil {
				return
			}
		}(ip, upgradeCommand)
	}

	wg.Wait()
	close(errCh)

	ok, err := k8sClient.CheckClusterHealth(0)
	if err != nil {
		return fmt.Errorf("error waiting for cluster to be ready: %w", err)
	}
	if !ok {
		return errors.New("cluster is not healthy")
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

func upgradeServer(k8sClient k8s.Client, product, installType string, serverIPs []string) error {
	return upgradeProduct(k8sClient, product, "server", installType, serverIPs)
}

func upgradeAgent(k8sClient k8s.Client, product, installType string, agentIPs []string) error {
	return upgradeProduct(k8sClient, product, "agent", installType, agentIPs)
}
