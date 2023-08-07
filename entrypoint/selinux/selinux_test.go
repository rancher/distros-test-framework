//go:build selinux

package selinux

import (
	"fmt"

	"github.com/rancher/distros-test-framework/pkg/assert"
	"github.com/rancher/distros-test-framework/pkg/customflag"
	"github.com/rancher/distros-test-framework/pkg/testcase"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Test:", func() {

	It("Start Up with no issues", func() {
		testcase.TestBuildCluster(GinkgoT())
	})

	It("Validate Nodes Pre upgrade", func() {
		testcase.TestNodeStatus(
			assert.NodeAssertReadyStatus(),
			nil,
		)
	})

	It("Validate Pods Pre upgrade", func() {
		testcase.TestPodStatus(
			assert.PodAssertRestart(),
			assert.PodAssertReady(),
			assert.PodAssertStatus(),
		)
	})

	It("Validate selinux is enabled Pre upgrade", func() {
		testcase.TestSelinuxEnabled()
	})

	It("Validate container, server and selinux version Pre upgrade", func() {
		testcase.TestSelinuxVersions()
	})

	It("Validate container security Pre upgrade", func() {
		testcase.TestSelinuxSpcT()
	})

	It("Upgrade manual", func() {
		err := testcase.TestUpgradeClusterManually(customflag.ServiceFlag.InstallUpgrade.String())
		Expect(err).NotTo(HaveOccurred())
	})

	It("Validate Nodes Post upgrade", func() {
		testcase.TestNodeStatus(
			assert.NodeAssertReadyStatus(),
			nil,
		)
	})

	It("Validate Pods Post upgrade", func() {
		testcase.TestPodStatus(
			assert.PodAssertRestart(),
			assert.PodAssertReady(),
			assert.PodAssertStatus(),
		)
	})

	It("Validate selinux is enabled Post upgrade", func() {
		testcase.TestSelinuxEnabled()
	})

	It("Validate container, server and selinux version Post upgrade", func() {
		testcase.TestSelinuxVersions()
	})

	It("Validate container security Post upgrade", func() {
		testcase.TestSelinuxSpcT()
	})

	/*It("Validate uninstall selinux policies", func() {
		testcase.TestUninstallPolicy()
	})*/

})

var _ = AfterEach(func() {
	if CurrentSpecReport().Failed() {
		fmt.Printf("\nFAILED! %s\n", CurrentSpecReport().FullText())
	} else {
		fmt.Printf("\nPASSED! %s\n", CurrentSpecReport().FullText())
	}
})
