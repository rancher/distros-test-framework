//go:build canal

package versionbump

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"

	"github.com/rancher/distros-test-framework/pkg/assert"
	. "github.com/rancher/distros-test-framework/pkg/customflag"
	. "github.com/rancher/distros-test-framework/pkg/template"
	"github.com/rancher/distros-test-framework/pkg/testcase"
)

const (
	calicoCmd  = kgn + " : | grep 'hardened-calico' -A1, "
	flannelCmd = kgn + " : | grep 'hardened-flannel' -A1"
)

var _ = Describe("Canal Version bump:", func() {
	It("Start Up with no issues", func() {
		testcase.TestBuildCluster(cluster)
	})

	It("Validate Node", func() {
		testcase.TestNodeStatus(
			cluster,
			assert.NodeAssertReadyStatus(),
			nil)
	})

	It("Validate Pod", func() {
		testcase.TestPodStatus(
			cluster,
			assert.PodAssertRestart(),
			assert.PodAssertReady())
	})

	It("Test Calico and Flannel version", func() {
		Template(TestTemplate{
			TestCombination: &RunCmd{
				Run: []TestMapConfig{
					{
						Cmd:                  calicoCmd + flannelCmd,
						ExpectedValue:        TestMap.ExpectedValue,
						ExpectedValueUpgrade: TestMap.ExpectedValueUpgrade,
					},
				},
			},
			InstallMode: ServiceFlag.InstallMode.String(),
		})
	})

	It("Verifies ClusterIP Service", func() {
		testcase.TestServiceClusterIP(true, true)
	})

	It("Verifies NodePort Service", func() {
		testcase.TestServiceNodePort(true, true)
	})

	It("Verifies Ingress", func() {
		testcase.TestIngress(true, true)
	})
})

var _ = AfterEach(func() {
	if CurrentSpecReport().Failed() {
		fmt.Printf("\nFAILED! %s\n\n", CurrentSpecReport().FullText())
	} else {
		fmt.Printf("\nPASSED! %s\n\n", CurrentSpecReport().FullText())
	}
})
