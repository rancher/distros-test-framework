package testcase

import (
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rancher/distros-test-framework/factory"
	"github.com/rancher/distros-test-framework/pkg/assert"
	"github.com/rancher/distros-test-framework/pkg/customflag"
	"github.com/rancher/distros-test-framework/shared"
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

	filters := map[string]string{
		"namespace": "cert-manager",
	}
	Eventually(func(g Gomega) {
		pods, err := shared.GetPodsFiltered(filters)
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

func TestDeployRancher(flags *customflag.FlagConfig) {
	cluster := factory.ClusterConfig(GinkgoT())
	addRepoCmd := fmt.Sprintf(
		"helm repo add %s %s && "+
			"helm repo update",
		flags.ExternalFlag.HelmChartsFlag.RepoName,
		flags.ExternalFlag.HelmChartsFlag.RepoUrl)

	installRancherCmd := fmt.Sprintf(
		"kubectl create namespace cattle-system --kubeconfig=%s && "+
			"helm install rancher %s/rancher ",
		shared.KubeConfigFile,
		flags.ExternalFlag.HelmChartsFlag.RepoName)

	if flags.ExternalFlag.HelmChartsFlag.Args != "" {
		installRancherCmd += helmArgsBuilder(flags)
	}

	installRancherCmd += fmt.Sprintf("-n cattle-system "+
		"--version=%s "+
		"--set global.cattle.psp.enabled=false "+
		"--set hostname=%s "+
		"--kubeconfig=%s",
		flags.ExternalFlag.RancherVersion,
		cluster.FQDN,
		shared.KubeConfigFile)

	shared.LogLevel("info", "Helm command: %s", addRepoCmd)
	res, err := shared.RunCommandHost(addRepoCmd)
	Expect(err).NotTo(HaveOccurred(),
		"failed to add helm repo: %v\nCommand: %s\nResult: %s\n", err, addRepoCmd, res)
	shared.LogLevel("info", "Install command: %s", installRancherCmd)
	res, err = shared.RunCommandHost(installRancherCmd)
	Expect(err).NotTo(HaveOccurred(),
		"failed to deploy rancher via helm: %v\nCommand: %s\nResult: %s\n", err, installRancherCmd, res)

	filters := map[string]string{
		"namespace": "cattle-system",
		"label":     "app=rancher",
	}
	Eventually(func(g Gomega) {
		pods, err := shared.GetPodsFiltered(filters)
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

func helmArgsBuilder(flags *customflag.FlagConfig) (finalArgs string) {
	helmArgs := flags.ExternalFlag.HelmChartsFlag.Args
	if strings.Contains(helmArgs, ",") {
		argsSlice := strings.Split(helmArgs, ",")
		for _, arg := range argsSlice {
			if !strings.Contains(finalArgs, arg) {
				finalArgs += fmt.Sprintf("--set %s ", arg)
			}
		}
	} else {
		finalArgs = fmt.Sprintf("--set %s ", helmArgs)
	}

	return finalArgs
}
