package testcase

import (
	"fmt"
	"strings"

	"github.com/rancher/distros-test-framework/pkg/assert"
	"github.com/rancher/distros-test-framework/pkg/customflag"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/gomega"
)

func TestDeployCertManager(cluster *shared.Cluster, version string) {
	err := addRepo("jetstack", "https://charts.jetstack.io")
	Expect(err).To(BeNil(), err)

	applyCrdsCmd := fmt.Sprintf(
		"kubectl apply --kubeconfig=%s --validate=false -f "+
			"https://github.com/jetstack/cert-manager/releases/download/%s/cert-manager.crds.yaml",
		shared.KubeConfigFile, version)
	installCertMgrCmd := fmt.Sprintf("kubectl create namespace cert-manager --kubeconfig=%s && ",
		shared.KubeConfigFile) + fmt.Sprintf(
		"helm install cert-manager jetstack/cert-manager -n cert-manager --version %s --kubeconfig=%s",
		version, shared.KubeConfigFile)

	res, err := shared.RunCommandHost(applyCrdsCmd, installCertMgrCmd)
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

func TestDeployRancher(cluster *shared.Cluster, flags *customflag.FlagConfig) {
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

		bootstrapPassCmd := strings.TrimSpace(line) + " --kubeconfig=" + shared.KubeConfigFile
		bootstrapPassword, err := shared.RunCommandHost(bootstrapPassCmd)
		Expect(err).NotTo(HaveOccurred(),
			"failed to retrieve rancher bootstrap password: %v\nCommand: %s\n", err, bootstrapPassCmd)

		rancherURL += bootstrapPassword

		break
	}
	shared.LogLevel("info", "\nRancher URL: %s", rancherURL)
}

func installRancher(cluster *shared.Cluster, flags *customflag.FlagConfig) string {
	err := addRepo(flags.Charts.RepoName, flags.Charts.RepoUrl)
	Expect(err).To(BeNil(), err)

	if flags.Rancher.RepoUrl != "" &&
		flags.Rancher.RepoUrl != flags.Charts.RepoUrl {
		err = addRepo(flags.Rancher.RepoName, flags.Rancher.RepoUrl)
		Expect(err).To(BeNil(), err)
	} else {
		flags.Rancher.RepoName = flags.Charts.RepoName
		flags.Rancher.RepoUrl = flags.Charts.RepoUrl
	}

	installRancherCmd := fmt.Sprintf(
		"kubectl create namespace cattle-system --kubeconfig=%s && "+
			"helm install rancher %s/rancher ",
		shared.KubeConfigFile,
		flags.Rancher.RepoName)

	if flags.Charts.Args != "" {
		installRancherCmd += chartsArgsBuilder(flags)
	}

	installRancherCmd += fmt.Sprintf("-n cattle-system "+
		"--version=%s "+
		"--set global.cattle.psp.enabled=false "+
		"--set hostname=%s "+
		"--kubeconfig=%s",
		flags.Rancher.Version,
		cluster.FQDN,
		shared.KubeConfigFile)

	shared.LogLevel("info", "Install command: %s", installRancherCmd)
	res, err := shared.RunCommandHost(installRancherCmd)
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
	shared.LogLevel("info", "Adding repo to helm - name: %v, url: %v", name, url)
	cmd := fmt.Sprintf(
		"helm repo add %s %s && helm repo update",
		name, url)
	res, err := shared.RunCommandHost(cmd)
	if err != nil {
		shared.LogLevel("error", "failed to add helm repo...\nCommand: %s\nResult: %s\n", cmd, res)
		return err
	}

	return nil
}
