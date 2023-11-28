//go:build cilium

package versionbump

import (
	"fmt"

	"github.com/rancher/distros-test-framework/pkg/assert"
	"github.com/rancher/distros-test-framework/pkg/customflag"
	"github.com/rancher/distros-test-framework/pkg/template"
	"github.com/rancher/distros-test-framework/pkg/testcase"

	. "github.com/onsi/ginkgo/v2"
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

	It("Verifies bump version on rke2 for cilium version", func() {
		template.VersionTemplate(template.VersionTestTemplate{
			TestCombination: &template.RunCmd{
				Run: []template.TestMap{
					{
						Cmd: "sudo /var/lib/rancher/rke2/bin/crictl --config /var/lib/rancher/rke2/agent/etc/crictl.yaml " +
							"images | grep cilium , rke2 -v",
						ExpectedValue:        template.TestMapTemplate.ExpectedValue,
						ExpectedValueUpgrade: template.TestMapTemplate.ExpectedValueUpgrade,
					},
				},
			},
			InstallMode: customflag.ServiceFlag.InstallMode.String(),
			TestConfig: &template.TestConfig{
				TestFunc:       template.ConvertToTestCase(customflag.ServiceFlag.TestConfig.TestFuncs),
				ApplyWorkload: customflag.ServiceFlag.TestConfig.ApplyWorkload,
				DeleteWorkload: customflag.ServiceFlag.TestConfig.DeleteWorkload,
				WorkloadName:   customflag.ServiceFlag.TestConfig.WorkloadName,
			},
			Description: customflag.ServiceFlag.TestConfig.Description,
		})
	})

	It("Verifies ClusterIP Service", func() {
		testcase.TestServiceClusterIp(true, true)
	})

	It("Verifies NodePort Service", func() {
		testcase.TestServiceNodePort(true, true)
	})

	It("Verifies Ingress", func() {
		testcase.TestIngress(true, true)
	})

	It("Verifies Daemonset", func() {
		testcase.TestDaemonset(true, true)
	})

	It("Verifies dns access", func() {
		testcase.TestDnsAccess(true, true)
	})
})

var _ = AfterEach(func() {
	if CurrentSpecReport().Failed() {
		fmt.Printf("\nFAILED! %s\n", CurrentSpecReport().FullText())
	} else {
		fmt.Printf("\nPASSED! %s\n", CurrentSpecReport().FullText())
	}
})
