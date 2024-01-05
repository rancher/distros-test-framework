package testcase

import (
	"strings"

	"github.com/rancher/distros-test-framework/pkg/assert"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/gomega"
)

type TestData struct {
	Namespace string
	Label     string
	SVC       string
	Expected  string
}

func TestIngressDualStack(deleteWorkload bool) {
	cluster, err := FetchCluster()
	Expect(err).NotTo(HaveOccurred())

	err = shared.ManageWorkload("apply", "dualstack-ingress.yaml")
	Expect(err).NotTo(HaveOccurred())

	td := TestData{
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
		err = assert.ValidateOnNode(cluster.ExtraConfig.BastionIP,
			"curl -sL -H 'Host: test1.com' http://"+ingressIP+"/name.html",
			td.Expected)
		Expect(err).NotTo(HaveOccurred(), err)
	}

	if deleteWorkload {
		err = shared.ManageWorkload("delete", "dualstack-ingress.yaml")
		Expect(err).NotTo(HaveOccurred())
	}
}

func TestNodePort(deleteWorkload bool) {
	err := shared.ManageWorkload("apply", "dualstack-nodeport.yaml")
	Expect(err).NotTo(HaveOccurred())

	td := TestData{
		Namespace: "default",
		Label:     "app=dualstack-nodeport",
		SVC:       "dualstack-nodeport-svc",
		Expected:  "dualstack-nodeport-deployment",
	}

	assert.PodStatusRunning(td.Namespace, td.Label)
	TestServiceNodePortDualStack(td)

	if deleteWorkload {
		err = shared.ManageWorkload("delete", "dualstack-nodeport.yaml")
		Expect(err).NotTo(HaveOccurred())
	}
}

func TestClusterIPsInCIDRRange(deleteWorkload bool) {
	err := shared.ManageWorkload("apply", "dualstack-clusterip.yaml")
	Expect(err).NotTo(HaveOccurred())

	td := TestData{
		Namespace: "default",
		Label:     "app=clusterip-demo",
		SVC:       "clusterip-svc-demo",
	}

	assert.PodStatusRunning(td.Namespace, td.Label)
	TestIPsInCIDRRange(td.Label, td.SVC)

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
	td := TestData{
		Namespace: "default",
		Label:     "app=MyDualApp",
		SVC:       "httpd-deployment",
		Expected:  "It works!",
	}

	assert.PodStatusRunning(td.Namespace, td.Label)

	for i, svc := range services {
		td.SVC = "my-service-"
		td.SVC += svc
		TestServiceClusterIPs(td)
		assert.SVCSpecHasChars(
			td.Namespace, td.SVC, expectedIPFamily[i])
	}

	if deleteWorkload {
		err = shared.ManageWorkload("delete", "dualstack-multi.yaml")
		Expect(err).NotTo(HaveOccurred())
	}
}
