package clusterrestore

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

	It("Verifies ClusterIP Service Before Snapshot", func() {
		testcase.TestServiceClusterIP(true, false)
	})

	It("Verifies NodePort Service Before Snapshot", func() {
		testcase.TestServiceNodePort(true, false)
	})

	// deploy more workloads before and after snapshot -- do not delete the workloads
	It("Verifies Cluster Reset Restore", func() {
		testcase.TestClusterRestoreS3(cluster, true, false, flags)
	})

	// It("Verifies Ingress After Snapshot", func() {
	// 	testcase.TestIngress(true, true)
	// })

	// It("Validate Nodes", func() {
	// 	testcase.TestNodeStatus(
	// 		cluster,
	// 		assert.NodeAssertReadyStatus(),
	// 		nil,
	// 	)
	// })

	// It("Validate Pods", func() {
	// 	testcase.TestPodStatus(
	// 		cluster,
	// 		assert.PodAssertRestart(),
	// 		assert.PodAssertReady())
	// })

	// It("Verifies Daemonset", func() {
	// 	testcase.TestDaemonset(true, true)
	// })

	// It("Verifies NodePort Service After Reset", func() {
	// 	testcase.TestServiceNodePort(false, true)
	// })

	// It("Verifies dns access", func() {
	// 	testcase.TestDNSAccess(true, true)
	// })

	// if cluster.Config.Product == "k3s" {
	// 	It("Verifies Local Path Provisioner storage", func() {
	// 		testcase.TestLocalPathProvisionerStorage(cluster, true, true)
	// 	})

	// 	It("Verifies LoadBalancer Service", func() {
	// 		testcase.TestServiceLoadBalancer(true, true)
	// 	})
	// }
})

var _ = AfterEach(func() {
	if CurrentSpecReport().Failed() {
		fmt.Printf("\nFAILED! %s\n", CurrentSpecReport().FullText())
	} else {
		fmt.Printf("\nPASSED! %s\n", CurrentSpecReport().FullText())
	}
})
