package testcase

import (
	"fmt"
	"os"
	"strings"

	"github.com/rancher/distros-test-framework/pkg/assert"
	"github.com/rancher/distros-test-framework/pkg/k8s"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/gomega"
)

// TestUpgradeClusterSUC upgrades cluster using the system-upgrade-controller.
func TestUpgradeClusterSUC(cluster *shared.Cluster, k8sClient *k8s.Client, version string) error {
	shared.PrintClusterState()

	shared.LogLevel("info", "Upgrading SUC to version: %s\n", version)

	sucApplyErr := shared.ManageWorkload("apply", "suc.yaml")
	Expect(sucApplyErr).NotTo(HaveOccurred(),
		"system-upgrade-controller manifest did not deploy successfully")

	crdApplyErr := shared.ManageWorkload("apply", "suc_crd.yaml")
	Expect(crdApplyErr).NotTo(HaveOccurred(),
		"suc_crd.yaml apply did not deploy successfully")

	getPodsSystemUpgrade := "kubectl get pods -n system-upgrade --kubeconfig="
	err := assert.CheckComponentCmdHost(
		getPodsSystemUpgrade+shared.KubeConfigFile,
		"system-upgrade-controller",
		statusRunning,
	)
	Expect(err).NotTo(HaveOccurred(), err)

	originalFilePath := shared.BasePath() + fmt.Sprintf("/workloads/%s/%s-",
		cluster.Config.Arch, cluster.Config.Product)
	if os.Getenv("split_roles") == "true" {
		originalFilePath += "suc-plan-splitroles.yaml"
	} else {
		originalFilePath += "suc-plan.yaml"
	}
	shared.LogLevel("debug", "Using plan in path: %s", originalFilePath)
	newFilePath := shared.BasePath() + fmt.Sprintf("/workloads/%s/plan.yaml", cluster.Config.Arch)

	content, err := os.ReadFile(originalFilePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %s", err)
	}

	newContent := strings.ReplaceAll(string(content), "$UPGRADEVERSION", version)
	err = os.WriteFile(newFilePath, []byte(newContent), 0o644)
	if err != nil {
		return fmt.Errorf("failed to write file: %s", err)
	}

	planApplyErr := shared.ManageWorkload("apply", "plan.yaml")
	Expect(planApplyErr).NotTo(HaveOccurred(), "failed to upgrade cluster - apply plan.yaml step failed.")

	ok, err := k8sClient.CheckClusterHealth(0)
	Expect(err).NotTo(HaveOccurred(), err, "error checking cluster health")
	Expect(ok).To(BeTrue(), "cluster health check failed")

	return nil
}
