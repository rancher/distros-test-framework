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

	It("Verifies Ingress Before Restore", func() {
		testcase.TestIngress(true, false)
	})

	It("Verifies NodePort Service Before Restore", func() {
		testcase.TestServiceNodePort(true, false)
	})

	It("Verifies Cluster Reset Restore", func() {
		testcase.TestClusterRestore(cluster, awsClient, cfg, flags)
	})

	It("Verifies ClusterIP Service after Restore if we can apply, work and delete workloads", func() {
		testcase.TestServiceClusterIP(true, true)
	})

	It("Verifies NodePort Service after Restore if service is still there and we can access it", func() {
		testcase.TestServiceNodePort(false, false)
	})

	It("Verifies Ingress after Restore if still there and we can access it", func() {
		testcase.TestIngress(false, false)
	})
})

var _ = AfterEach(func() {
	if CurrentSpecReport().Failed() {
		fmt.Printf("\nFAILED! %s\n", CurrentSpecReport().FullText())
	} else {
		fmt.Printf("\nPASSED! %s\n", CurrentSpecReport().FullText())
	}
})
