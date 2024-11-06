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

	// deploy more workloads before and after snapshot -- do not delete the workloads
	It("Verifies Cluster Reset Restore", func() {
		testcase.TestClusterRestore(cluster, true, flags)
	})

	// TODO: its better have a custom POST validation as u are already doing.
	// It("Validate Nodes", func() {
	// 	testcase.TestNodeStatus(
	// 		cluster,
	// 		assert.NodeAssertReadyStatus(),
	// 		nil,
	// 	)
	// })
	//
	// It("Validate Pods", func() {
	// 	testcase.TestPodStatus(
	// 		cluster,
	// 		assert.PodAssertRestart(),
	// 		assert.PodAssertReady())
	// })

	// It("Verifies NodePort Service After Snapshot", func() {
	// 	testcase.TestServiceNodePort(false, true)
	// })

})

var _ = AfterEach(func() {
	if CurrentSpecReport().Failed() {
		fmt.Printf("\nFAILED! %s\n", CurrentSpecReport().FullText())
	} else {
		fmt.Printf("\nPASSED! %s\n", CurrentSpecReport().FullText())
	}
})
