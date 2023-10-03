package testcase

import (
	"github.com/rancher/distros-test-framework/shared"
	"github.com/rancher/distros-test-framework/pkg/assert"

	. "github.com/onsi/gomega"
)

func TestDualStackIngress(deleteWorkload bool) {
	_, err := shared.ManageWorkload("apply", "dualstack-ingress.yaml")
	Expect(err).NotTo(HaveOccurred())
	
	testdata := map[string]string{
		"namespace":"default",
		"label":"app=dualstack-ing",
		"svc":"dualstack-ing-svc",
		"expected":"dualstack-ing-ds", 
	}
	
	assert.CheckPodStatusRunningByNSLabel(testdata["namespace"],testdata["label"])
	TestIngressDualStack(testdata)

	if deleteWorkload {
		_, err = shared.ManageWorkload("delete", "dualstack-ingress.yaml")
		Expect(err).NotTo(HaveOccurred())
	}
}

func TestDualStackNodePort(deleteWorkload bool) {
	_, err := shared.ManageWorkload("apply", "dualstack-nodeport.yaml")
	Expect(err).NotTo(HaveOccurred())

	testdata := map[string]string{
		"namespace":"default",
		"label":"app=dualstack-nodeport",
		"svc":"dualstack-nodeport-svc",
		"expected":"dualstack-nodeport-deployment",
	}
	
	assert.CheckPodStatusRunningByNSLabel(testdata["namespace"], testdata["label"])
	TestServiceNodePortDualStack(testdata)

	if deleteWorkload {
		_, err = shared.ManageWorkload("delete", "dualstack-nodeport.yaml")
		Expect(err).NotTo(HaveOccurred())	
	}
		
}

func TestDualStackClusterIPsInCIDRRange(deleteWorkload bool) {
	_,err := shared.ManageWorkload("apply","dualstack-clusterip.yaml")
	Expect(err).NotTo(HaveOccurred())

	testdata := map[string]string{
		"namespace":"default",
		"label":"app=clusterip-demo",
		"svc":"clusterip-svc-demo",
	}

	assert.CheckPodStatusRunningByNSLabel(testdata["namespace"], testdata["label"])
	TestIPsInCIDRRangeDualStack(testdata["label"], testdata["svc"])

	if deleteWorkload {
		_, err = shared.ManageWorkload("delete", "dualstack-clusterip.yaml")
		Expect(err).NotTo(HaveOccurred())
	}
	
}

func TestDualStackIPFamilies(deleteWorkload bool) {
	_,err := shared.ManageWorkload("apply","dualstack-multi.yaml")
	Expect(err).NotTo(HaveOccurred())

	services := []string{"v4", "v6", "require-dual", "prefer-dual"}
	expectedIPFamily := []string{
		`"ipFamilies":["IPv4"],"ipFamilyPolicy":"SingleStack"`,
		`"ipFamilies":["IPv6"],"ipFamilyPolicy":"SingleStack"`,
		`"ipFamilies":["IPv4","IPv6"],"ipFamilyPolicy":"RequireDualStack"`,
		`"ipFamilies":["IPv4","IPv6"],"ipFamilyPolicy":"PreferDualStack"`,
	}
	testdata := map[string]string{
		"namespace":"default",
		"label":"app=MyDualApp",
		"deployment":"httpd-deployment",
		"expected": "It works!",
	}

	assert.CheckPodStatusRunningByNSLabel(testdata["namespace"], testdata["label"])
	
	for i, svc := range services {
		testdata["svc"] = "my-service-"
		testdata["svc"] += svc
		TestServiceClusterIPDualStack(testdata)
		assert.CheckServiceSpecContainsSubstring(testdata["svc"], expectedIPFamily[i])
	}
	
	if deleteWorkload {
		_, err = shared.ManageWorkload("delete", "dualstack-multi.yaml")
		Expect(err).NotTo(HaveOccurred())
	}

}

