package testcase

import (
	"github.com/rancher/distros-test-framework/core/service/assert"
	"github.com/rancher/distros-test-framework/core/service/customflag"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var Running = "Running"
var ExecDnsUtils = "kubectl exec -n auto-dns -t dnsutils --kubeconfig="
var Nslookup = "kubernetes.default.svc.cluster.local"

func TestIngress(deployWorkload bool) {
	if deployWorkload {
		_, err := shared.ManageWorkload(
			"create",
			"ingress.yaml",
			customflag.ServiceFlag.ClusterConfig.Arch.String(),
		)
		Expect(err).NotTo(HaveOccurred(),
			"Ingress manifest not deployed")
	}

	getIngressRunning := "kubectl get pods  -l k8s-app=nginx-app-ingress" +
		" --field-selector=status.phase=Running  --kubeconfig="
	err := assert.ValidateOnHost(getIngressRunning+shared.KubeConfigFile, Running)
	if err != nil {
		GinkgoT().Errorf("%v", err)
	}

	ingressIps, err := shared.FetchIngressIP()
	Expect(err).NotTo(HaveOccurred(), "Ingress ip is not returned")

	for _, ip := range ingressIps {
		assert.CheckComponentCmdNode("curl -s --header host:foo1.bar.com"+
			" http://"+ip+"/name.html", "test-ingress", ip)
	}
}

func TestDnsAccess(deployWorkload bool) {
	if deployWorkload {
		_, err := shared.ManageWorkload(
			"create",
			"dnsutils.yaml",
			customflag.ServiceFlag.ClusterConfig.Arch.String(),
		)
		Expect(err).NotTo(HaveOccurred(),
			"dnsutils manifest not deployed")
	}

	getPodDnsUtils := "kubectl get pods dnsutils --kubeconfig="
	err := assert.ValidateOnHost(getPodDnsUtils+shared.KubeConfigFile, Running)
	if err != nil {
		GinkgoT().Errorf("%v", err)
	}

	execDnsUtils := "kubectl exec -t dnsutils --kubeconfig="
	assert.CheckComponentCmdHost(
		execDnsUtils+shared.KubeConfigFile+" -- nslookup kubernetes.default",
		"kubernetes.default.svc.cluster.local",
	)
}
