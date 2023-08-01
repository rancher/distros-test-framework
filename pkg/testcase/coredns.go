package testcase

import (
	"log"

	"github.com/rancher/distros-test-framework/pkg/assert"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestCoredns(deployWorkload bool) {
	if deployWorkload {
		_, err := shared.ManageWorkload("create", "dnsutils.yaml", "arch")
		Expect(err).NotTo(HaveOccurred(),
			"dnsutils manifest not deployed", err)
	}

	_, err := shared.AddHelmRepo("traefik", "https://helm.traefik.io/traefik")
	if err != nil {
		log.Fatalf("failed to add Helm repo: %v", err)
	}

	kubeconfigFlag := " --kubeconfig=" + shared.KubeConfigFile
	fullCmd := shared.JoinCommands("helm list --all-namespaces ", kubeconfigFlag)
	err = assert.CheckComponentCmdHost(
		fullCmd,
		"rke2-coredns-1.19.402",
	)
	if err != nil {
		GinkgoT().Errorf("%v", err)
	}

	err = assert.ValidateOnHost(ExecDnsUtils+shared.KubeConfigFile+
		" -- nslookup kubernetes.default", Nslookup)
	if err != nil {
		return
	}
}
