package testcase

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/rancher/distros-test-framework/config"
	"github.com/rancher/distros-test-framework/factory"
	"github.com/rancher/distros-test-framework/pkg/assert"
	"github.com/rancher/distros-test-framework/pkg/customflag"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// TestUpgradeClusterSUC upgrades cluster using the system-upgrade-controller.
func TestUpgradeClusterSUC(cfg *config.Product, version string) error {
	fmt.Printf("\nUpgrading cluster to: %s\n", version)

	workloadErr := shared.ManageWorkload("apply", "suc.yaml")
	Expect(workloadErr).NotTo(HaveOccurred(),
		"system-upgrade-controller manifest did not deploy successfully")

	getPodsSystemUpgrade := "kubectl get pods -n system-upgrade --kubeconfig="
	err := assert.CheckComponentCmdHost(
		getPodsSystemUpgrade+shared.KubeConfigFile,
		"system-upgrade-controller",
		statusRunning,
	)
	Expect(err).NotTo(HaveOccurred(), err)

	originalFilePath := shared.BasePath() +
		fmt.Sprintf("/workloads/amd64/%s-upgrade-plan.yaml", cfg.Product)
	newFilePath := shared.BasePath() + "/workloads/amd64/plan.yaml"

	content, err := os.ReadFile(originalFilePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %s", err)
	}

	newContent := strings.ReplaceAll(string(content), "$UPGRADEVERSION", version)
	err = os.WriteFile(newFilePath, []byte(newContent), 0644)
	if err != nil {
		return fmt.Errorf("failed to write file: %s", err)
	}

	workloadErr = shared.ManageWorkload("apply", "plan.yaml")
	Expect(workloadErr).NotTo(HaveOccurred(), "failed to upgrade cluster.")

	return nil
}

// TestUpgradeClusterManually upgrades the cluster "manually"
func TestUpgradeClusterManually(cluster *factory.Cluster, version string) error {
	fmt.Printf("\nUpgrading cluster to: %s", version)

	if version == "" {
		return shared.ReturnLogError("please provide a non-empty version or commit to upgrade to")
	}

	if cluster.NumServers == 0 && cluster.NumAgents == 0 {
		return shared.ReturnLogError("no nodes found to upgrade")
	}

	if cluster.NumServers > 0 {
		if err := upgradeServer(cluster.Config.Product, version, cluster.ServerIPs); err != nil {
			return err
		}
	}

	if cluster.NumAgents > 0 {
		if err := upgradeAgent(cluster.Config.Product, version, cluster.AgentIPs); err != nil {
			return err
		}
	}

	return nil
}

// upgradeProduct upgrades a node server or agent type to the specified version
func upgradeProduct(product, nodeType string, installType string, ips []string) error {
	var wg sync.WaitGroup
	errCh := make(chan error, len(ips))

	upgradeCommand := getInstallCmd(product, installType, nodeType)

	for _, ip := range ips {
		wg.Add(1)
		go func(ip, upgradeCommand string) {
			defer wg.Done()
			defer GinkgoRecover()

			fmt.Printf("\nUpgrading %s %s to: %s", ip, nodeType, upgradeCommand)

			if _, err := shared.RunCommandOnNode(upgradeCommand, ip); err != nil {
				shared.LogLevel("\nwarn", fmt.Sprintf("upgrading %s %s: %v", nodeType, ip, err))
				errCh <- err
				return
			}

			fmt.Println("\nRestarting " + nodeType + ": " + ip)
			shared.RestartCluster(product, ip)
		}(ip, upgradeCommand)
	}
	wg.Wait()
	close(errCh)

	return nil
}

func getInstallCmd(product, installType string, nodeType string) string {
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

func upgradeServer(product, installType string, serverIPs []string) error {
	return upgradeProduct(product, "server", installType, serverIPs)
}

func upgradeAgent(product, installType string, agentIPs []string) error {
	return upgradeProduct(product, "agent", installType, agentIPs)
}
