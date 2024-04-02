//go:build versionbump

package versionbump

import (
	"fmt"

	"github.com/rancher/distros-test-framework/pkg/assert"
	"github.com/rancher/distros-test-framework/pkg/productflag"
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
				Run: []productflag.TestMapConfig{
					{
						Cmd:                  productflag.TestMap.Cmd,
						ExpectedValue:        productflag.TestMap.ExpectedValue,
						ExpectedValueUpgrade: productflag.TestMap.ExpectedValueUpgrade,
					},
				},
			},
			InstallMode: productflag.ServiceFlag.InstallMode.String(),
			TestConfig: &TestConfig{
				TestFunc:       ConvertToTestCase(productflag.ServiceFlag.TestTemplateConfig.TestFuncs),
				ApplyWorkload:  productflag.ServiceFlag.TestTemplateConfig.ApplyWorkload,
				DeleteWorkload: productflag.ServiceFlag.TestTemplateConfig.DeleteWorkload,
				WorkloadName:   productflag.ServiceFlag.TestTemplateConfig.WorkloadName,
			},
			Description: productflag.ServiceFlag.TestTemplateConfig.Description,
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
