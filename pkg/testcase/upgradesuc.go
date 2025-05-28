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

	applySucYamls()

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

func applySucYamls() {
	sucUrl := "https://github.com/rancher/system-upgrade-controller/releases/latest/download/system-upgrade-controller.yaml"
	sucCRDUrl := "https://github.com/rancher/system-upgrade-controller/releases/latest/download/crd.yaml"

	shared.LogLevel("info", "Applying system-upgrade-controller manifest from url: %s", sucUrl)
	applyErr := shared.ApplyWorkloadURL(sucUrl)
	if applyErr != nil {
		shared.LogLevel(
			"warn", "error applying system-upgrade-controller manifest from url: %s error: %v", sucUrl, applyErr)
		shared.LogLevel("debug", "applying system-upgrade-controller manifest from local file")
		// Fallback to local file if URL fails
		applyErr = shared.ManageWorkload("apply", "suc.yaml")
	}
	Expect(applyErr).NotTo(HaveOccurred(),
		"system-upgrade-controller manifest did not deploy successfully")

	shared.LogLevel("debug", "Applying SUC CRD manifest from url: %s", sucCRDUrl)
	applyErr = shared.ApplyWorkloadURL(sucCRDUrl)
	if applyErr != nil {
		shared.LogLevel("warn", "error applying SUC CRD manifest from url: %s error: %v", sucCRDUrl, applyErr)
		shared.LogLevel("debug", "applying SUC CRD manifest from local file")
		// Fallback to local file if URL fails
		applyErr = shared.ManageWorkload("apply", "suc_crd.yaml")
	}
	Expect(applyErr).NotTo(HaveOccurred(),
		"suc_crd.yaml apply did not deploy successfully")
}
