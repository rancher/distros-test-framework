//go:build upgradesuc

package upgradecluster

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rancher/distros-test-framework/pkg/assert"
	"github.com/rancher/distros-test-framework/pkg/testcase"
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

	if cluster.Config.Product == "rke2" {
		It("Verifies Snapshot Webhook pre-upgrade", func() {
			err := testcase.TestSnapshotWebhook(true)
			Expect(err).To(HaveOccurred())
		})
	}

	if cluster.Config.Product == "k3s" {
		It("Verifies LoadBalancer Service before upgrade", func() {
			testcase.TestServiceLoadBalancer(true, false)
		})

		It("Verifies Local Path Provisioner storage before upgrade", func() {
			testcase.TestLocalPathProvisionerStorage(cluster, true, false)
		})

		It("Verifies Traefik IngressRoute before upgrade using old GKV", func() {
			testcase.TestIngressRoute(cluster, true, false, "traefik.containo.us/v1alpha1")
		})
	}

	It("\nUpgrade via SUC", func() {
		_ = testcase.TestUpgradeClusterSUC(cluster, flags.SUCUpgradeVersion.String())
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

	It("Verifies ClusterIP Service post-upgrade", func() {
		testcase.TestServiceClusterIP(false, true)
	})

	It("Verifies NodePort Service post-upgrade", func() {
		testcase.TestServiceNodePort(false, true)
	})

	It("Verifies Daemonset post-upgrade", func() {
		testcase.TestDaemonset(false, true)
	})

	It("Verifies DNS Access post-upgrade", func() {
		testcase.TestDNSAccess(true, true)
	})

	if cluster.Config.Product == "rke2" {
		It("Verifies Snapshot Webhook after upgrade", func() {
			err := testcase.TestSnapshotWebhook(true)
			Expect(err).To(HaveOccurred())
		})
	}

	if cluster.Config.Product == "k3s" {
		It("Verifies LoadBalancer Service after upgrade", func() {
			testcase.TestServiceLoadBalancer(false, true)
		})

		It("Verifies Local Path Provisioner storage after upgrade", func() {
			testcase.TestLocalPathProvisionerStorage(cluster, false, true)
		})

		It("Verifies Traefik IngressRoute after upgrade using old GKV", func() {
			testcase.TestIngressRoute(cluster, false, true, "traefik.containo.us/v1alpha1")
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
