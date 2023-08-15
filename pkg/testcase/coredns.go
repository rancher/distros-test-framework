package testcase

import (
	"github.com/rancher/distros-test-framework/pkg/assert"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/gomega"
)

func TestCoredns(deleteWorkload bool) {
	_, err := shared.ManageWorkload("apply", "dnsutils.yaml")
	Expect(err).NotTo(HaveOccurred(),"dnsutils manifest not deployed")

	_, err = shared.AddHelmRepo("traefik", "https://helm.traefik.io/traefik")
	Expect(err).NotTo(HaveOccurred(), "failed to add Helm repo: %v", err)

	kubeconfigFlag := " --kubeconfig=" + shared.KubeConfigFile
	fullCmd := shared.JoinCommands("helm list --all-namespaces ", kubeconfigFlag)
	err = assert.CheckComponentCmdHost(
		fullCmd,
		"rke2-coredns-1.19.402",
	)
	Expect(err).NotTo(HaveOccurred(), err)

	err = assert.ValidateOnHost(ExecDnsUtils+shared.KubeConfigFile+
		" -- nslookup kubernetes.default", Nslookup)
	Expect(err).NotTo(HaveOccurred(), err)

	if deleteWorkload {
		_, err := shared.ManageWorkload("delete", "dnsutils.yaml")
		Expect(err).NotTo(HaveOccurred(),"dnsutils manifest not deleted")
	}
}
