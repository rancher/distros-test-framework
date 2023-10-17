package clusterreset

import (
	"fmt"

	"github.com/rancher/distros-test-framework/pkg/assert"
	"github.com/rancher/distros-test-framework/pkg/testcase"

	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Test:", func() {

	FIt("Start Up with no issues", func() {
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

	It("Verifies ClusterIP Service", func() {
		testcase.TestServiceClusterIp(true)
	})

	It("Verifies NodePort Service Before Reset", func() {
		testcase.TestServiceNodePort(false)
	})

	It("Verifies Killall", func() {
		testcase.TestKillall("", "")
	})

	FIt("Verifies Stopped Server", func() {
		testcase.StopServer("", "") //add test case that calls in stopping the service (node.go)
	})

	It("Verifies Cluster Reset", func() {
		testcase.ClusterReset("", "") //add test case that calls in cluster reset testcase (cluster.go)
	})

	It("Verifies Database Directories Deleted", func() {
		testcase.DeleteDatabaseDirectories("", "")
	})

	It("Verifies Started Server", func() {
		testcase.StartServer("", "") //add test case that calls in started service (node.go)
	})

	It("Verifies Ingress", func() {
		testcase.TestIngress(true)
	})

	It("Verifies Daemonset", func() {
		testcase.TestDaemonset(true)
	})

	It("Verifies NodePort Service After Reset", func() {
		testcase.TestServiceNodePort(false)
	})

	It("Verifies dns access", func() {
		testcase.TestDnsAccess(true)
	})

	if cfg.Product == "k3s" {
		It("Verifies Local Path Provisioner storage", func() {
			testcase.TestLocalPathProvisionerStorage(true)
		})

		It("Verifies LoadBalancer Service", func() {
			testcase.TestServiceLoadBalancer(true)
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
