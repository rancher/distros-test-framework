//go:build upgradesuc

package upgradecluster

import (
	"fmt"

	"github.com/rancher/distros-test-framework/pkg/assert"
	"github.com/rancher/distros-test-framework/pkg/testcase"

	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("SUC Upgrade Tests:", func() {
	It("Starts up with no issues", func() {
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

	It("Validate Metrics Server pre-upgrade", func() {
		testcase.TestNodeMetricsServer(true, true)
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

	It("Verifies DNS Access pre-upgrade", func() {
		testcase.TestDNSAccess(true, false)
	})

	if cluster.Config.Product == "k3s" {
		It("Verifies LoadBalancer Service pre-upgrade", func() {
			testcase.TestServiceLoadBalancer(true, false)
		})

		It("Verifies Local Path Provisioner storage pre-upgrade", func() {
			testcase.TestLocalPathProvisionerStorage(cluster, true, false)
		})

		It("Verifies Traefik IngressRoute using new GKV pre-upgrade", func() {
			testcase.TestIngressRoute(cluster, true, false, "traefik.io/v1alpha1")
		})
	}

	It("\nUpgrade via SUC", func() {
		_ = testcase.TestUpgradeClusterSUC(cluster, k8sClient, flags.SUCUpgradeVersion.String())
	})

	It("Checks Node status post-upgrade", func() {
		testcase.TestNodeStatus(
			cluster,
			assert.NodeAssertReadyStatus(),
			assert.NodeAssertVersionUpgraded(),
		)
	})

	It("Checks Pod status post-upgrade", func() {
		testcase.TestPodStatus(
			cluster,
			nil,
			assert.PodAssertReady())
	})

	It("Validate Metrics Server post-upgrade", func() {
		testcase.TestNodeMetricsServer(true, true)
	})

	It("Verifies ClusterIP Service post-upgrade", func() {
		testcase.TestServiceClusterIP(false, true)
	})

	It("Verifies NodePort Service post-upgrade", func() {
		testcase.TestServiceNodePort(false, true)
	})

	It("Verifies Ingress post-upgrade", func() {
		testcase.TestIngress(false, true)
	})

	It("Verifies Daemonset post-upgrade", func() {
		testcase.TestDaemonset(false, true)
	})

	It("Verifies DNS Access post-upgrade", func() {
		testcase.TestDNSAccess(true, true)
	})

	if cluster.Config.Product == "k3s" {
		It("Verifies LoadBalancer Service post-upgrade", func() {
			testcase.TestServiceLoadBalancer(false, true)
		})

		It("Verifies Local Path Provisioner storage post-upgrade", func() {
			testcase.TestLocalPathProvisionerStorage(cluster, false, true)
		})

		It("Verifies Traefik IngressRoute using new GKV post-upgrade", func() {
			testcase.TestIngressRoute(cluster, false, true, "traefik.io/v1alpha1")
		})
	}

		if customflag.ServiceFlag.SelinuxTest {
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
	}
})

var _ = AfterEach(func() {
	if CurrentSpecReport().Failed() {
		fmt.Printf("\nFAILED! %s\n\n", CurrentSpecReport().FullText())
	} else {
		fmt.Printf("\nPASSED! %s\n\n", CurrentSpecReport().FullText())
	}
})
