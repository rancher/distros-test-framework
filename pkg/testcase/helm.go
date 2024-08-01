package testcase

import (
	"fmt"
	"strings"

	"github.com/rancher/distros-test-framework/factory"
	"github.com/rancher/distros-test-framework/pkg/assert"
	"github.com/rancher/distros-test-framework/pkg/customflag"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/gomega"
)

func TestDeployCertManager(cluster *factory.Cluster, version string) {
	addRepoCmd := "helm repo add jetstack https://charts.jetstack.io && helm repo update"
	applyCrdsCmd := fmt.Sprintf(
		"kubectl apply --kubeconfig=%s --validate=false -f "+
			"https://github.com/jetstack/cert-manager/releases/download/%s/cert-manager.crds.yaml",
		factory.KubeConfigFile, version)
	installCertMgrCmd := fmt.Sprintf("kubectl create namespace cert-manager --kubeconfig=%s && ",
		factory.KubeConfigFile) + fmt.Sprintf(
		"helm install cert-manager jetstack/cert-manager -n cert-manager --version %s --kubeconfig=%s",
		version, factory.KubeConfigFile)

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

		for i := range pods {
			processPodStatus(cluster,
				g,
				&pods[i],
				assert.PodAssertRestart(),
				assert.PodAssertReady())
		}
	}, "120s", "5s").Should(Succeed())
}

func TestDeployRancher(cluster *factory.Cluster, flags *customflag.FlagConfig) {
	response := installRancher(cluster, flags)

	filters := map[string]string{
		"namespace": "cattle-system",
		"label":     "app=rancher",
	}

	Eventually(func(g Gomega) {
		pods, err := shared.GetPodsFiltered(filters)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(pods).NotTo(BeEmpty())

		for i := range pods {
			processPodStatus(
				cluster,
				g,
				&pods[i],
				assert.PodAssertRestart(),
				assert.PodAssertReady())
		}
	}, "900s", "10s").Should(Succeed())

	rancherURL := fmt.Sprintf("https://%s/dashboard/?setup=", cluster.FQDN)
	for _, line := range strings.Split(response, "\n") {
		if !strings.HasPrefix(line, "kubectl") {
			continue
		}

		bootstrapPassCmd := strings.TrimSpace(line) + " --kubeconfig=" + factory.KubeConfigFile
		bootstrapPassword, err := shared.RunCommandHost(bootstrapPassCmd)
		Expect(err).NotTo(HaveOccurred(),
			"failed to retrieve rancher bootstrap password: %v\nCommand: %s\n", err, bootstrapPassCmd)

		rancherURL += bootstrapPassword

		break
	}
	shared.LogLevel("info", "\nRancher URL: %s", rancherURL)
}

func installRancher(cluster *factory.Cluster, flags *customflag.FlagConfig) string {
	addRepoCmd := fmt.Sprintf(
		"helm repo add %s %s && "+
			"helm repo update",
		flags.HelmCharts.RepoName,
		flags.HelmCharts.RepoUrl)

	installRancherCmd := fmt.Sprintf(
		"kubectl create namespace cattle-system --kubeconfig=%s && "+
			"helm install rancher %s/rancher ",
		factory.KubeConfigFile,
		flags.HelmCharts.RepoName)

	if flags.HelmCharts.Args != "" {
		installRancherCmd += helmArgsBuilder(flags)
	}

	installRancherCmd += fmt.Sprintf("-n cattle-system "+
		"--version=%s "+
		"--set global.cattle.psp.enabled=false "+
		"--set hostname=%s "+
		"--kubeconfig=%s",
		flags.RancherConfig.RancherVersion,
		cluster.FQDN,
		factory.KubeConfigFile)

	shared.LogLevel("info", "Helm command: %s", addRepoCmd)
	response, err := shared.RunCommandHost(addRepoCmd)
	Expect(err).NotTo(HaveOccurred(), "failed to add helm repo: %v\nCommand: %s\nResult: %s\n", err, addRepoCmd, response)

	shared.LogLevel("info", "Install command: %s", installRancherCmd)
	response, err = shared.RunCommandHost(installRancherCmd)
	Expect(err).NotTo(HaveOccurred(), "failed to deploy rancher via helm: %v\nCommand: %s\nResult: %s\n",
		err, installRancherCmd, response)

	return response
}

func helmArgsBuilder(flags *customflag.FlagConfig) (finalArgs string) {
	helmArgs := flags.HelmCharts.Args
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
