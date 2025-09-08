package testcase

import (
	"fmt"
	"os"
	"strings"

	"github.com/rancher/distros-test-framework/internal/pkg/assert"
	"github.com/rancher/distros-test-framework/internal/pkg/k8s"

	"github.com/rancher/distros-test-framework/internal/resources"

	"github.com/rancher/distros-test-framework/internal/provisioning/driver"

	. "github.com/onsi/gomega"
)

// TestUpgradeClusterSUC upgrades cluster using the system-upgrade-controller.
func TestUpgradeClusterSUC(cluster *driver.Cluster, k8sClient *k8s.Client, version string) error {
	resources.PrintClusterState()

	resources.LogLevel("info", "Upgrading SUC to version: %s\n", version)

	applySucYamls()

	getPodsSystemUpgrade := "kubectl get pods -n system-upgrade --kubeconfig="
	err := assert.CheckComponentCmdHost(
		getPodsSystemUpgrade+resources.KubeConfigFile,
		"system-upgrade-controller",
		statusRunning,
	)
	Expect(err).NotTo(HaveOccurred(), err)

	originalFilePath := resources.BasePath() + fmt.Sprintf("/workloads/%s/%s-",
		cluster.Config.Arch, cluster.Config.Product)
	if os.Getenv("split_roles") == "true" {
		originalFilePath += "suc-plan-splitroles.yaml"
	} else {
		originalFilePath += "suc-plan.yaml"
	}
	resources.LogLevel("debug", "Using plan in path: %s", originalFilePath)
	newFilePath := resources.BasePath() + fmt.Sprintf("/workloads/%s/plan.yaml", cluster.Config.Arch)

	content, err := os.ReadFile(originalFilePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %s", err)
	}

	newContent := strings.ReplaceAll(string(content), "$UPGRADEVERSION", version)
	err = os.WriteFile(newFilePath, []byte(newContent), 0o644)
	if err != nil {
		return fmt.Errorf("failed to write file: %s", err)
	}

	planApplyErr := resources.ManageWorkload("apply", "plan.yaml")
	Expect(planApplyErr).NotTo(HaveOccurred(), "failed to upgrade cluster - apply plan.yaml step failed.")

	ok, err := k8sClient.CheckClusterHealth(0)
	Expect(err).NotTo(HaveOccurred(), err, "error checking cluster health")
	Expect(ok).To(BeTrue(), "cluster health check failed")

	return nil
}

func applySucYamls() {
	sucUrl := "https://github.com/rancher/system-upgrade-controller/releases/latest/download/system-upgrade-controller.yaml"
	sucCRDUrl := "https://github.com/rancher/system-upgrade-controller/releases/latest/download/crd.yaml"

	resources.LogLevel("info", "Applying system-upgrade-controller manifest from url: %s", sucUrl)
	applyErr := resources.ApplyWorkloadURL(sucUrl)
	if applyErr != nil {
		resources.LogLevel(
			"warn", "error applying system-upgrade-controller manifest from url: %s error: %v", sucUrl, applyErr)
		resources.LogLevel("debug", "applying system-upgrade-controller manifest from local file")
		// Fallback to local file if URL fails
		applyErr = resources.ManageWorkload("apply", "suc.yaml")
	}
	Expect(applyErr).NotTo(HaveOccurred(),
		"system-upgrade-controller manifest did not deploy successfully")

	resources.LogLevel("debug", "Applying SUC CRD manifest from url: %s", sucCRDUrl)
	applyErr = resources.ApplyWorkloadURL(sucCRDUrl)
	if applyErr != nil {
		resources.LogLevel("warn", "error applying SUC CRD manifest from url: %s error: %v", sucCRDUrl, applyErr)
		resources.LogLevel("debug", "applying SUC CRD manifest from local file")
		// Fallback to local file if URL fails
		applyErr = resources.ManageWorkload("apply", "suc_crd.yaml")
	}
	Expect(applyErr).NotTo(HaveOccurred(),
		"suc_crd.yaml apply did not deploy successfully")
}
