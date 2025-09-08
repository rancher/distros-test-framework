package validatecluster

import (
	"fmt"

	"github.com/rancher/distros-test-framework/internal/pkg/assert"
	"github.com/rancher/distros-test-framework/internal/pkg/customflag"
	"github.com/rancher/distros-test-framework/internal/pkg/testcase"

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

	It("Validate Metrics Server", func() {
		testcase.TestNodeMetricsServer(true, true)
	})

	It("Verifies ClusterIP Service", func() {
		testcase.TestServiceClusterIP(true, true)
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
		testcase.TestDNSAccess(true, true)
	})

	if cluster.Config.Product == "k3s" {
		It("Verifies Local Path Provisioner storage", func() {
			testcase.TestLocalPathProvisionerStorage(cluster, true, true)
		})

		It("Verifies LoadBalancer Service", func() {
			testcase.TestServiceLoadBalancer(true, true)
		})

		// TODO: Remove when v1.32 is the minimum supported version
		It("Verifies Traefik IngressRoute using old GKV", func() {
			testcase.TestIngressRoute(cluster, true, true, "traefik.containo.us/v1alpha1")
		})

		It("Verifies Traefik IngressRoute using new GKV", func() {
			testcase.TestIngressRoute(cluster, true, true, "traefik.io/v1alpha1")
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
