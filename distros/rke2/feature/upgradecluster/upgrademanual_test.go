package upgradecluster

import (
	"fmt"

	"github.com/rancher/distros-test-framework/lib/assert"
	"github.com/rancher/distros-test-framework/cmd"
	"github.com/rancher/distros-test-framework/component/fixture"
	"github.com/rancher/distros-test-framework/lib/shared"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Test:", func() {

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

	It("Verifies ClusterIP Service Pre upgrade", func() {
		fixture.TestServiceClusterIp(true)
	})

	It("Verifies NodePort Service Pre upgrade", func() {
		fixture.TestServiceNodePort(true)
	})

	It("Verifies Ingress Pre upgrade", func() {
		fixture.TestIngress(true)
	})

	It("Verifies Daemonset Pre upgrade", func() {
		fixture.TestDaemonset(true)
	})

	It("Verifies DNS Access Pre upgrade", func() {
		fixture.TestDnsAccess(true)
	})

	It("Upgrade manual", func() {
		err := fixture.TestUpgradeClusterManually(
			cmd.ServiceFlag.ClusterConfig.Product.String(), 
			cmd.ServiceFlag.InstallUpgrade.String())
		Expect(err).NotTo(HaveOccurred())
	})

	It("Checks Node Status pos upgrade", func() {
		fixture.TestNodeStatus(
			assert.NodeAssertReadyStatus(),
			assert.NodeAssertVersionTypeUpgrade(cmd.ServiceFlag, cmd.ServiceFlag.ClusterConfig.Product.String()),
			cmd.ServiceFlag.ClusterConfig.Product.String(),
		)
	})

	It("Checks Pod Status pos upgrade", func() {
		fixture.TestPodStatus(
			nil,
			assert.PodAssertReady(),
			assert.PodAssertStatus(),
		)
	})

	It("Verifies ClusterIP Service Post upgrade", func() {
		fixture.TestServiceClusterIp(false)
		defer shared.ManageWorkload("delete", "clusterip.yaml")
	})

	It("Verifies NodePort Service Post upgrade", func() {
		fixture.TestServiceNodePort(false)
		defer shared.ManageWorkload("delete", "nodeport.yaml")
	})

	It("Verifies Ingress Post upgrade", func() {
		fixture.TestIngress(false)
		defer shared.ManageWorkload("delete", "ingress.yaml")
	})

	It("Verifies Daemonset Post upgrade", func() {
		fixture.TestDaemonset(false)
		defer shared.ManageWorkload("delete", "daemonset.yaml")
	})

	It("Verifies DNS Access Post upgrade", func() {
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
