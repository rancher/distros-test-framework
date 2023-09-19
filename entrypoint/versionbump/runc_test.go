//go:build runc

package versionbump

import (
	"fmt"

	"github.com/rancher/distros-test-framework/pkg/customflag"
	"github.com/rancher/distros-test-framework/pkg/template"
	"github.com/rancher/distros-test-framework/pkg/testcase"

	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("VersionTemplate Upgrade:", func() {

	It("Start Up with no issues", func() {
		testcase.TestBuildCluster(GinkgoT())
	})

	// It("Validate Nodes", func() {
	// 	testcase.TestNodeStatus(
	// 		assert.NodeAssertReadyStatus(),
	// 		nil,
	// 	)
	// })
	//
	// It("Validate Pods", func() {
	// 	testcase.TestPodStatus(
	// 		assert.PodAssertRestart(),
	// 		assert.PodAssertReady(),
	// 		assert.PodAssertStatus(),
	// 	)
	// })

	cmd := fmt.Sprintf("(find /var/lib/rancher/%s/data/ -type f -name runc -exec {} --version \\;)",
		cfg.Product)
	It("Verifies Runc bump", func() {
		template.VersionTemplate(template.VersionTestTemplate{
			TestCombination: &template.RunCmd{
				Run: []template.TestMap{
					{
						Cmd:                  cmd,
						ExpectedValue:        template.TestMapTemplate.ExpectedValue,
						ExpectedValueUpgrade: template.TestMapTemplate.ExpectedValueUpgrade,
					},
				},
			},
			InstallMode: customflag.ServiceFlag.InstallMode.String(),
			TestConfig: &template.TestConfig{
				TestFunc:       template.ConvertToTestCase(customflag.ServiceFlag.TestConfig.TestFuncs),
				DeployWorkload: customflag.ServiceFlag.TestConfig.DeployWorkload,
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
