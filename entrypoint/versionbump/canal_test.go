//go:build canal

package versionbump

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	"github.com/rancher/distros-test-framework/pkg/assert"
	"github.com/rancher/distros-test-framework/pkg/customflag"
	"github.com/rancher/distros-test-framework/pkg/template"
	"github.com/rancher/distros-test-framework/pkg/testcase"
)

var _ = Describe("VersionTemplate Upgrade:", func() {
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

	It("Verifies bump version", func() {
		template.VersionTemplate(template.VersionTestTemplate{
			TestCombination: &template.RunCmd{
				Run: []template.TestMap{
					{
						Cmd: "kubectl -n kube-system get pods -l k8s-app=canal -o jsonpath=\"{..image}\", " +
							"kubectl -n kube-system get pods -l k8s-app=canal -o jsonpath=\"{..image}\" ",
						ExpectedValue:        template.TestMapTemplate.ExpectedValue,
						ExpectedValueUpgrade: template.TestMapTemplate.ExpectedValueUpgrade,
					},
				},
			},
			InstallUpgrade: customflag.ServiceFlag.InstallUpgrade,
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
