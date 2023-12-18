package testcase

import (
	"github.com/rancher/distros-test-framework/pkg/assert"
	"github.com/rancher/distros-test-framework/shared"
	"strings"

	. "github.com/onsi/gomega"
)

type TestData struct {
	Namespace string
	Label     string
	SVC       string
	Expected  string
}

func TestIngressDualStack(deleteWorkload bool) {
	err := shared.ManageWorkload("apply", "dualstack-ingress.yaml")
	Expect(err).NotTo(HaveOccurred())

	testdata := TestData{
		Namespace: "default",
		Label:     "app=dualstack-ing",
		SVC:       "dualstack-ing-svc",
		Expected:  "dualstack-ing-ds",
	}

	assert.ValidatePodIsRunning(testdata.Namespace, testdata.Label)

	ingressIPs, err := shared.FetchIngressIP(testdata.Namespace)
	Expect(err).NotTo(HaveOccurred(), "Ingress ip is not returned")

	for _, ingressIP := range ingressIPs {
		if strings.Contains(ingressIP, ":") {
			ingressIP = shared.EncloseSqBraces(ingressIP)
		}
		err := assert.ValidateOnNode(shared.BastionIP,
			"curl -sL -H 'Host: test1.com' http://"+ingressIP+"/name.html",
			testdata.Expected)
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

	testdata := TestData{
		Namespace: "default",
		Label:     "app=dualstack-nodeport",
		SVC:       "dualstack-nodeport-svc",
		Expected:  "dualstack-nodeport-deployment",
	}

	assert.ValidatePodIsRunning(testdata.Namespace, testdata.Label)
	TestServiceNodePortDualStack(testdata)

	if deleteWorkload {
		err = shared.ManageWorkload("delete", "dualstack-nodeport.yaml")
		Expect(err).NotTo(HaveOccurred())
	}
}

func TestClusterIPsInCIDRRange(deleteWorkload bool) {
	err := shared.ManageWorkload("apply", "dualstack-clusterip.yaml")
	Expect(err).NotTo(HaveOccurred())

	testdata := TestData{
		Namespace: "default",
		Label:     "app=clusterip-demo",
		SVC:       "clusterip-svc-demo",
	}

	assert.ValidatePodIsRunning(testdata.Namespace, testdata.Label)
	TestIPsInCIDRRange(testdata.Label, testdata.SVC)

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
	testdata := TestData{
		Namespace: "default",
		Label:     "app=MyDualApp",
		SVC:       "httpd-deployment",
		Expected:  "It works!",
	}

	assert.ValidatePodIsRunning(testdata.Namespace, testdata.Label)

	for i, svc := range services {
		testdata.SVC = "my-service-"
		testdata.SVC += svc
		TestServiceClusterIPs(testdata)
		assert.ValidateSVCSpecHasChars(
			testdata.Namespace, testdata.SVC, expectedIPFamily[i])
	}

	if deleteWorkload {
		err = shared.ManageWorkload("delete", "dualstack-multi.yaml")
		Expect(err).NotTo(HaveOccurred())
	}
}
