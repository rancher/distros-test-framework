package testcase

import (
	"errors"
	"fmt"
	"time"

	//"sync"

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
		if err := upgradeProduct(k8sClient, cluster.Config.Product, "server", version, cluster.ServerIPs); err != nil {
			return err
		}
	}

	if cluster.NumAgents > 0 {
		if err := upgradeProduct(k8sClient, cluster.Config.Product, "agent", version, cluster.AgentIPs); err != nil {
			return err
		}
	}

	return nil
}

// upgradeProduct upgrades a node server or agent type to the specified version.
func upgradeProduct(k8sClient *k8s.Client, product, nodeType, installType string, ips []string) error {
	// var wg sync.WaitGroup
	// errCh := make(chan error, len(ips))

	upgradeCommand := shared.GetInstallCmd(product, installType, nodeType)

	for _, ip := range ips {
		// wg.Add(1)
		// go func(ip, upgradeCommand string) {
		// 	defer wg.Done()

		// 	shared.LogLevel("info", "Upgrading %s %s: %s", ip, nodeType, upgradeCommand)

		// 	if _, err := shared.RunCommandOnNode(upgradeCommand, ip); err != nil {
		// 		shared.LogLevel("warn", "upgrading %s %s: %v", nodeType, ip, err)
		// 		errCh <- err
		// 		return
		// 	}

		// 	shared.LogLevel("info", "Restarting %s: %s", nodeType, ip)
		// 	err := shared.RestartCluster(product, ip)
		// 	if err != nil {
		// 		return
		// 	}
		// }(ip, upgradeCommand)

		shared.LogLevel("info", "Upgrading %s %s: %s", ip, nodeType, upgradeCommand)
		if _, err := shared.RunCommandOnNode(upgradeCommand, ip); err != nil {
			shared.LogLevel("warn", "upgrading %s %s: %v", nodeType, ip, err)
			//errCh <- err
			return err
		}

		shared.LogLevel("info", "Waiting 60s after installing upgrade...")
		time.Sleep(60 * time.Second)

		if product == "rke2" {
			shared.LogLevel("info", "Restarting %s service on %s: %s", product, nodeType, ip)
			_, err := shared.ManageService(product, "restart", nodeType, []string{ip})
			if err != nil {
				return err
			}
		}

		shared.LogLevel("info", "Waiting 90s for node to stablize after restarting service...")
		time.Sleep(90 * time.Second)

		// err := k8sClient.WaitForNodeReady(ip)
		// if err != nil {
		// 	shared.LogLevel("warn", "error waiting for node with IP: %v to be ready: %w", ip, err)
		// 	return err
		// } 

	}

	//wg.Wait()
	//close(errCh)

	ok, err := k8sClient.CheckClusterHealth(0)
	if err != nil {
		return fmt.Errorf("error waiting for cluster to be ready: %w", err)
	}
	if !ok {
		return errors.New("cluster is not healthy")
	}

	return nil
}
