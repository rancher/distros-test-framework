package testcase

import (
	"fmt"
	"strings"

	"github.com/rancher/distros-test-framework/internal/pkg/assert"
	"github.com/rancher/distros-test-framework/internal/pkg/customflag"
	"github.com/rancher/distros-test-framework/internal/provisioning/driver"
	"github.com/rancher/distros-test-framework/internal/resources"

	. "github.com/onsi/gomega"
)

func TestDeployCertManager(cluster *driver.Cluster, version string) {
	err := addRepo("jetstack", "https://charts.jetstack.io")
	Expect(err).To(BeNil(), err)

	applyCrdsCmd := fmt.Sprintf(
		"kubectl apply --kubeconfig=%s --validate=false -f "+
			"https://github.com/jetstack/cert-manager/releases/download/%s/cert-manager.crds.yaml",
		resources.KubeConfigFile, version)
	installCertMgrCmd := fmt.Sprintf("kubectl create namespace cert-manager --kubeconfig=%s && ",
		resources.KubeConfigFile) + fmt.Sprintf(
		"helm install cert-manager jetstack/cert-manager -n cert-manager --version %s --kubeconfig=%s",
		version, resources.KubeConfigFile)

	res, err := resources.RunCommandHost(applyCrdsCmd, installCertMgrCmd)
	Expect(err).NotTo(HaveOccurred(),
		"failed to deploy cert-manager via helm: %v\nCommand: %s\nResult: %s\n", err, installCertMgrCmd, res)

	filters := map[string]string{
		"namespace": "cert-manager",
	}
	Eventually(func(g Gomega) {
		pods, err := resources.GetPodsFiltered(filters)
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

func TestDeployRancher(cluster *driver.Cluster, flags *customflag.FlagConfig) {
	response := installRancher(cluster, flags)

	filters := map[string]string{
		"namespace": "cattle-system",
		"label":     "app=rancher",
	}

	Eventually(func(g Gomega) {
		pods, err := resources.GetPodsFiltered(filters)
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

		bootstrapPassCmd := strings.TrimSpace(line) + " --kubeconfig=" + resources.KubeConfigFile
		bootstrapPassword, err := resources.RunCommandHost(bootstrapPassCmd)
		Expect(err).NotTo(HaveOccurred(),
			"failed to retrieve rancher bootstrap password: %v\nCommand: %s\n", err, bootstrapPassCmd)

		rancherURL += bootstrapPassword

		break
	}
	resources.LogLevel("info", "\nRancher URL: %s", rancherURL)
}

func installRancher(cluster *driver.Cluster, flags *customflag.FlagConfig) string {
	err := addRepo(flags.Charts.RepoName, flags.Charts.RepoUrl)
	Expect(err).To(BeNil(), err)

	installRancherCmd := fmt.Sprintf(
		"kubectl create namespace cattle-system --kubeconfig=%s && "+
			"helm install rancher %s/rancher ",
		resources.KubeConfigFile,
		flags.Charts.RepoName)

	if flags.Charts.Args != "" {
		installRancherCmd += chartsArgsBuilder(flags)
	}

	installRancherCmd += fmt.Sprintf("-n cattle-system "+
		"--version=%s "+
		"--set global.cattle.psp.enabled=false "+
		"--set hostname=%s "+
		"--kubeconfig=%s",
		flags.Charts.Version,
		cluster.FQDN,
		resources.KubeConfigFile)

	resources.LogLevel("info", "Install command: %s", installRancherCmd)
	res, err := resources.RunCommandHost(installRancherCmd)
	Expect(err).NotTo(HaveOccurred(), "failed to deploy rancher via helm: %v\nCommand: %s\nResult: %s\n",
		err, installRancherCmd, res)

	return res
}

func chartsArgsBuilder(flags *customflag.FlagConfig) (finalArgs string) {
	helmArgs := flags.Charts.Args
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

func addRepo(name, url string) (err error) {
	resources.LogLevel("info", "Adding repo to helm - name: %v, url: %v", name, url)
	cmd := fmt.Sprintf(
		"helm repo add %s %s && helm repo update",
		name, url)
	res, err := resources.RunCommandHost(cmd)
	if err != nil {
		resources.LogLevel("error", "failed to add helm repo...\nCommand: %s\nResult: %s\n", cmd, res)
		return err
	}

	return nil
}
