package testcase

import (
	"strings"

	"github.com/rancher/distros-test-framework/pkg/assert"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/gomega"
)

const (
	statusRunning = "Running"
	nslookup      = "kubernetes.default.svc.cluster.local"
)

func TestIngress(deleteWorkload bool) {
	_, err := shared.ManageWorkload("apply", "ingress.yaml")
	Expect(err).NotTo(HaveOccurred(), "Ingress manifest not deployed")

	getIngressRunning := "kubectl get pods -n test-ingress -l k8s-app=nginx-app-ingress" +
		" --field-selector=status.phase=Running  --kubeconfig="
	err = assert.ValidateOnHost(getIngressRunning+shared.KubeConfigFile, statusRunning)
	Expect(err).NotTo(HaveOccurred(), err)

	ingressIps, err := shared.FetchIngressIP("test-ingress")
	Expect(err).NotTo(HaveOccurred(), "Ingress ip is not returned")

	for _, ip := range ingressIps {
		err = assert.CheckComponentCmdNode("curl -s --header host:foo1.bar.com"+
			" http://"+ip+"/name.html",
			ip,
			"test-ingress",
		)
	}
	Expect(err).NotTo(HaveOccurred(), err)

	if deleteWorkload {
		_, err := shared.ManageWorkload("delete", "ingress.yaml")
		Expect(err).NotTo(HaveOccurred(), "Ingress manifest not deleted")
	}
}

func TestDnsAccess(deleteWorkload bool) {
	_, err := shared.ManageWorkload("apply", "dnsutils.yaml")
	Expect(err).NotTo(HaveOccurred(), "dnsutils manifest not deployed")

	getPodDnsUtils := "kubectl get pods -n dnsutils dnsutils  --kubeconfig="
	err = assert.ValidateOnHost(getPodDnsUtils+shared.KubeConfigFile, statusRunning)
	Expect(err).NotTo(HaveOccurred(), err)

	execDnsUtils := "kubectl exec -n dnsutils -t dnsutils --kubeconfig="
	err = assert.CheckComponentCmdHost(
		execDnsUtils+shared.KubeConfigFile+" -- nslookup kubernetes.default",
		nslookup,
	)
	Expect(err).NotTo(HaveOccurred(), err)

	if deleteWorkload {
		_, err := shared.ManageWorkload("delete", "dnsutils.yaml")
		Expect(err).NotTo(HaveOccurred(), "dnsutils manifest not deleted")
	}
}

func TestIngressDualStack(testinfo map[string]string) {
	ingressIPs, err := shared.FetchIngressIP(testinfo["namespace"])
	Expect(err).NotTo(HaveOccurred(), "Ingress ip is not returned")

	for _, ingressIP := range ingressIPs {
		if strings.Contains(ingressIP,":") {
			ingressIP = shared.EncloseInBrackets(ingressIP)
		}
		err := assert.ValidateOnNode(shared.BastionIP,
			"curl -sL -H 'Host: test1.com' http://"+ ingressIP +"/name.html",
			testinfo["expected"])
		Expect(err).NotTo(HaveOccurred(), err)
	}
}
