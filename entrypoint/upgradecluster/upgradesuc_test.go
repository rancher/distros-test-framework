//go:build upgradesuc

package upgradecluster

import (
	"fmt"

	"github.com/rancher/distros-test-framework/pkg/assert"
	"github.com/rancher/distros-test-framework/pkg/customflag"
	"github.com/rancher/distros-test-framework/pkg/testcase"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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

	It("Verifies ClusterIP Service pre upgrade", func() {
		testcase.TestServiceClusterIp(true)
	})

	It("Verifies NodePort Service pre upgrade", func() {
		testcase.TestServiceNodePort(true)
	})

	It("Verifies Ingress pre upgrade", func() {
		testcase.TestIngress(true)
	})

	It("Verifies Daemonset pre upgrade", func() {
		testcase.TestDaemonset(true)
	})

	It("Verifies DNS Access pre upgrade", func() {
		testcase.TestDnsAccess(true)
	})

	It("\nUpgrade via SUC", func() {
		err := testcase.TestUpgradeClusterSUC(customflag.ServiceFlag.UpgradeVersionSUC.String())
		Expect(err).NotTo(HaveOccurred())
	})

	It("Checks Node Status pos upgrade suc", func() {
		testcase.TestNodeStatus(
			assert.NodeAssertReadyStatus(),
			assert.NodeAssertVersionUpgraded(),
		)
	})

	It("Checks Pod Status pos upgrade suc", func() {
		testcase.TestPodStatus(
			nil,
			assert.PodAssertReady(),
			assert.PodAssertStatus(),
		)
	})

	It("Verifies ClusterIP Service pos upgrade", func() {
		testcase.TestServiceClusterIp(false)
	})

	It("Verifies NodePort Service pos upgrade", func() {
		testcase.TestServiceNodePort(false)
	})

	It("Verifies Ingress pos upgrade", func() {
		testcase.TestIngress(false)
	})

	It("Verifies Daemonset pos upgrade", func() {
		testcase.TestDaemonset(false)
	})

	It("Verifies DNS Access pos upgrade", func() {
		testcase.TestDnsAccess(true)
	})
})

var _ = AfterEach(func() {
	if CurrentSpecReport().Failed() {
		fmt.Printf("\nFAILED! %s\n", CurrentSpecReport().FullText())
	} else {
		fmt.Printf("\nPASSED! %s\n", CurrentSpecReport().FullText())
	}
})
