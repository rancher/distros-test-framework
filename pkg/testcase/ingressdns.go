package testcase

import (
	"fmt"
	"os"
	"strings"

	"github.com/rancher/distros-test-framework/factory"
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
	err := assert.ValidateOnHost(getIngressRunning+factory.KubeConfigFile, statusRunning)
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
	err := assert.ValidateOnHost(getPodDnsUtils+factory.KubeConfigFile, statusRunning)
	Expect(err).NotTo(HaveOccurred(), err)

	execDnsUtils := "kubectl exec -n dnsutils -t dnsutils --kubeconfig="
	err = assert.CheckComponentCmdHost(
		execDnsUtils+factory.KubeConfigFile+" -- nslookup kubernetes.default",
		nslookup,
	)
	Expect(err).NotTo(HaveOccurred(), err)

	if deleteWorkload {
		workloadErr = shared.ManageWorkload("delete", "dnsutils.yaml")
		Expect(workloadErr).NotTo(HaveOccurred(), "dnsutils manifest not deleted")
	}
}

func TestIngressRoute(cluster *factory.Cluster, applyWorkload, deleteWorkload bool, apiVersion string) {
	workerNodes, err := shared.GetNodesByRoles("worker")
	Expect(workerNodes).NotTo(BeEmpty())
	Expect(err).NotTo(HaveOccurred())
	publicIp := fmt.Sprintf("%s.nip.io", workerNodes[0].ExternalIP)

	if applyWorkload {
		// Update base IngressRoute manifest to use one of the Node External IPs
		originalFilePath := shared.BasePath() +
			fmt.Sprintf("/workloads/%s/ingressroute.yaml", cluster.Config.Arch)
		newFilePath := shared.BasePath() +
			fmt.Sprintf("/workloads/%s/dynamic-ingressroute.yaml", cluster.Config.Arch)
		content, errRead := os.ReadFile(originalFilePath)
		if errRead != nil {
			Expect(errRead).NotTo(HaveOccurred(), "failed to read file for ingressroute resource")
		}

		replacer := strings.NewReplacer("$YOURDNS", publicIp, "$APIVERSION", apiVersion)
		newContent := replacer.Replace(string(content))
		errWrite := os.WriteFile(newFilePath, []byte(newContent), 0644)
		if errWrite != nil {
			Expect(errWrite).NotTo(HaveOccurred(),
				"failed to update file for ingressroute resource to use one of the node external ips")
		}

		// Deploy manifest and ensure pods are running
		workloadErr := shared.ManageWorkload("apply", "dynamic-ingressroute.yaml")
		Expect(workloadErr).NotTo(HaveOccurred(), "IngressRoute manifest not successfully deployed")
	}

	validateIngressRoute(publicIp)

	if deleteWorkload {
		err = shared.ManageWorkload("delete", "dynamic-ingressroute.yaml")
		Expect(err).NotTo(HaveOccurred(), "IngressRoute manifest not successfully deleted")
	}
}

func validateIngressRoute(publicIp string) {
	getIngressRoutePodsRunning := fmt.Sprintf("kubectl get pods -n test-ingressroute -l app=whoami"+
		" --kubeconfig=%s", factory.KubeConfigFile)
	err := assert.ValidateOnHost(getIngressRoutePodsRunning, statusRunning)
	Expect(err).NotTo(HaveOccurred(), err)

	// Query the IngressRoute Host
	filters := map[string]string{
		"namespace": "test-ingressroute",
		"label":     "app=whoami",
	}

	var positiveAsserts []string
	negativeAsserts := "404 page not found"
	// retrying to get the node ip for the pods before running the tests.
	Eventually(func(_ Gomega) {
		pods, getErr := shared.GetPodsFiltered(filters)
		Expect(getErr).NotTo(HaveOccurred(), getErr)
		for _, pod := range pods {
			Expect(pod.IP).NotTo(Equal("<none>"))
			Expect(pod.IP).NotTo(BeEmpty())
			if pod.IP != "<none>" && pod.IP != "" {
				positiveAsserts = []string{
					fmt.Sprintf("Hostname: %s", pod.Name),
					fmt.Sprintf("IP: %s", pod.IP),
				}
			}
		}
	}, "40s", "5s").Should(Succeed())

	err = assert.CheckComponentCmdHost("curl -sk http://"+publicIp+"/notls", positiveAsserts...)
	Expect(err).NotTo(HaveOccurred(), err)

	err = assert.CheckComponentCmdHost("curl -sk https://"+publicIp+"/tls", positiveAsserts...)
	Expect(err).NotTo(HaveOccurred(), err)

	err = assert.CheckComponentCmdHost("curl -sk http://"+publicIp+"/tls", negativeAsserts)
	Expect(err).NotTo(HaveOccurred(), err)

	err = assert.CheckComponentCmdHost("curl -sk https://"+publicIp+"/notls", negativeAsserts)
	Expect(err).NotTo(HaveOccurred(), err)
}
