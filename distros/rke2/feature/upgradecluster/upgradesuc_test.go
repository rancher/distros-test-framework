package upgradecluster

import (
	"fmt"

	"github.com/rancher/distros-test-framework/cmd"
	"github.com/rancher/distros-test-framework/component/fixture"
	"github.com/rancher/distros-test-framework/lib/assert"
	"github.com/rancher/distros-test-framework/lib/shared"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("SUC Upgrade Tests:", func() {

	It("Starts up with no issues", func() {
		fixture.TestBuildCluster(GinkgoT(), cmd.ServiceFlag.ClusterConfig.Product.String())
	})

	It("Validate Nodes", func() {
		fixture.TestNodeStatus(
			assert.NodeAssertReadyStatus(),
			nil,
			cmd.ServiceFlag.ClusterConfig.Product.String(),
		)
	})

	It("Validate Pods", func() {
		fixture.TestPodStatus(
			assert.PodAssertRestart(),
			assert.PodAssertReady(),
			assert.PodAssertStatus(),
		)
	})

	It("Verifies ClusterIP Service pre upgrade", func() {
		fixture.TestServiceClusterIp(true)
	})

	It("Verifies NodePort Service pre upgrade", func() {
		fixture.TestServiceNodePort(true)
	})

	It("Verifies Ingress pre upgrade", func() {
		fixture.TestIngress(true)
	})

	It("Verifies Daemonset pre upgrade", func() {
		fixture.TestDaemonset(true)
	})

	It("Verifies DNS Access pre upgrade", func() {
		fixture.TestDnsAccess(true)
	})

	It("\nUpgrade via SUC", func() {
		err := fixture.TestUpgradeClusterSUC(cmd.ServiceFlag.UpgradeVersionSUC.String())
		Expect(err).NotTo(HaveOccurred())
	})

	It("Checks Node Status pos upgrade suc", func() {
		fixture.TestNodeStatus(
			assert.NodeAssertReadyStatus(),
			assert.NodeAssertVersionUpgraded(),
			cmd.ServiceFlag.ClusterConfig.Product.String(),
		)
	})

	It("Checks Pod Status pos upgrade suc", func() {
		fixture.TestPodStatus(
			nil,
			assert.PodAssertReady(),
			assert.PodAssertStatus(),
		)
	})

	It("Verifies ClusterIP Service pos upgrade", func() {
		fixture.TestServiceClusterIp(false)
		defer shared.ManageWorkload("delete", "clusterip.yaml")
	})

	It("Verifies NodePort Service pos upgrade", func() {
		fixture.TestServiceNodePort(false)
		defer shared.ManageWorkload("delete", "nodeport.yaml")
	})

	It("Verifies Ingress pos upgrade", func() {
		fixture.TestIngress(false)
		defer shared.ManageWorkload("delete", "ingress.yaml")
	})

	It("Verifies Daemonset pos upgrade", func() {
		fixture.TestDaemonset(false)
		defer shared.ManageWorkload("delete", "daemonset.yaml")
	})

	It("Verifies DNS Access pos upgrade", func() {
		fixture.TestDnsAccess(true)
		defer shared.ManageWorkload("delete", "dns.yaml")
	})
})

var _ = AfterEach(func() {
	if CurrentSpecReport().Failed() {
		fmt.Printf("\nFAILED! %s\n", CurrentSpecReport().FullText())
	} else {
		fmt.Printf("\nPASSED! %s\n", CurrentSpecReport().FullText())
	}
})
