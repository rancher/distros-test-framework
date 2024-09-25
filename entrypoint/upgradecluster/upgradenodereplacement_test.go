//go:build upgradereplacement

package upgradecluster

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

	It("Validate Node", func() {
		testcase.TestNodeStatus(
			cluster,
			assert.NodeAssertReadyStatus(),
			nil)
	})

	It("Validate Pod", func() {
		testcase.TestPodStatus(
			cluster,
			assert.PodAssertRestart(),
			assert.PodAssertReady())
	})

	It("Verifies ClusterIP Service pre-upgrade", func() {
		testcase.TestServiceClusterIP(true, false)
	})

	if cluster.Config.Product == "k3s" {
		It("Verifies LoadBalancer Service pre-upgrade", func() {
			testcase.TestServiceLoadBalancer(true, false)
		})
	}

	It("Verifies Ingress pre-upgrade", func() {
		testcase.TestIngress(true, false)
	})

	It("Upgrade by Node replacement", func() {
		testcase.TestUpgradeReplaceNode(cluster, flags)
	})

	It("Checks Node Status after upgrade and validate version", func() {
		testcase.TestNodeStatus(
			cluster,
			assert.NodeAssertReadyStatus(),
			assert.NodeAssertVersionTypeUpgrade(&customflag.ServiceFlag))
	})

	It("Checks Pod Status after upgrade", func() {
		testcase.TestPodStatus(
			cluster,
			assert.PodAssertRestart(),
			assert.PodAssertReady())
	})

	It("Verifies ClusterIP Service after upgrade", func() {
		testcase.TestServiceClusterIP(false, true)
	})

	It("Verifies NodePort Service after upgrade applying and deleting workload", func() {
		testcase.TestServiceNodePort(true, true)
	})

	if cluster.Config.Product == "k3s" {
		It("Verifies LoadBalancer Service after upgrade", func() {
			testcase.TestServiceLoadBalancer(false, true)
		})
	}

	// TestIngress needs to run at the end to ensure it has the ip back in after the upgrade.
	It("Verifies Ingress after upgrade", func() {
		testcase.TestIngress(false, true)
	})
})

var _ = AfterEach(func() {
	if CurrentSpecReport().Failed() {
		fmt.Printf("\nFAILED! %s\n\n", CurrentSpecReport().FullText())
	} else {
		fmt.Printf("\nPASSED! %s\n\n", CurrentSpecReport().FullText())
	}
})
