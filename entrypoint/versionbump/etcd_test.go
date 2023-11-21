//go:build etcd

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

	It("Verifies bump version on product for etcd", func() {
		cmd := "sudo journalctl -u k3s | grep 'etcd-version' | awk -F'\"' " +
			"'{ for(i=1; i<=NF; ++i) if($i == \"etcd-version\") print $(i+2) }' ," + "k3s -v"
		if cfg.Product == "rke2" {
			cmd = "sudo /var/lib/rancher/rke2/bin/crictl -r unix:///run/k3s/containerd/containerd.sock images | grep etcd ," +
				"rke2 -v"
		}

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
				ApplyWorkload: customflag.ServiceFlag.TestConfig.ApplyWorkload,
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
