package testcase

import (
	"fmt"
	"os"
	"strings"

	"github.com/rancher/distros-test-framework/pkg/assert"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/gomega"
)

const (
	statusRunning = "Running"
	nslookup      = "kubernetes.default.svc.cluster.local"
)

func TestIngress(applyWorkload, deleteWorkload bool) {
	var workloadErr error
	if applyWorkload {
		workloadErr = shared.ManageWorkload("apply", "ingress.yaml")
		Expect(workloadErr).NotTo(HaveOccurred(), "ingress manifest not deployed")
	}

	getIngressRunning := "kubectl get pods -n test-ingress -l k8s-app=nginx-app-ingress" +
		" --field-selector=status.phase=Running  --kubeconfig="
	err := assert.ValidateOnHost(getIngressRunning+shared.KubeConfigFile, statusRunning)
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
		workloadErr = shared.ManageWorkload("delete", "ingress.yaml")
		Expect(workloadErr).NotTo(HaveOccurred(), "Ingress manifest not deleted")
	}
}

func TestDnsAccess(applyWorkload, deleteWorkload bool) {
	var workloadErr error
	if applyWorkload {
		workloadErr = shared.ManageWorkload("apply", "dnsutils.yaml")
		Expect(workloadErr).NotTo(HaveOccurred(), "dnsutils manifest not deployed")
	}

	getPodDnsUtils := "kubectl get pods -n dnsutils dnsutils  --kubeconfig="
	err := assert.ValidateOnHost(getPodDnsUtils+shared.KubeConfigFile, statusRunning)
	Expect(err).NotTo(HaveOccurred(), err)

	execDnsUtils := "kubectl exec -n dnsutils -t dnsutils --kubeconfig="
	err = assert.CheckComponentCmdHost(
		execDnsUtils+shared.KubeConfigFile+" -- nslookup kubernetes.default",
		nslookup,
	)
	Expect(err).NotTo(HaveOccurred(), err)

	if deleteWorkload {
		workloadErr = shared.ManageWorkload("delete", "dnsutils.yaml")
		Expect(workloadErr).NotTo(HaveOccurred(), "dnsutils manifest not deleted")
	}
}

func TestIngressRoute(applyWorkload, deleteWorkload bool) {
	publicIp := fmt.Sprintf("%s.nip.io", shared.FetchNodeExternalIP()[0])
	if applyWorkload {
		// Update base IngressRoute manifest to use one of the Node External IPs
		originalFilePath := shared.BasePath() +
		fmt.Sprintf("/distros-test-framework/workloads/%s/ingressroute.yaml", shared.Arch)
		newFilePath := shared.BasePath() + 
			fmt.Sprintf("/distros-test-framework/workloads/%s/dynamic-ingressroute.yaml", shared.Arch)
		content, err := os.ReadFile(originalFilePath)
		if err != nil {
			Expect(err).NotTo(HaveOccurred(), "failed to read file for ingressroute resource")
		}

		newContent := strings.ReplaceAll(string(content), "$YOURDNS", publicIp)
		err = os.WriteFile(newFilePath, []byte(newContent), 0644)
		if err != nil {
			Expect(err).NotTo(HaveOccurred(), "failed to update file for ingressroute resource to use one of the node external ips")
		}

		// Deploy manifest and ensure pods are running
		var workloadErr error
		err = shared.ManageWorkload("apply", "dynamic-ingressroute.yaml")
		Expect(workloadErr).NotTo(HaveOccurred(), "IngressRoute manifest not successfully deployed")
	}

	getIngressRoutePodsRunning := fmt.Sprintf("kubectl get pods -n test-ingressroute -l app=whoami" +
		" --field-selector=status.phase=Running --kubeconfig=%s", shared.KubeConfigFile)
	err := assert.ValidateOnHost(getIngressRoutePodsRunning, statusRunning)
	Expect(err).NotTo(HaveOccurred(), err)

	// Query the IngressRoute Host
	pods, err := shared.GetPodsByNamespaceAndLabel("test-ingressroute", "app=whoami", false)
	Expect(err).NotTo(HaveOccurred(), err)
	
	negativeAsserts := "404 page not found"
	for _, pod := range pods {
		positiveAsserts := []string{
			fmt.Sprintf("Hostname: %s", pod.Name),
			fmt.Sprintf("IP: %s", pod.NodeIP),
		}
		Eventually(func(g Gomega) {
			// Positive test cases
			err = assert.CheckComponentCmdHost("curl -sk http://"+publicIp+"/notls", positiveAsserts...)
			Expect(err).NotTo(HaveOccurred(), err)

			err = assert.CheckComponentCmdHost("curl -sk https://"+publicIp+"/tls", positiveAsserts...)
			Expect(err).NotTo(HaveOccurred(), err)

			// Negative test cases
			err = assert.CheckComponentCmdHost("curl -sk http://"+publicIp+"/tls", negativeAsserts)
			Expect(err).NotTo(HaveOccurred(), err)

			err = assert.CheckComponentCmdHost("curl -sk https://"+publicIp+"/notls", negativeAsserts)
			Expect(err).NotTo(HaveOccurred(), err)
		}, "30s", "5s").Should(Succeed())
	}

	if deleteWorkload {
		err = shared.ManageWorkload("delete", "dynamic-ingressroute.yaml")
		Expect(err).NotTo(HaveOccurred(), "IngressRoute manifest not successfully deleted")
	}
}
