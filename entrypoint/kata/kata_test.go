package kata

import (
	"github.com/rancher/distros-test-framework/internal/pkg/assert"
	"github.com/rancher/distros-test-framework/internal/pkg/testcase"
	"github.com/rancher/distros-test-framework/internal/resources"

	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Test Kata Cluster:", Ordered, func() {
	It("Validate Nodes", func() {
		testcase.TestNodeStatus(cluster, assert.NodeAssertReadyStatus(), nil)
	})
	It("Validate Pods", func() {
		testcase.TestPodStatus(cluster, assert.PodAssertRestart(), assert.PodAssertReady())
	})
	It("KATA-01 installs Kata without changing the default runtime", func() {
		testcase.TestKataInstall(cluster, &kataConfig)
	})
	It("KATA-02 correlates explicit and alias Kata pods with KVM guests", func() {
		testcase.TestKataRuntime(&kataConfig)
	})
	It("KATA-03 preserves bidirectional same-node and cross-node DNS/HTTP", func() {
		testcase.TestKataCoexistence(&kataConfig)
	})
	It("KATA-04 rejects an ineligible worker without creating a sandbox", func() {
		testcase.TestKataRejectIneligible(&kataConfig)
	})
	It("KATA-07 recovers existing and fresh workloads after the RKE2 agent restart", func() {
		testcase.TestKataRestart(&kataConfig)
	})
	It("KATA-14 checks the basic guest isolation boundary", func() {
		testcase.TestKataIsolation(&kataConfig)
	})
	It("KATA-15 characterizes cold/warm startup and host resource overhead", func() {
		testcase.TestKataPerformance(&kataConfig)
	})
})

var _ = AfterEach(func() {
	state := "PASSED!"
	if CurrentSpecReport().Failed() {
		state = "FAILED!"
	}
	resources.LogLevel("info", "%s %s", state, CurrentSpecReport().FullText())
})
