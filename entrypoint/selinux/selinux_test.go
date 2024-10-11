package selinux

import (
	"fmt"

	"github.com/rancher/distros-test-framework/pkg/assert"
	"github.com/rancher/distros-test-framework/pkg/customflag"
	"github.com/rancher/distros-test-framework/pkg/testcase"

	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Test:", func() {

	It("Start Up with no issues", func() {
		testcase.TestBuildCluster(cluster)
	})

	It("Validate Nodes", func() {
		testcase.TestNodeStatus(
			cluster,
			assert.NodeAssertReadyStatus(),
			nil,
		)
	})

	It("Validate Pods", func() {
		testcase.TestPodStatus(
			cluster,
			assert.PodAssertRestart(),
			assert.PodAssertReady())
	})

	It("Validate selinux is enabled", func() {
		testcase.TestSelinuxEnabled(cluster)
	})

	It("Validate container, server and selinux version", func() {
		testcase.TestSelinux(cluster)
	})

	It("Validate container security", func() {
		testcase.TestSelinuxSpcT(cluster)
	})

	It("Validate context", func() {
		testcase.TestSelinuxContext(cluster)
	})

	if customflag.ServiceFlag.InstallMode.String() != "" {
		It("Upgrade manual", func() {
			_ = testcase.TestUpgradeClusterManual(cluster, customflag.ServiceFlag.InstallMode.String())
		})

		It("Validate Nodes Post upgrade", func() {
			testcase.TestNodeStatus(
				cluster,
				assert.NodeAssertReadyStatus(),
				assert.NodeAssertVersionTypeUpgrade(&customflag.ServiceFlag),
			)
		})

		It("Validate Pods Post upgrade", func() {
			testcase.TestPodStatus(
				cluster,
				assert.PodAssertRestart(),
				assert.PodAssertReady())
		})

		It("Validate selinux is enabled Post upgrade", func() {
			testcase.TestSelinuxEnabled(cluster)
		})

		It("Validate container, server and selinux version Post upgrade", func() {
			testcase.TestSelinux(cluster)
		})

		It("Validate container security Post upgrade", func() {
			testcase.TestSelinuxSpcT(cluster)
		})

		It("Validate context", func() {
			testcase.TestSelinuxContext(cluster)
		})
	}

	It("Validate uninstall selinux policies", func() {
		testcase.TestUninstallPolicy(cluster)
	})

})

var _ = AfterEach(func() {
	if CurrentSpecReport().Failed() {
		fmt.Printf("\nFAILED! %s\n\n", CurrentSpecReport().FullText())
	} else {
		fmt.Printf("\nPASSED! %s\n\n", CurrentSpecReport().FullText())
	}
})
