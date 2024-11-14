package clusterreset

import (
	"fmt"

	"github.com/rancher/distros-test-framework/pkg/assert"
	"github.com/rancher/distros-test-framework/pkg/testcase"

	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Test:", func() {

	It("Start Up with no issues", func() {
		testcase.TestBuildCluster(cluster)
	})

	It("Validate Nodes Before Reset", func() {
		testcase.TestNodeStatus(
			cluster,
			assert.NodeAssertReadyStatus(),
			nil,
		)
	})

	It("Validate Pods Before Reset", func() {
		testcase.TestPodStatus(
			cluster,
			assert.PodAssertRestart(),
			assert.PodAssertReady())
	})

	It("Verifies ClusterIP Service Before Reset", func() {
		testcase.TestServiceClusterIP(true, true)
	})

	It("Verifies NodePort Service Before Reset", func() {
		testcase.TestServiceNodePort(true, false)
	})

	It("Verifies Cluster Reset", func() {
		testcase.TestClusterReset(cluster)
	})

	It("Validate Nodes After Reset", func() {
		testcase.TestNodeStatus(
			cluster,
			assert.NodeAssertReadyStatus(),
			nil,
		)
	})

	It("Validate Pods After Reset", func() {
		testcase.TestPodStatus(
			cluster,
			assert.PodAssertRestart(),
			assert.PodAssertReady(),
		)
	})

	It("Verifies Ingress After Reset", func() {
		testcase.TestIngress(true, true)
	})

	It("Verifies Daemonset After Reset", func() {
		testcase.TestDaemonset(true, true)
	})

	It("Verifies NodePort Service After Reset", func() {
		testcase.TestServiceNodePort(false, true)
	})

	It("Verifies dns access After Reset", func() {
		testcase.TestDNSAccess(true, true)
	})

	if cluster.Config.Product == "k3s" {
		It("Verifies Local Path Provisioner storage After Reset", func() {
			testcase.TestLocalPathProvisionerStorage(cluster, true, true)
		})

		It("Verifies LoadBalancer Service After Reset", func() {
			testcase.TestServiceLoadBalancer(true, true)
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
