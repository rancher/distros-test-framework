//go:build upgrademanual

package upgradecluster

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

	It("Validate Node", func() {
		testcase.TestNodeStatus(
			assert.NodeAssertReadyStatus(),
			nil,
		)
	})

	It("Validate Pod", func() {
		testcase.TestPodStatus(
			assert.PodAssertRestart(),
			assert.PodAssertReady(),
			assert.PodAssertStatus(),
		)
	})

	It("Verifies ClusterIP Service pre-upgrade", func() {
		testcase.TestServiceClusterIp(true, false)
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
		testcase.TestDnsAccess(true, false)
	})

	if cfg.Product == "rke2" {
		It("Verifies Snapshot Webhook pre-upgrade", func() {
			err := testcase.TestSnapshotWebhook(true)
			Expect(err).To(HaveOccurred())
		})
	}

	if cfg.Product == "k3s" {
		It("Verifies LoadBalancer Service pre-upgrade", func() {
			testcase.TestServiceLoadBalancer(true, false)
		})

		It("Verifies Local Path Provisioner storage pre-upgrade", func() {
			testcase.TestLocalPathProvisionerStorage(true, false)
		})

		It("Verifies Traefik IngressRoute before upgrade using old GKV", func() {
			testcase.TestIngressRoute(true, false, "traefik.containo.us/v1alpha1")
		})
	}

	It("Upgrade Manual", func() {
		err := testcase.TestUpgradeClusterManually(customflag.ServiceFlag.InstallMode.String())
		Expect(err).NotTo(HaveOccurred())
	})

	It("Checks Node Status after upgrade and validate version", func() {
		testcase.TestNodeStatus(
			assert.NodeAssertReadyStatus(),
			assert.NodeAssertVersionTypeUpgrade(customflag.ServiceFlag),
		)
	})

	It("Checks Pod Status after upgrade", func() {
		testcase.TestPodStatus(
			assert.PodAssertRestart(),
			assert.PodAssertReady(),
			assert.PodAssertStatus(),
		)
	})

	It("Verifies ClusterIP Service after upgrade", func() {
		testcase.TestServiceClusterIp(false, true)
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
		testcase.TestDnsAccess(false, true)
	})

	if cfg.Product == "rke2" {
		It("Verifies Snapshot Webhook after upgrade", func() {
			err := testcase.TestSnapshotWebhook(true)
			Expect(err).To(HaveOccurred())
		})
	}

	if cfg.Product == "k3s" {
		It("Verifies LoadBalancer Service after upgrade", func() {
			testcase.TestServiceLoadBalancer(false, true)
		})

		It("Verifies Local Path Provisioner storage after upgrade", func() {
			testcase.TestLocalPathProvisionerStorage(false, true)
		})

		It("Verifies Traefik IngressRoute after upgrade using old GKV", func() {
			testcase.TestIngressRoute(false, true, "traefik.containo.us/v1alpha1")
		})
	}
})

var _ = AfterEach(func() {
	if CurrentSpecReport().Failed() {
		fmt.Printf("\nFAILED! %s\n", CurrentSpecReport().FullText())
	} else {
		fmt.Printf("\nPASSED! %s\n", CurrentSpecReport().FullText())
	}
})
