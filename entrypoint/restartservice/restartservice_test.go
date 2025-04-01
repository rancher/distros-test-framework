package restartservice

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

	It("Validate Nodes before service restarts", func() {
		testcase.TestNodeStatus(
			cluster,
			assert.NodeAssertReadyStatus(),
			nil,
		)
	})

	It("Validate Pods before service restarts", func() {
		testcase.TestPodStatus(
			cluster,
			assert.PodAssertRestart(),
			assert.PodAssertReady())
	})

	It("Verifies ClusterIP Service before service restarts", func() {
		testcase.TestServiceClusterIP(true, false)
	})

	It("Verifies NodePort Service before service restarts", func() {
		testcase.TestServiceNodePort(true, false)
	})

	It("Verifies Ingress before service restarts", func() {
		testcase.TestIngress(true, false)
	})

	if cluster.Config.Product == "k3s" {
		It("Verifies Local Path Provisioner storage before service restarts", func() {
			testcase.TestLocalPathProvisionerStorage(cluster, true, false)
		})

		It("Verifies LoadBalancer Service before service restarts", func() {
			testcase.TestServiceLoadBalancer(true, false)
		})
	}

	It("Restart service on server and agent nodes", func() {
		testcase.TestRestartService(cluster)
	})

	It("Validate Nodes after service restarts", func() {
		testcase.TestNodeStatus(
			cluster,
			assert.NodeAssertReadyStatus(),
			nil,
		)
	})

	It("Validate Pods after service restarts", func() {
		testcase.TestPodStatus(
			cluster,
			assert.PodAssertRestart(),
			assert.PodAssertReady())
	})

	It("Verifies ClusterIP Service after service restarts", func() {
		testcase.TestServiceClusterIP(false, true)
	})

	It("Verifies NodePort Service after service restarts", func() {
		testcase.TestServiceNodePort(false, true)
	})

	It("Verifies Ingress after service restarts", func() {
		testcase.TestIngress(false, true)
	})

	It("Verifies Daemonset", func() {
		testcase.TestDaemonset(true, true)
	})

	It("Verifies dns access", func() {
		testcase.TestDNSAccess(true, true)
	})

	if cluster.Config.Product == "k3s" {
		It("Verifies Local Path Provisioner storage after service restarts", func() {
			testcase.TestLocalPathProvisionerStorage(cluster, false, true)
		})

		It("Verifies LoadBalancer Service after service restarts", func() {
			testcase.TestServiceLoadBalancer(false, true)
		})
	}
})

var _ = AfterEach(func() {
	if CurrentSpecReport().Failed() {
		fmt.Printf("\nFAILED! %s\n\n", CurrentSpecReport().FullText())
	} else {
		fmt.Printf("\nPASSED! %s\n\n", CurrentSpecReport().FullText())
	}
})
