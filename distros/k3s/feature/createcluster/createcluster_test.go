package createcluster

import (
	"fmt"

	"github.com/rancher/distros-test-framework/cmd"
	"github.com/rancher/distros-test-framework/lib/assert"
	"github.com/rancher/distros-test-framework/component/fixture"

	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Test:", func() {

	FIt("Start Up with no issues", func() {
		fmt.Printf("Product: ", cmd.ServiceFlag.ClusterConfig.Product.String())
		fixture.TestBuildCluster(GinkgoT(), cmd.ServiceFlag.ClusterConfig.Product.String())
	})

	It("Validate Node", func() {
		fixture.TestNodeStatus(
			assert.NodeAssertReadyStatus(),
			nil,
			cmd.ServiceFlag.ClusterConfig.Product.String(),
		)
	})

	It("Validate Pod", func() {
		fixture.TestPodStatus(
			assert.PodAssertRestart(),
			assert.PodAssertReady(),
			assert.PodAssertStatus(),
		)
	})

	It("Verifies ClusterIP Service", func() {
		fixture.TestServiceClusterIp(true)
	})

	It("Verifies NodePort Service", func() {
		fixture.TestServiceNodePort(true)
	})

	It("Verifies LoadBalancer Service", func() {
		fixture.TestServiceLoadBalancer(true)
	})

	It("Verifies Ingress", func() {
		fixture.TestIngress(true)
	})

	It("Verifies Daemonset", func() {
		fixture.TestDaemonset(true)
	})

	It("Verifies Local Path Provisioner storage", func() {
		fixture.TestLocalPathProvisionerStorage(true)
	})

	It("Verifies dns access", func() {
		fixture.TestDnsAccess(true)
	})
})

var _ = AfterEach(func() {
	if CurrentSpecReport().Failed() {
		fmt.Printf("\nFAILED! %s\n", CurrentSpecReport().FullText())
	} else {
		fmt.Printf("\nPASSED! %s\n", CurrentSpecReport().FullText())
	}
})
