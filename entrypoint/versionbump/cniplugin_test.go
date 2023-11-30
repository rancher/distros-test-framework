//go:build cniplugin

package versionbump

import (
	"fmt"

	"github.com/rancher/distros-test-framework/pkg/assert"
	"github.com/rancher/distros-test-framework/pkg/customflag"
	. "github.com/rancher/distros-test-framework/pkg/template"
	"github.com/rancher/distros-test-framework/pkg/testcase"

	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("CNI plugin Version Upgrade:", func() {
	It("Start Up with no issues", func() {
		testcase.TestBuildCluster(GinkgoT())
	})

	It("Validate Node", func() {
		testcase.TestNodeStatus(
			assert.NodeAssertReadyStatus(),
			nil)
	})

	It("Validate Pod", func() {
		testcase.TestPodStatus(
			assert.PodAssertRestart(),
			assert.PodAssertReady(),
			assert.PodAssertStatus())
	})

	It("Verifies bump version for cni plugins and flannel", func() {
		Template(TestTemplate{
			TestCombination: &RunCmd{
				Run: []TestMap{
					{
						Cmd:                  "/var/lib/rancher/k3s/data/current/bin/cni , /var/lib/rancher/k3s/data/current/bin/flannel",
						ExpectedValue:        TestMapTemplate.ExpectedValue,
						ExpectedValueUpgrade: TestMapTemplate.ExpectedValueUpgrade,
					},
				},
			},
			InstallMode: customflag.ServiceFlag.InstallMode.String(),
			TestConfig: &TestConfig{
				TestFunc:       ConvertToTestCase(customflag.ServiceFlag.TestConfig.TestFuncs),
				ApplyWorkload:  customflag.ServiceFlag.TestConfig.ApplyWorkload,
				DeleteWorkload: customflag.ServiceFlag.TestConfig.DeleteWorkload,
				WorkloadName:   customflag.ServiceFlag.TestConfig.WorkloadName,
			},
			Description: customflag.ServiceFlag.TestConfig.Description,
		})
	})
})

var _ = AfterEach(func() {
	if CurrentSpecReport().Failed() {
		fmt.Printf("\nFAILED! %s\n", CurrentSpecReport().FullText())
	} else {
		fmt.Printf("\nPASSED! %s\n", CurrentSpecReport().FullText())
	}
})
