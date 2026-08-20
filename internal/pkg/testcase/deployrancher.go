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
	Expect(err).To(BeNil())

	crdsRes, err := resources.RunHostArgs("kubectl", "apply",
		"--kubeconfig="+resources.KubeConfigFile, "--validate=false",
		"-f", "https://github.com/jetstack/cert-manager/releases/download/"+version+"/cert-manager.crds.yaml")
	Expect(err).NotTo(HaveOccurred(), "failed to apply cert-manager CRDs: %v\nResult: %s\n", err, crdsRes)

	nsRes, err := resources.RunHostArgs("kubectl", "create", "namespace", "cert-manager",
		"--kubeconfig="+resources.KubeConfigFile)
	Expect(err).NotTo(HaveOccurred(), "failed to create cert-manager namespace: %v\nResult: %s\n", err, nsRes)

	res, err := resources.RunHostArgs("helm", "install", "cert-manager", "jetstack/cert-manager",
		"-n", "cert-manager", "--version", version, "--kubeconfig="+resources.KubeConfigFile)
	Expect(err).NotTo(HaveOccurred(),
		"failed to deploy cert-manager via helm: %v\nResult: %s\n", err, res)

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
	Expect(err).To(BeNil())

	nsRes, err := resources.RunHostArgs("kubectl", "create", "namespace", "cattle-system",
		"--kubeconfig="+resources.KubeConfigFile)
	Expect(err).NotTo(HaveOccurred(), "failed to create cattle-system namespace: %v\nResult: %s\n", err, nsRes)

	installArgs := []string{"install", "rancher", flags.Charts.RepoName + "/rancher"}
	installArgs = append(installArgs, chartsArgsList(flags)...)
	installArgs = append(installArgs,
		"-n", "cattle-system",
		"--version="+flags.Charts.Version,
		"--set", "global.cattle.psp.enabled=false",
		"--set", "hostname="+cluster.FQDN,
		"--kubeconfig="+resources.KubeConfigFile)

	resources.LogLevel("info", "Install command: helm %v", installArgs)
	res, err := resources.RunHostArgs("helm", installArgs...)
	Expect(err).NotTo(HaveOccurred(), "failed to deploy rancher via helm: %v\nCommand: helm %v\nResult: %s\n",
		err, installArgs, res)

	return res
}

// chartsArgsList turns the comma-separated chartsArgs flag into deduplicated
// `--set key=value` argument pairs.
func chartsArgsList(flags *customflag.FlagConfig) []string {
	if flags.Charts.Args == "" {
		return nil
	}

	var out []string
	seen := make(map[string]bool)
	for _, arg := range strings.Split(flags.Charts.Args, ",") {
		arg = strings.TrimSpace(arg)
		if arg == "" || seen[arg] {
			continue
		}
		seen[arg] = true
		out = append(out, "--set", arg)
	}

	return out
}

func addRepo(name, url string) (err error) {
	resources.LogLevel("info", "Adding repo to helm - name: %v, url: %v", name, url)
	if _, err = resources.RunHostArgs("helm", "repo", "add", "--", name, url); err != nil {
		return err
	}
	cmd := "helm repo update"
	res, err := resources.RunCommandHost(cmd)
	if err != nil {
		resources.LogLevel("error", "failed to add helm repo...\nCommand: %s\nResult: %s\n", cmd, res)
		return err
	}

	return nil
}
