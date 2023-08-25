package testcase

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/rancher/distros-test-framework/factory"
	"github.com/rancher/distros-test-framework/pkg/assert"
	"github.com/rancher/distros-test-framework/pkg/customflag"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// TestUpgradeClusterSUC upgrades cluster using the system-upgrade-controller.
func TestUpgradeClusterSUC(version string) error {
	fmt.Printf("\nUpgrading cluster to version: %s\n", version)

	_, err := shared.ManageWorkload("apply", "suc.yaml")
	Expect(err).NotTo(HaveOccurred(),
		"system-upgrade-controller manifest did not deploy successfully")

	getPodsSystemUpgrade := "kubectl get pods -n system-upgrade --kubeconfig="
	err = assert.CheckComponentCmdHost(
		getPodsSystemUpgrade+shared.KubeConfigFile,
		"system-upgrade-controller",
		statusRunning,
	)
	Expect(err).NotTo(HaveOccurred(), err)

	originalFilePath := shared.BasePath() + "/distros-test-framework/workloads/amd64/rke2-upgrade-plan.yaml"
	newFilePath := shared.BasePath() + "/distros-test-framework/workloads/amd64/plan.yaml"

	content, err := os.ReadFile(originalFilePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %s", err)
	}

	newContent := strings.ReplaceAll(string(content), "$UPGRADEVERSION", version)
	err = os.WriteFile(newFilePath, []byte(newContent), 0644)
	if err != nil {
		return fmt.Errorf("failed to write file: %s", err)
	}

	_, err = shared.ManageWorkload("apply", "plan.yaml")
	Expect(err).NotTo(HaveOccurred(), "failed to upgrade cluster.")

	return nil
}

// TestUpgradeClusterManually upgrades the cluster "manually"
func TestUpgradeClusterManually(version string) error {
	fmt.Printf("\nUpgrading cluster to version: %s\n", version)

	if version == "" {
		return fmt.Errorf("please provide a non-empty version or commit to upgrade to")
	}
	cluster := factory.GetCluster(GinkgoT())

	if cluster.NumServers == 0 && cluster.NumAgents == 0 {
		return fmt.Errorf("no nodes found to upgrade")
	}

	if cluster.NumServers > 0 {
		if err := upgradeServer(version, cluster.ServerIPs); err != nil {
			return err
		}
	}

	if cluster.NumAgents > 0 {
		if err := upgradeAgent(version, cluster.AgentIPs); err != nil {
			return err
		}
	}

	return nil
}

// upgradeNode upgrades a node server or agent type to the specified version
func upgradeNode(nodeType string, installType string, ips []string) error {
	var wg sync.WaitGroup
	errCh := make(chan error, len(ips))

	upgradeCommand := getInstallCmd(installType, nodeType)

	for _, ip := range ips {
		wg.Add(1)
		go func(ip, upgradeCommand string) {
			defer wg.Done()
			defer GinkgoRecover()

			fmt.Println("Upgrading " + nodeType + " to: " + upgradeCommand)
			if _, err := shared.RunCommandOnNode(upgradeCommand, ip); err != nil {
				fmt.Printf("\nError upgrading %s %s: %v\n\n", nodeType, ip, err)
				errCh <- err
				close(errCh)
				return
			}

			product, err := shared.GetProduct()
			if err != nil {
				return
			}
			fmt.Println("Restarting " + nodeType + ": " + ip)
			if _, err := shared.RestartCluster(product, ip); err != nil {
				fmt.Printf("\nError restarting %s %s: %v\n\n", nodeType, ip, err)
				errCh <- err
				close(errCh)
				return
			}
			time.Sleep(20 * time.Second)
		}(ip, upgradeCommand)
	}
	wg.Wait()
	close(errCh)

	return nil
}

func getInstallCmd(installType string, nodeType string) string {
	var installFlag string
	var installCmd string
	product, err := shared.GetProduct()
	if err != nil {
		return err.Error()
	}

	var channel = getChannel()

	if strings.HasPrefix(installType, "v") {
		installFlag = fmt.Sprintf("INSTALL_%s_VERSION=%s", strings.ToUpper(product), installType)
	} else {
		installFlag = fmt.Sprintf("INSTALL_%s_COMMIT=%s", strings.ToUpper(product), installType)
	}

	installCmd = fmt.Sprintf("curl -sfL https://get.%s.io | sudo %%s %%s sh -s - %s", product, nodeType)

	return fmt.Sprintf(installCmd, installFlag, channel)
}

func getChannel() string {
	product, err := shared.GetProduct()
	if err != nil {
		return err.Error()
	}
	var defaultChannel = fmt.Sprintf("INSTALL_%s_CHANNEL=%s", strings.ToUpper(product), "stable")

	if customflag.ServiceFlag.InstallType.Channel != "" {
		return fmt.Sprintf("INSTALL_%s_CHANNEL=%s", strings.ToUpper(product),
			customflag.ServiceFlag.InstallType.Channel)
	}

	return defaultChannel
}

func upgradeServer(installType string, serverIPs []string) error {
	return upgradeNode("server", installType, serverIPs)
}

func upgradeAgent(installType string, agentIPs []string) error {
	return upgradeNode("agent", installType, agentIPs)
}
