//go:build storageconformance

package sonobuoyconformance

import (
	"fmt"

	"github.com/rancher/distros-test-framework/pkg/assert"
	"github.com/rancher/distros-test-framework/pkg/testcase"

	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Sonobuoy Storage Conformance Tests...", func() {
	It("Starts Up with no issues", func() {
		testcase.TestBuildCluster(cluster)
	})

	It("Validates Node", func() {
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

	It("Validates the storage conformance with longhorn", func() {
		testcase.TestStorageConformance(flags.External.SonobuoyVersion)
	})
})