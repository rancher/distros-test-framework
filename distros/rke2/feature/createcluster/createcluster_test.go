package createcluster

import (
	"fmt"

	"github.com/rancher/distros-test-framework/lib/assert"
	"github.com/rancher/distros-test-framework/cmd"
	"github.com/rancher/distros-test-framework/component/fixture"
	"github.com/rancher/distros-test-framework/lib/shared"

	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Test:", func() {

	It("Start Up with no issues", func() {
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

	It("Verifies ClusterIP Service", func() {
		fixture.TestServiceClusterIp(true)
		defer shared.ManageWorkload("delete", "clusterip.yaml")
	})

	It("Verifies NodePort Service", func() {
		fixture.TestServiceNodePort(true)
		defer shared.ManageWorkload("delete", "nodeport.yaml")
	})

	It("Verifies Ingress", func() {
		fixture.TestIngress(true)
		defer shared.ManageWorkload("delete", "ingress.yaml")
	})

	It("Verifies Daemonset", func() {
		fixture.TestDaemonset(true)
		defer shared.ManageWorkload("delete", "daemonset.yaml")
	})

	It("Verifies dns access", func() {
		fixture.TestDnsAccess(true)
		defer shared.ManageWorkload("delete", "dnsutils.yaml")
	})
})

var _ = AfterEach(func() {
	if CurrentSpecReport().Failed() {
		fmt.Printf("\nFAILED! %s\n", CurrentSpecReport().FullText())
	} else {
		fmt.Printf("\nPASSED! %s\n", CurrentSpecReport().FullText())
	}
})
