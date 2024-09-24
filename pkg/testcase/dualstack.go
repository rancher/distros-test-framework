package testcase

import (
	"strings"

	"github.com/rancher/distros-test-framework/pkg/assert"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/gomega"
)

type testData struct {
	Namespace string
	Label     string
	SVC       string
	Expected  string
}

func TestIngressDualStack(cluster *shared.Cluster, deleteWorkload bool) {
	err := shared.ManageWorkload("apply", "dualstack-ingress.yaml")
	Expect(err).NotTo(HaveOccurred())

	td := testData{
		Namespace: "default",
		Label:     "app=dualstack-ing",
		SVC:       "dualstack-ing-svc",
		Expected:  "dualstack-ing-ds",
	}

	assert.PodStatusRunning(td.Namespace, td.Label)

	ingressIPs, err := shared.FetchIngressIP(td.Namespace)
	Expect(err).NotTo(HaveOccurred(), "Ingress ip is not returned")

	for _, ingressIP := range ingressIPs {
		if strings.Contains(ingressIP, ":") {
			ingressIP = shared.EncloseSqBraces(ingressIP)
		}
		err = assert.ValidateOnNode(cluster.BastionConfig.PublicIPv4Addr,
			"curl -sL -H 'Host: test1.com' http://"+ingressIP+"/name.html",
			td.Expected)
		Expect(err).NotTo(HaveOccurred(), err)
	}

	if deleteWorkload {
		err = shared.ManageWorkload("delete", "dualstack-ingress.yaml")
		Expect(err).NotTo(HaveOccurred())
	}
}

func TestNodePort(cluster *shared.Cluster, deleteWorkload bool) {
	err := shared.ManageWorkload("apply", "dualstack-nodeport.yaml")
	Expect(err).NotTo(HaveOccurred())

	td := testData{
		Namespace: "default",
		Label:     "app=dualstack-nodeport",
		SVC:       "dualstack-nodeport-svc",
		Expected:  "dualstack-nodeport-deployment",
	}

	assert.PodStatusRunning(td.Namespace, td.Label)
	testServiceNodePortDualStack(cluster, td)

	if deleteWorkload {
		err = shared.ManageWorkload("delete", "dualstack-nodeport.yaml")
		Expect(err).NotTo(HaveOccurred())
	}
}

func TestClusterIPsInCIDRRange(cluster *shared.Cluster, deleteWorkload bool) {
	err := shared.ManageWorkload("apply", "dualstack-clusterip.yaml")
	Expect(err).NotTo(HaveOccurred())

	td := testData{
		Namespace: "default",
		Label:     "app=clusterip-demo",
		SVC:       "clusterip-svc-demo",
	}

	assert.PodStatusRunning(td.Namespace, td.Label)
	testIPsInCIDRRange(cluster, td.Label, td.SVC)

	if deleteWorkload {
		err = shared.ManageWorkload("delete", "dualstack-clusterip.yaml")
		Expect(err).NotTo(HaveOccurred())
	}
}

func TestIPFamiliesDualStack(deleteWorkload bool) {
	err := shared.ManageWorkload("apply", "dualstack-multi.yaml")
	Expect(err).NotTo(HaveOccurred())

	services := []string{"v4", "v6", "require-dual", "prefer-dual"}
	expectedIPFamily := []string{
		`"ipFamilies":["IPv4"],"ipFamilyPolicy":"SingleStack"`,
		`"ipFamilies":["IPv6"],"ipFamilyPolicy":"SingleStack"`,
		`"ipFamilies":["IPv4","IPv6"],"ipFamilyPolicy":"RequireDualStack"`,
		`"ipFamilies":["IPv4","IPv6"],"ipFamilyPolicy":"PreferDualStack"`,
	}
	td := testData{
		Namespace: "default",
		Label:     "app=MyDualApp",
		SVC:       "httpd-deployment",
		Expected:  "It works!",
	}

	assert.PodStatusRunning(td.Namespace, td.Label)

	for i, svc := range services {
		td.SVC = "my-service-"
		td.SVC += svc
		testServiceClusterIPs(td)

		cmd := "kubectl get svc " + td.SVC + " -n " + td.Namespace +
			" -o jsonpath='{range .items[*]}{.spec}' --kubeconfig=" + shared.KubeConfigFile
		res, err2 := shared.RunCommandHost(cmd)
		Expect(err2).NotTo(HaveOccurred(), err2)
		Expect(res).To(ContainSubstring(expectedIPFamily[i]))
	}

	if deleteWorkload {
		err = shared.ManageWorkload("delete", "dualstack-multi.yaml")
		Expect(err).NotTo(HaveOccurred())
	}
}

func TestIngressWithPodRestartAndNetPol(cluster *shared.Cluster, deleteWorkload bool) {
	err := shared.ManageWorkload("apply", "k3s_issue_10053_ns.yaml",
		"k3s_issue_10053_pod1.yaml", "k3s_issue_10053_pod2.yaml")
	Expect(err).NotTo(HaveOccurred(), "failed to deploy initial manifests")

	var serverPodIP string
	filters := map[string]string{"namespace": "test-k3s-issue-10053"}

	Eventually(func(g Gomega) {
		pods, poderr := shared.GetPodsFiltered(filters)
		g.Expect(poderr).NotTo(HaveOccurred())
		g.Expect(pods).NotTo(BeEmpty())

		for i := range pods {
			processPodStatus(cluster, g, &pods[i], assert.PodAssertRestart(), assert.PodAssertReady())
			if pods[i].Name == "server" {
				serverPodIP = pods[i].IP
			}
		}
	}, "120s", "5s").Should(Succeed())

	assert.ValidateIntraNSPodConnectivity("test-k3s-issue-10053", "client", serverPodIP, "Hostname: server")

	// Deploy network policy that explicitly allows access to the server pod
	err = shared.ManageWorkload("apply", "k3s_issue_10053_netpol.yaml")
	Expect(err).NotTo(HaveOccurred(), "whoami pod failed to deploy")

	// Ensure connectivity from client pod to server pod BEFORE restarting the server
	assert.ValidateIntraNSPodConnectivity("test-k3s-issue-10053", "client", serverPodIP, "Hostname: server")

	// Redeploy server pod and ensure it is up and running again. Retrieve its new IP.
	err = shared.ManageWorkload("delete", "k3s_issue_10053_pod1.yaml")
	Expect(err).NotTo(HaveOccurred(), "whoami pod failed to delete")
	err = shared.ManageWorkload("apply", "k3s_issue_10053_pod1.yaml")
	Expect(err).NotTo(HaveOccurred(), "whoami pod failed to redeploy")

	Eventually(func(g Gomega) {
		pods, poderr := shared.GetPodsFiltered(filters)
		g.Expect(poderr).NotTo(HaveOccurred())
		g.Expect(pods).NotTo(BeEmpty())

		for i := range pods {
			processPodStatus(cluster, g, &pods[i], assert.PodAssertRestart(), assert.PodAssertReady())
			if pods[i].Name == "server" {
				serverPodIP = pods[i].IP
			}
		}
	}, "120s", "5s").Should(Succeed())

	// Ensure connectivity from client pod to server pod AFTER restarting the server
	assert.ValidateIntraNSPodConnectivity("test-k3s-issue-10053", "client", serverPodIP, "Hostname: server")

	if deleteWorkload {
		err = shared.ManageWorkload("delete", "k3s_issue_10053_ns.yaml")
		Expect(err).NotTo(HaveOccurred())
	}
}
