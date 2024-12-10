package testcase

import (
	"errors"
	"fmt"
	"sync"

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
		if err := upgradeProduct(k8sClient, cluster, "server", version); err != nil {
			return err
		}
	}

	if cluster.NumAgents > 0 {
		if err := upgradeProduct(k8sClient, cluster, "agent", version); err != nil {
			return err
		}
	}

	return nil
}

// upgradeProduct upgrades a node server or agent type to the specified version.
func upgradeProduct(k8sClient *k8s.Client, cluster *shared.Cluster, nodeType, installType string) error {
	var wg sync.WaitGroup

	var ips []string
	if nodeType == "server" {
		ips = cluster.ServerIPs
	} else {
		ips = cluster.AgentIPs
	}

	errCh := make(chan error, len(ips))

	upgradeCommand := shared.GetInstallCmd(cluster.Config.Product, installType, nodeType)

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
			err := shared.RestartCluster(cluster.Config.Product, ip)
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
