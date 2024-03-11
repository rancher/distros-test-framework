package testcase

import (
	"fmt"
	"os"
	"strings"

	"github.com/rancher/distros-test-framework/config"
	"github.com/rancher/distros-test-framework/factory"
	"github.com/rancher/distros-test-framework/pkg/assert"
	"github.com/rancher/distros-test-framework/shared"

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
		getPodsSystemUpgrade+factory.KubeConfigFile,
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
