//go:build versionbump

package versionbump

import (
	"fmt"

	"github.com/rancher/distros-test-framework/pkg/assert"
	"github.com/rancher/distros-test-framework/pkg/customflag"
	. "github.com/rancher/distros-test-framework/pkg/template"
	"github.com/rancher/distros-test-framework/pkg/testcase"

	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Version Bump Template Upgrade:", func() {
	It("Start Up with no issues", func() {
		testcase.TestBuildCluster(GinkgoT())
	})

	It("Validate Nodes", func() {
		testcase.TestNodeStatus(
			assert.NodeAssertReadyStatus(),
			nil)
	})

	It("Validate Pods", func() {
		testcase.TestPodStatus(
			assert.PodAssertRestart(),
			assert.PodAssertReady(),
			assert.PodAssertStatus())
	})

	It("Test Bump version", func() {
		Template(TestTemplate{
			TestCombination: &RunCmd{
				Run: []TestMap{
					{
						Cmd:                  TestMapTemplate.Cmd,
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
