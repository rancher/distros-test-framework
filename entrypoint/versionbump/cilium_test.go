//go:build cilium

package versionbump

import (
	"fmt"

	"github.com/rancher/distros-test-framework/pkg/assert"
	"github.com/rancher/distros-test-framework/pkg/customflag"
	"github.com/rancher/distros-test-framework/pkg/template"
	"github.com/rancher/distros-test-framework/pkg/testcase"
	"github.com/rancher/distros-test-framework/shared"

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
			InstallUpgrade: customflag.ServiceFlag.InstallType.String(),
			TestConfig: &template.TestConfig{
				TestFunc:       template.ConvertToTestCase(customflag.ServiceFlag.TestConfig.TestFuncs),
				DeployWorkload: customflag.ServiceFlag.TestConfig.DeployWorkload,
				WorkloadName:   customflag.ServiceFlag.TestConfig.WorkloadName,
			},
			Description: customflag.ServiceFlag.TestConfig.Description,
		})
	})

	It("Verifies ClusterIP Service", func() {
		testcase.TestServiceClusterIp(true)
		shared.ManageWorkload("delete", "clusterip.yaml",
			customflag.ServiceFlag.ClusterConfig.Arch.String())
	})

	It("Verifies NodePort Service", func() {
		testcase.TestServiceNodePort(true)
		shared.ManageWorkload("delete", "nodeport.yaml",
			customflag.ServiceFlag.ClusterConfig.Arch.String())
	})

	It("Verifies Ingress", func() {
		testcase.TestIngress(true)
		shared.ManageWorkload("delete", "ingress.yaml",
			customflag.ServiceFlag.ClusterConfig.Arch.String())
	})

	It("Verifies Daemonset", func() {
		testcase.TestDaemonset(true)
		shared.ManageWorkload("delete", "daemonset.yaml",
			customflag.ServiceFlag.ClusterConfig.Arch.String())
	})

	It("Verifies dns access", func() {
		testcase.TestDnsAccess(true)
		shared.ManageWorkload("delete", "dnsutils.yaml",
			customflag.ServiceFlag.ClusterConfig.Arch.String())
	})
})

var _ = AfterEach(func() {
	if CurrentSpecReport().Failed() {
		fmt.Printf("\nFAILED! %s\n", CurrentSpecReport().FullText())
	} else {
		fmt.Printf("\nPASSED! %s\n", CurrentSpecReport().FullText())
	}
})
