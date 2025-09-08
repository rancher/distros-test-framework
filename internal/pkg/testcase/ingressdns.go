package testcase

import (
	"fmt"
	"os"
	"strings"

	"github.com/rancher/distros-test-framework/internal/pkg/assert"
	"github.com/rancher/distros-test-framework/internal/provisioning/driver"
	"github.com/rancher/distros-test-framework/internal/resources"

	. "github.com/onsi/gomega"
)

const (
	statusRunning = "Running"
	nslookup      = "kubernetes.default.svc.cluster.local"
)

func TestIngress(applyWorkload, deleteWorkload bool) {
	var workloadErr error
	if applyWorkload {
		workloadErr = resources.ManageWorkload("apply", "ingress.yaml")
		Expect(workloadErr).NotTo(HaveOccurred(), "ingress manifest not deployed")
	}

	getIngressRunning := "kubectl get pods -n test-ingress -l k8s-app=nginx-app-ingress" +
		" --field-selector=status.phase=Running  --kubeconfig="
	err := assert.ValidateOnHost(getIngressRunning+resources.KubeConfigFile, statusRunning)
	Expect(err).NotTo(HaveOccurred(), err)

	ingressIps, err := resources.FetchIngressIP("test-ingress")
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
		workloadErr = resources.ManageWorkload("delete", "ingress.yaml")
		Expect(workloadErr).NotTo(HaveOccurred(), "Ingress manifest not deleted")
	}
}

func TestDNSAccess(applyWorkload, deleteWorkload bool) {
	var workloadErr error
	if applyWorkload {
		workloadErr = resources.ManageWorkload("apply", "dnsutils.yaml")
		Expect(workloadErr).NotTo(HaveOccurred(), "dnsutils manifest not deployed")
	}

	getPodDnsUtils := "kubectl get pods -n dnsutils dnsutils  --kubeconfig="
	err := assert.ValidateOnHost(getPodDnsUtils+resources.KubeConfigFile, statusRunning)
	Expect(err).NotTo(HaveOccurred(), err)

	execDNSUtils := "kubectl exec -n dnsutils -t dnsutils --kubeconfig="
	err = assert.CheckComponentCmdHost(
		execDNSUtils+resources.KubeConfigFile+" -- nslookup kubernetes.default",
		nslookup,
	)
	Expect(err).NotTo(HaveOccurred(), err)

	if deleteWorkload {
		workloadErr = resources.ManageWorkload("delete", "dnsutils.yaml")
		Expect(workloadErr).NotTo(HaveOccurred(), "dnsutils manifest not deleted")
	}
}

func TestIngressRoute(cluster *driver.Cluster, applyWorkload, deleteWorkload bool, apiVersion string) {
	// TODO: Remove when v1.32 in minimum supported version
	_, version, _ := resources.Product()
	workloadFile := "dynamic-ingressroute.yaml"
	if !resources.SliceContainsString([]string{"1.29", "1.30", "1.31"}, version) &&
		(apiVersion == "traefik.containo.us/v1alpha1") {
		resources.LogLevel("info", "\n%v\nAbove version is not supported for apiVersion: %v", version, apiVersion)
		return
	}

	workerNodes, err := resources.GetNodesByRoles("worker")
	Expect(workerNodes).NotTo(BeEmpty())
	Expect(err).NotTo(HaveOccurred())
	publicIp := workerNodes[0].ExternalIP + ".nip.io"

	if applyWorkload {
		// Update base IngressRoute manifest to use one of the Node External IPs.
		originalFilePath := resources.BasePath() +
			fmt.Sprintf("/workloads/%s/ingressroute.yaml", cluster.Config.Arch)
		newFilePath := resources.BasePath() +
			fmt.Sprintf("/workloads/%s/%s", cluster.Config.Arch, workloadFile)
		content, errRead := os.ReadFile(originalFilePath)
		if errRead != nil {
			Expect(errRead).NotTo(HaveOccurred(), "failed to read file for ingressroute resource")
		}

		replacer := strings.NewReplacer("$YOURDNS", publicIp, "$APIVERSION", apiVersion)
		newContent := replacer.Replace(string(content))
		errWrite := os.WriteFile(newFilePath, []byte(newContent), 0o644)
		if errWrite != nil {
			Expect(errWrite).NotTo(HaveOccurred(),
				"failed to update file for ingressroute resource to use one of the node external ips")
		}

		// Deploy manifest and ensure pods are running.
		resources.LogLevel("debug", "Applying workload: %s", workloadFile)
		workloadErr := resources.ManageWorkload("apply", workloadFile)
		Expect(workloadErr).NotTo(HaveOccurred(), "IngressRoute manifest not successfully deployed")
	}

	validateIngressRoute("test-ingressroute", "app=whoami", publicIp)

	if deleteWorkload {
		resources.LogLevel("debug", "Deleting workload: %s", workloadFile)
		err = resources.ManageWorkload("delete", workloadFile)
		Expect(err).NotTo(HaveOccurred(), "IngressRoute manifest not successfully deleted")
	}
}

func validateIngressRoute(namespace, label, publicIP string) {
	getIngressRoutePodsRunning := fmt.Sprintf("kubectl get pods -n %s -l %s"+
		" --kubeconfig=%s", namespace, label, resources.KubeConfigFile)
	err := assert.ValidateOnHost(getIngressRoutePodsRunning, statusRunning)
	Expect(err).NotTo(HaveOccurred(), err)

	// Query the IngressRoute Host.
	filters := map[string]string{
		"namespace": namespace,
		"label":     label,
	}

	var positiveAsserts []string
	negativeAsserts := "404 page not found"
	negativeAsserts = resources.CleanString(negativeAsserts)

	// retrying to get the node ip for the pods before running the tests.
	Eventually(func(g Gomega) {
		pods, getErr := resources.GetPodsFiltered(filters)
		g.Expect(getErr).NotTo(HaveOccurred(), getErr)
		for i := range pods {
			g.Expect(pods[i].IP).NotTo(Equal("<none>"))
			g.Expect(pods[i].IP).NotTo(BeEmpty())
			if pods[i].IP != "<none>" && pods[i].IP != "" {
				positiveAsserts = []string{
					"Hostname: " + pods[i].Name,
					"IP: " + pods[i].IP,
				}
				positiveAsserts = resources.CleanSliceStrings(positiveAsserts)
			}
		}
	}, "40s", "5s").Should(Succeed())

	err = assert.CheckComponentCmdHost("curl -sk http://"+publicIP+"/notls", positiveAsserts...)
	Expect(err).NotTo(HaveOccurred(), err)

	err = assert.CheckComponentCmdHost("curl -sk https://"+publicIP+"/tls", positiveAsserts...)
	Expect(err).NotTo(HaveOccurred(), err)

	err = assert.CheckComponentCmdHost("curl -sk http://"+publicIP+"/tls", negativeAsserts)
	Expect(err).NotTo(HaveOccurred(), err)

	err = assert.CheckComponentCmdHost("curl -sk https://"+publicIP+"/notls", negativeAsserts)
	Expect(err).NotTo(HaveOccurred(), err)
}
