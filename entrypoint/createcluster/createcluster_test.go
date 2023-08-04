package createcluster

import (
	"fmt"

	"github.com/rancher/distros-test-framework/pkg/assert"
	"github.com/rancher/distros-test-framework/pkg/customflag"
	"github.com/rancher/distros-test-framework/pkg/testcase"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/ginkgo/v2"
)

var arch = customflag.ServiceFlag.ClusterConfig.Arch.String()

var _ = Describe("Test:", func() {
	It("Start Up with no issues", func() {
		testcase.TestBuildCluster(GinkgoT())
	})

	It("Validate Nodes", func() {
		testcase.TestNodeStatus(
			assert.NodeAssertReadyStatus(),
			nil,
		)
	})

	It("Validate Pods", func() {
		testcase.TestPodStatus(
			assert.PodAssertRestart(),
			assert.PodAssertReady(),
			assert.PodAssertStatus(),
		)
	})

	It("Verifies ClusterIP Service", func() {
		testcase.TestServiceClusterIp(true)
		shared.ManageWorkload("delete", arch, "clusterip.yaml")
	})

	It("Verifies NodePort Service", func() {
		testcase.TestServiceNodePort(true)
		shared.ManageWorkload("delete", arch, "nodeport.yaml")
	})

	It("Verifies Ingress", func() {
		testcase.TestIngress(true)
		shared.ManageWorkload("delete", arch, "ingress.yaml")
	})

	It("Verifies Daemonset", func() {
		testcase.TestDaemonset(true)
		shared.ManageWorkload("delete", arch, "daemonset.yaml")
	})

	It("Verifies dns access", func() {
		testcase.TestDnsAccess(true)
		shared.ManageWorkload("delete", arch, "dnsutils.yaml")
	})

	if cfg.Product == "k3s" {
		It("Verifies Local Path Provisioner storage", func() {
			testcase.TestLocalPathProvisionerStorage(true)
			shared.ManageWorkload("delete", arch, "local-path-provisioner.yaml")
		})
		It("Verifies LoadBalancer Service", func() {
			testcase.TestServiceLoadBalancer(true)
			shared.ManageWorkload("delete", arch, "loadbalancer.yaml")
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
