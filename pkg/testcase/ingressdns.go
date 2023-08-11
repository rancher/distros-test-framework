package testcase

import (
	"github.com/rancher/distros-test-framework/pkg/assert"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	statusRunning = "Running"
	ExecDnsUtils = "kubectl exec -n auto-dns -t dnsutils --kubeconfig="
	Nslookup = "kubernetes.default.svc.cluster.local"
)

func TestIngress(delete bool) {
	_, err := shared.ManageWorkload("apply", arch, "ingress.yaml")
	Expect(err).NotTo(HaveOccurred(), "Ingress manifest not deployed")

	getIngressRunning := "kubectl get pods -n test-ingress -l k8s-app=nginx-app-ingress" +
		" --field-selector=status.phase=Running  --kubeconfig="
	err = assert.ValidateOnHost(getIngressRunning+shared.KubeConfigFile, statusRunning)
	if err != nil {
		GinkgoT().Errorf("%v", err)
	}

	ingressIps, err := shared.FetchIngressIP("test-ingress")
	Expect(err).NotTo(HaveOccurred(), "Ingress ip is not returned")

	for _, ip := range ingressIps {
		err = assert.CheckComponentCmdNode("curl -s --header host:foo1.bar.com"+
			" http://"+ip+"/name.html",
			"test-ingress",
			ip,
		)
	}
	if err != nil {
		GinkgoT().Errorf("%v", err)
	}

	if delete {
		_, err := shared.ManageWorkload("apply", arch, "ingress.yaml")
		Expect(err).NotTo(HaveOccurred(), "Ingress manifest not deleted")
	}
}

func TestDnsAccess(delete bool) {
	_, err := shared.ManageWorkload("apply", arch, "dnsutils.yaml")
	Expect(err).NotTo(HaveOccurred(), "dnsutils manifest not deployed")

	getPodDnsUtils := "kubectl get pods -n dnsutils dnsutils  --kubeconfig="
	err = assert.ValidateOnHost(getPodDnsUtils+shared.KubeConfigFile, statusRunning)
	if err != nil {
		GinkgoT().Errorf("%v", err)
	}

	execDnsUtils := "kubectl exec -n dnsutils -t dnsutils --kubeconfig="
	err = assert.CheckComponentCmdHost(
		execDnsUtils+shared.KubeConfigFile+" -- nslookup kubernetes.default",
		Nslookup,
	)
	if err != nil {
		GinkgoT().Errorf("%v", err)
	}

	if delete {
		_, err := shared.ManageWorkload("delete", arch, "dnsutils.yaml")
		Expect(err).NotTo(HaveOccurred(), "dnsutils manifest not deleted")
	}
}
