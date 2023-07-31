package fixture

import (
	"github.com/rancher/distros-test-framework/lib/assert"
	"github.com/rancher/distros-test-framework/lib/shared"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var statusRunning = "Running"
var ExecDnsUtils = "kubectl exec -t dnsutils --kubeconfig="
var Nslookup = "kubernetes.default.svc.cluster.local"

func TestIngress(deployWorkload bool) {
	var ingressIps []string
	if deployWorkload {
		_, err := shared.ManageWorkload("create", "ingress.yaml")
		Expect(err).NotTo(HaveOccurred(), "Ingress manifest not deployed")
	}

	getIngressRunning := "kubectl get pods -l app=othertest  " +
		"--field-selector=status.phase=Running --kubeconfig="
	err := assert.ValidateOnHost(getIngressRunning+shared.KubeConfigFile, statusRunning)
	if err != nil {
		GinkgoT().Errorf("Error: %v", err)
	}

	nodes, err := shared.ParseNodes(false)
	Expect(err).NotTo(HaveOccurred())

	Eventually(func(Gomega) bool {
		ingressIps, err = shared.FetchIngressIP("default")
		if err != nil {
			return false
		}
		if len(ingressIps) != len(nodes) {
			return false
		}
		return true
	}, "420s", "5s").Should(BeTrue())

	for _, ip := range ingressIps {
		if assert.CheckComponentCmdHost("curl -s --header host:test1.com " +
			"http://" + ip + "/name.html", "othertest-deploy"); err != nil {
			return
		}
	}
}

func TestDnsAccess(deployWorkload bool) {
	if deployWorkload {
		_, err := shared.ManageWorkload("create", "dnsutils.yaml")
		Expect(err).NotTo(HaveOccurred(),
			"dnsutils manifest not deployed", err)
	}

	getDnsUtils := "kubectl get pods -n default dnsutils --kubeconfig="
	err := assert.ValidateOnHost(getDnsUtils+shared.KubeConfigFile, statusRunning)
	if err != nil {
		GinkgoT().Errorf("Error: %v", err)
	}

	assert.CheckComponentCmdHost(
		ExecDnsUtils+shared.KubeConfigFile+" -- nslookup kubernetes.default",
		Nslookup,
	)
}
