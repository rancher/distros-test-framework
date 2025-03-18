//go:build upgrademanual

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
			nil,
		)
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

	It("Verifies NodePort Service pre-upgrade", func() {
		testcase.TestServiceNodePort(true, false)
	})

	It("Verifies Ingress pre-upgrade", func() {
		testcase.TestIngress(true, false)
	})

	It("Verifies Daemonset pre-upgrade", func() {
		testcase.TestDaemonset(true, false)
	})

	It("Verifies dns access pre-upgrade", func() {
		testcase.TestDNSAccess(true, false)
	})

	if cluster.Config.Product == "k3s" {
		It("Verifies LoadBalancer Service pre-upgrade", func() {
			testcase.TestServiceLoadBalancer(true, false)
		})

		It("Verifies Local Path Provisioner storage pre-upgrade", func() {
			testcase.TestLocalPathProvisionerStorage(cluster, true, false)
		})

		// TODO: Remove once v1.32 is the minimum version
		It("Verifies Traefik IngressRoute using old GKV pre-upgrade", func() {
			testcase.TestIngressRoute(cluster, true, false, "traefik.containo.us/v1alpha1")
		})

		It("Verifies Traefik IngressRoute using new GKV pre-upgrade", func() {
			testcase.TestIngressRoute(cluster, true, false, "traefik.io/v1alpha1")
		})
	}

	It("Upgrade Manual", func() {
		_ = testcase.TestUpgradeClusterManual(cluster, k8sClient, flags.InstallMode.String())
	})

	It("Checks Node Status after upgrade and validate version", func() {
		testcase.TestNodeStatus(
			cluster,
			assert.NodeAssertReadyStatus(),
			assert.NodeAssertVersionTypeUpgrade(&customflag.ServiceFlag),
		)
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

	It("Verifies NodePort Service after upgrade", func() {
		testcase.TestServiceNodePort(false, true)
	})

	It("Verifies Ingress after upgrade", func() {
		testcase.TestIngress(false, true)
	})

	It("Verifies Daemonset after upgrade", func() {
		testcase.TestDaemonset(false, true)
	})

	It("Verifies dns access after upgrade", func() {
		testcase.TestDNSAccess(false, true)
	})

	if cluster.Config.Product == "k3s" {
		It("Verifies LoadBalancer Service after upgrade", func() {
			testcase.TestServiceLoadBalancer(false, true)
		})

		It("Verifies Local Path Provisioner storage after upgrade", func() {
			testcase.TestLocalPathProvisionerStorage(cluster, false, true)
		})

		// TODO: Remove once v1.32 is the minimum version
		It("Verifies Traefik IngressRoute after upgrade using old GKV", func() {
			testcase.TestIngressRoute(cluster, false, true, "traefik.containo.us/v1alpha1")
		})

		It("Verifies Traefik IngressRoute after upgrade using new GKV", func() {
			testcase.TestIngressRoute(cluster, false, true, "traefik.io/v1alpha1")
		})
	}
})

var _ = AfterEach(func() {
	if CurrentSpecReport().Failed() {
		fmt.Printf("\nFAILED! %s\n\n", CurrentSpecReport().FullText())
	} else {
		fmt.Printf("\nPASSED! %s\n\n", CurrentSpecReport().FullText())
	}
})
