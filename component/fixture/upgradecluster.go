package fixture

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
	"regexp"

	"github.com/rancher/distros-test-framework/cmd"
	"github.com/rancher/distros-test-framework/lib/assert"
	"github.com/rancher/distros-test-framework/lib/cluster"
	"github.com/rancher/distros-test-framework/lib/shared"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// TestUpgradeClusterSUC upgrades cluster using the system-upgrade-controller.
func TestUpgradeClusterSUC(version string) error {
	fmt.Println("\nUpgrading cluster to version: %s", version)

	_, err := shared.ManageWorkload("create", "suc.yaml")
	Expect(err).NotTo(HaveOccurred(),
		"system-upgrade-controller manifest did not deploy successfully")

	getPodsSystemUpgrade := "kubectl get pods -n system-upgrade --kubeconfig="
	assert.CheckComponentCmdHost(
		getPodsSystemUpgrade+shared.KubeConfigFile,
		"system-upgrade-controller",
		statusRunning,
	)
	Expect(err).NotTo(HaveOccurred())

	originalFilePath := shared.BasePath() + "/resources/workloads" + "/upgrade-plan.yaml"
	newFilePath := shared.BasePath() + "/resources/workloads" + "/plan.yaml"

	content, err := os.ReadFile(originalFilePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %s", err)
	}

	newContent := strings.ReplaceAll(string(content), "$UPGRADEVERSION", version)
	err = os.WriteFile(newFilePath, []byte(newContent), 0644)
	if err != nil {
		return fmt.Errorf("failed to write file: %s", err)
	}

	_, err = shared.ManageWorkload("create", "plan.yaml")
	Expect(err).NotTo(HaveOccurred(), "failed to upgrade cluster.")

	return nil
}

// TestUpgradeClusterManually upgrades cluster "manually"
func TestUpgradeClusterManually(product, version string) error {
	if version == "" {
		return fmt.Errorf("please provide a non-empty version or commit to upgrade")
	}
	cluster := activity.GetCluster(GinkgoT(), product)
	distro := cluster.ClusterType

	if cluster.NumServers == 0 && cluster.NumAgents == 0 {
		return fmt.Errorf("no nodes found to upgrade")
	}

	if cluster.NumServers > 0 {
		if err := upgradeNode(distro, "server", version, cluster.ServerIPs); err != nil {
			return err
		}
	}

	if cluster.NumAgents > 0 {
		if err := upgradeNode(distro, "agent", version, cluster.AgentIPs); err != nil {
			return err
		}
	}

	return nil
}

// upgradeNode upgrades a node server or agent type to the specified version
func upgradeNode(distro string, nodeType string, installMode string, ips []string) error {
	var wg sync.WaitGroup
	errCh := make(chan error, len(ips))

	upgradeCommand := upgradeCommandBuilder(distro, installMode, nodeType)
	for _, ip := range ips {
		wg.Add(1)
		go func(ip, installFlag string) {
			defer wg.Done()
			defer GinkgoRecover()

			fmt.Println("Upgrading "+ distro + nodeType + " using cmd: " + upgradeCommand)
			if _, err := shared.RunCommandOnNode(upgradeCommand, ip); err != nil {
				fmt.Printf("\nError upgrading %s %s %s: %v\n\n", distro, nodeType, ip, err)
				errCh <- err
				close(errCh)
				return
			}

			fmt.Println("Restarting " + nodeType + ": " + ip)
			if _, err := shared.RestartServiceOnNode(distro, ip); err != nil {
				fmt.Printf("\nError restarting %s %s %s: %v\n\n", distro, nodeType, ip, err)
				errCh <- err
				close(errCh)
				return
			}
			time.Sleep(20 * time.Second)
		}(ip, installMode)
	}
	wg.Wait()
	close(errCh)

	return nil
}

func upgradeCommandBuilder(distro, installMode, nodeType string) string {
	var command, mode, installType string
	distroUpper := strings.ToUpper(distro)
	regMatch, err := regexp.MatchString("((rke2r|k3s)[1-5])", installMode)
	if err != nil {
		return fmt.Sprintln("Unable to match regex for version: %s", installMode)
	}
	if strings.HasPrefix(installMode, "v") && regMatch {
		mode = fmt.Sprintf("INSTALL_%s_VERSION=%s", distroUpper, installMode)
	} else {
		mode = fmt.Sprintf("INSTALL_%s_COMMIT=%s", distroUpper, installMode)
	}

	channel := fmt.Sprintf("INSTALL_%s_CHANNEL=%s", distroUpper, "stable")
	if cmd.ServiceFlag.InstallType.Channel != "" {
		channel = fmt.Sprintf("INSTALL_%s_CHANNEL=%s", distroUpper, cmd.ServiceFlag.InstallType.Channel)
	}

	if distro == "k3s" {
		installType = "sh -s - " + nodeType
	} 
	if distro == "rke2" {
		installType = "INSTALL_RKE2_TYPE=" + nodeType + " sh - "
	}

	command = "sudo curl -sfL https://get.%s.io | sudo %s %s %s"
	command = fmt.Sprintf(command, distro, mode, channel, installType)

	return command
}

