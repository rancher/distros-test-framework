//go:build upgradesuc

package upgradecluster

import (
	"fmt"

	"github.com/rancher/distros-test-framework/pkg/assert"
	"github.com/rancher/distros-test-framework/pkg/customflag"
	"github.com/rancher/distros-test-framework/pkg/testcase"

	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("SUC Upgrade Tests:", func() {

	It("Starts up with no issues", func() {
		testcase.TestBuildCluster(GinkgoT())
	})

	It("Validate Nodes", func() {
		testcase.TestNodeStatus(
			assert.NodeAssertReadyStatus(),
			nil,
		)
	})

	It("Validate Pods", func() {
		testcase.TestPodStatus(
			assert.PodAssertRestart(),
			assert.PodAssertReady(),
			assert.PodAssertStatus(),
		)
	})

	It("Verifies ClusterIP Service pre-upgrade", func() {
		testcase.TestServiceClusterIp(true, false)
	})

	It("Verifies NodePort Service pre-upgrade", func() {
		testcase.TestServiceNodePort(true, false)
	})

	It("Verifies Ingress pre-upgrade", func() {
		testcase.TestIngress(true, false)
	})

	It("Verifies Daemonset pre-upgrade", func() {
		testcase.TestDaemonset(true, false)
	})

	It("Verifies DNS Access pre-upgrade", func() {
		testcase.TestDnsAccess(true, false)
	})

	It("\nUpgrade via SUC", func() {
		_ = testcase.TestUpgradeClusterSUC(customflag.ServiceFlag.SUCUpgradeVersion.String())
	})

	It("Checks Node Status post-upgrade", func() {
		testcase.TestNodeStatus(
			assert.NodeAssertReadyStatus(),
			assert.NodeAssertVersionUpgraded(),
		)
	})

	It("Checks Pod Status post-upgrade", func() {
		testcase.TestPodStatus(
			nil,
			assert.PodAssertReady(),
			assert.PodAssertStatus(),
		)
	})

	It("Verifies ClusterIP Service post-upgrade", func() {
		testcase.TestServiceClusterIp(false, true)
	})

	It("Verifies NodePort Service post-upgrade", func() {
		testcase.TestServiceNodePort(false, true)
	})

	It("Verifies Ingress post-upgrade", func() {
		testcase.TestIngress(false, true)
	})

	It("Verifies Daemonset post-upgrade", func() {
		testcase.TestDaemonset(false, true)
	})

	It("Verifies DNS Access post-upgrade", func() {
		testcase.TestDnsAccess(false, true)
	})
})

var _ = AfterEach(func() {
	if CurrentSpecReport().Failed() {
		fmt.Printf("\nFAILED! %s\n", CurrentSpecReport().FullText())
	} else {
		fmt.Printf("\nPASSED! %s\n", CurrentSpecReport().FullText())
	}
})
