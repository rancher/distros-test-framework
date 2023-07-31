package upgradecluster

import (
	"fmt"

	"github.com/rancher/distros-test-framework/lib/assert"
	"github.com/rancher/distros-test-framework/cmd"
	"github.com/rancher/distros-test-framework/component/fixture"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Test:", func() {

	It("Start Up with no issues", func() {
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

	It("Upgrade Manual", func() {
		err := fixture.TestUpgradeClusterManually(
			cmd.ServiceFlag.ClusterConfig.Product.String(), 
			cmd.ServiceFlag.InstallUpgrade.String())
		Expect(err).NotTo(HaveOccurred())
	})

	It("Checks Node Status pos upgrade and validate version", func() {
		fixture.TestNodeStatus(
			assert.NodeAssertReadyStatus(),
			assert.NodeAssertVersionTypeUpgrade(cmd.ServiceFlag, cmd.ServiceFlag.ClusterConfig.Product.String()),
			cmd.ServiceFlag.ClusterConfig.Product.String(),
		)
	})

	It("Checks Pod Status pos upgrade", func() {
		fixture.TestPodStatus(
			assert.PodAssertRestart(),
			assert.PodAssertReady(),
			assert.PodAssertStatus(),
		)
	})

	It("Verifies ClusterIP Service after upgrade", func() {
		fixture.TestServiceClusterIp(false)
	})

	It("Verifies NodePort Service after upgrade", func() {
		fixture.TestServiceNodePort(false)
	})

	It("Verifies Ingress after upgrade", func() {
		fixture.TestIngress(false)
	})

	It("Verifies Daemonset after upgrade", func() {
		fixture.TestDaemonset(false)
	})

	It("Verifies LoadBalancer Service after upgrade", func() {
		fixture.TestServiceLoadBalancer(false)
	})

	It("Verifies Local Path Provisioner storage after upgrade", func() {
		fixture.TestLocalPathProvisionerStorage(false)
	})

	It("Verifies dns access after upgrade", func() {
		fixture.TestDnsAccess(false)
	})
})

var _ = AfterEach(func() {
	if CurrentSpecReport().Failed() {
		fmt.Printf("\nFAILED! %s\n", CurrentSpecReport().FullText())
	} else {
		fmt.Printf("\nPASSED! %s\n", CurrentSpecReport().FullText())
	}
})
