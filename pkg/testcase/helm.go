package testcase

import (
	"fmt"
	"strings"

	"github.com/rancher/distros-test-framework/factory"
	"github.com/rancher/distros-test-framework/pkg/assert"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestDeployCertManager(version string) {
	addRepoCmd := "helm repo add jetstack https://charts.jetstack.io && helm repo update"
	applyCrdsCmd := fmt.Sprintf(
		"kubectl apply --kubeconfig=%s --validate=false -f "+
			"https://github.com/jetstack/cert-manager/releases/download/%s/cert-manager.crds.yaml",
		shared.KubeConfigFile, version)
	installCertMgrCmd := fmt.Sprintf("kubectl create namespace cert-manager --kubeconfig=%s && ",
		shared.KubeConfigFile) + fmt.Sprintf(
		"helm install cert-manager jetstack/cert-manager -n cert-manager --version %s --kubeconfig=%s",
		version, shared.KubeConfigFile)
	res, err := shared.RunCommandHost(addRepoCmd, applyCrdsCmd, installCertMgrCmd)
	Expect(err).NotTo(HaveOccurred(),
		"failed to deploy cert-manager via helm: %v\nCommand: %s\nResult: %s\n", err, installCertMgrCmd, res)

	Eventually(func(g Gomega) {
		pods, err := shared.GetPodsByNamespace("cert-manager", false)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(pods).NotTo(BeEmpty())

		for _, pod := range pods {
			processPodStatus(g, pod,
				assert.PodAssertRestart(),
				assert.PodAssertReady(),
				assert.PodAssertStatus())
		}
	}, "120s", "5s").Should(Succeed())

}

func TestDeployRancher(helmVersion, imageVersion string) {
	cluster := factory.ClusterConfig(GinkgoT())
	addRepoCmd := "helm repo add rancher-latest https://releases.rancher.com/server-charts/latest && " +
		"helm repo update"
	installRancherCmd := "kubectl create namespace cattle-system --kubeconfig=" +
		shared.KubeConfigFile + " && helm install rancher rancher-latest/rancher " +
		"-n cattle-system --set global.cattle.psp.enabled=false " +
		fmt.Sprintf("--set hostname=%s --set rancherImageTag=%s --version=%s --kubeconfig=%s",
			cluster.FQDN, imageVersion, helmVersion, shared.KubeConfigFile)
	res, err := shared.RunCommandHost(addRepoCmd, installRancherCmd)
	Expect(err).NotTo(HaveOccurred(),
		"failed to deploy rancher via helm: %v\nCommand: %s\nResult: %s\n", err, installRancherCmd, res)

	Eventually(func(g Gomega) {
		pods, err := shared.GetPodsByNamespace("cattle-system", false)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(pods).NotTo(BeEmpty())

		for _, pod := range pods {
			processPodStatus(g, pod,
				assert.PodAssertRestart(),
				assert.PodAssertReady(),
				assert.PodAssertStatus())
		}
	}, "900s", "10s").Should(Succeed())

	rancherUrl := fmt.Sprintf("https://%s/dashboard/?setup=", cluster.FQDN)
	for _, line := range strings.Split(res, "\n") {
		if strings.HasPrefix(line, "kubectl") {
			bootstrapPassCmd := strings.TrimSpace(line) + " --kubeconfig=" + shared.KubeConfigFile
			bootstrapPassword, err := shared.RunCommandHost(bootstrapPassCmd)
			Expect(err).NotTo(HaveOccurred(),
				"failed to retrieve rancher bootstrap password: %v\nCommand: %s\n", err, bootstrapPassCmd)
			rancherUrl = rancherUrl + bootstrapPassword
			break
		}
	}
	fmt.Println("\nRancher URL:", rancherUrl)

}
