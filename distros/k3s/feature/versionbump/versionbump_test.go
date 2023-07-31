package versionbump

import (
	"fmt"

	"github.com/rancher/distros-test-framework/lib/assert"
	"github.com/rancher/distros-test-framework/cmd"
	"github.com/rancher/distros-test-framework/component/template"
	"github.com/rancher/distros-test-framework/component/fixture"

	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("VersionTemplate Upgrade:", func() {

	It("Start Up with no issues", func() {
		fixture.TestBuildCluster(GinkgoT(), cmd.ServiceFlag.ClusterConfig.Product.String())
	})

	It("Validate Node", func() {
		fixture.TestNodeStatus(
			assert.NodeAssertReadyStatus(),
			nil,
			cmd.ServiceFlag.ClusterConfig.Product.String(),)
	})

	It("Validate Pod", func() {
		fixture.TestPodStatus(
			assert.PodAssertRestart(),
			assert.PodAssertReady(),
			assert.PodAssertStatus())
	})

	It("Test Bump version", func() {
		template.VersionTemplate(template.VersionTestTemplate{
			TestCombination: &template.RunCmd{
				Run: []template.TestMap{
					{
						Cmd:                  template.TestMapTemplate.Cmd,
						ExpectedValue:        template.TestMapTemplate.ExpectedValue,
						ExpectedValueUpgrade: template.TestMapTemplate.ExpectedValueUpgrade,
					},
				},
			},
			InstallUpgrade: cmd.ServiceFlag.InstallUpgrade,
			TestConfig: &template.TestConfig{
				TestFunc:       template.ConvertToTestCase(cmd.ServiceFlag.TestConfig.TestFuncs),
				DeployWorkload: cmd.ServiceFlag.TestConfig.DeployWorkload,
				WorkloadName:   cmd.ServiceFlag.TestConfig.WorkloadName,
			},
			Description: cmd.ServiceFlag.TestConfig.Description,
		}, cmd.ServiceFlag.ClusterConfig.Product.String())
	})
})

var _ = AfterEach(func() {
	if CurrentSpecReport().Failed() {
		fmt.Printf("\nFAILED! %s\n", CurrentSpecReport().FullText())
	} else {
		fmt.Printf("\nPASSED! %s\n", CurrentSpecReport().FullText())
	}
})
