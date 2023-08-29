//go:build upgrademanual

package upgradecluster

import (
	"fmt"

	"github.com/rancher/distros-test-framework/pkg/assert"
	"github.com/rancher/distros-test-framework/pkg/customflag"
	"github.com/rancher/distros-test-framework/pkg/testcase"

	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Test:", func() {

	It("Start Up with no issues", func() {
		testcase.TestBuildCluster(GinkgoT())
	})

	It("Validate Node", func() {
		testcase.TestNodeStatus(
			assert.NodeAssertReadyStatus(),
			nil,
		)
	})

	It("Validate Pod", func() {
		testcase.TestPodStatus(
			assert.PodAssertRestart(),
			assert.PodAssertReady(),
			assert.PodAssertStatus(),
		)
	})

	It("Verifies ClusterIP Service", func() {
		testcase.TestServiceClusterIp(false)
	})

	It("Verifies NodePort Service", func() {
		testcase.TestServiceNodePort(false)
	})

	It("Verifies Ingress", func() {
		testcase.TestIngress(false)
	})

	It("Verifies Daemonset", func() {
		testcase.TestDaemonset(false)
	})

	It("Verifies dns access", func() {
		testcase.TestDnsAccess(false)
	})

	if cfg.Product == "k3s" {
		It("Verifies LoadBalancer Service", func() {
			testcase.TestServiceLoadBalancer(false)
		})

		It("Verifies Local Path Provisioner storage", func() {
			testcase.TestLocalPathProvisionerStorage(false)
		})
	}

	It("Upgrade Manual", func() {
		_ = testcase.TestUpgradeClusterManually(customflag.ServiceFlag.InstallMode.String())
	})

	It("Checks Node Status pos upgrade and validate version", func() {
		testcase.TestNodeStatus(
			assert.NodeAssertReadyStatus(),
			assert.NodeAssertVersionTypeUpgrade(customflag.ServiceFlag),
		)
	})

	It("Checks Pod Status pos upgrade", func() {
		testcase.TestPodStatus(
			assert.PodAssertRestart(),
			assert.PodAssertReady(),
			assert.PodAssertStatus(),
		)
	})

	It("Verifies ClusterIP Service after upgrade", func() {
		testcase.TestServiceClusterIp(true)
	})

	It("Verifies NodePort Service after upgrade", func() {
		testcase.TestServiceNodePort(true)
	})

	It("Verifies Ingress after upgrade", func() {
		testcase.TestIngress(true)
	})

	It("Verifies Daemonset after upgrade", func() {
		testcase.TestDaemonset(true)
	})

	It("Verifies dns access after upgrade", func() {
		testcase.TestDnsAccess(true)
	})

	if cfg.Product == "k3s" {
		It("Verifies LoadBalancer Service after upgrade", func() {
			testcase.TestServiceLoadBalancer(true)
		})

		It("Verifies Local Path Provisioner storage after upgrade", func() {
			testcase.TestLocalPathProvisionerStorage(true)
		})
	}
})

var _ = AfterEach(func() {
	if CurrentSpecReport().Failed() {
		fmt.Printf("\nFAILED! %s\n", CurrentSpecReport().FullText())
	} else {
		fmt.Printf("\nPASSED! %s\n", CurrentSpecReport().FullText())
	}
})
