package rebootinstances

import (
	"fmt"

	"github.com/rancher/distros-test-framework/pkg/assert"
	"github.com/rancher/distros-test-framework/pkg/testcase"

	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Test:", func() {

	It("Start Up with no issues on rebootinstances test", func() {
		testcase.TestBuildCluster(cluster)
	})

	It("Validate Nodes", func() {
		testcase.TestNodeStatus(
			cluster,
			assert.NodeAssertReadyStatus(),
			nil,
		)
	})

	It("Validate Pods", func() {
		testcase.TestPodStatus(
			cluster,
			assert.PodAssertRestart(),
			assert.PodAssertReady())
	})

	It("Verifies ClusterIP Service", func() {
		testcase.TestServiceClusterIP(true, true)
	})

	It("Verifies NodePort Service", func() {
		testcase.TestServiceNodePort(true, true)
	})

	It("Verifies Daemonset", func() {
		testcase.TestDaemonset(true, true)
	})

	It("Reboot server and agent nodes", func() {
		testcase.TestRebootInstances(cluster)
	})

	It("Validate Nodes after reboot", func() {
		testcase.TestNodeStatus(
			cluster,
			assert.NodeAssertReadyStatus(),
			nil,
		)
	})

	It("Verifies ClusterIP Service after reboot", func() {
		testcase.TestServiceClusterIP(true, true)
	})

	It("Verifies NodePort Service after reboot", func() {
		testcase.TestServiceNodePort(true, true)
	})

	It("Verifies Daemonset after reboot", func() {
		testcase.TestDaemonset(true, true)
	})

	It("Verifies dns access after reboot", func() {
		testcase.TestDNSAccess(true, true)
	})

	if cluster.Config.Product == "k3s" {
		It("Verifies Local Path Provisioner storage", func() {
			testcase.TestLocalPathProvisionerStorage(cluster, true, true)
		})

		It("Verifies LoadBalancer Service", func() {
			testcase.TestServiceLoadBalancer(true, true)
		})

		It("Verifies Traefik IngressRoute using old GKV", func() {
			testcase.TestIngressRoute(cluster, true, true, "traefik.containo.us/v1alpha1")
		})

		It("Verifies Traefik IngressRoute using new GKV", func() {
			testcase.TestIngressRoute(cluster, true, true, "traefik.io/v1alpha1")
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
