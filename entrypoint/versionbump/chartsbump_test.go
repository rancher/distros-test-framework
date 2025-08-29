//go:build chartsbump

package versionbump

import (
	"fmt"

	"github.com/rancher/distros-test-framework/pkg/assert"
	. "github.com/rancher/distros-test-framework/pkg/customflag"
	. "github.com/rancher/distros-test-framework/pkg/template"
	"github.com/rancher/distros-test-framework/pkg/testcase"

	. "github.com/onsi/ginkgo/v2"
)

const (
	getCharts          = "sudo cat /var/lib/rancher/rke2/data/v*/charts/*"
	calico             = getCharts + " | grep 'rke2-calico', "
	canal              = getCharts + " | grep 'rke2-canal', "
	coredns            = getCharts + " | grep 'rke2-coredns', "
	cilium             = getCharts + " | grep 'rke2-cilium', "
	flannel            = getCharts + " | grep 'rke2-flannel', "
	ingressController  = getCharts + " | grep 'rke2-ingress-nginx', "
	metrics            = getCharts + " | grep 'rke2-metrics-server', "
	multus             = getCharts + " | grep 'rke2-multus', "
	runtimeClasses     = getCharts + " | grep 'rke2-runtimeclasses', "
	snapshotController = getCharts + " | grep 'rke2-snapshot-controller', "
	snapshotValidation = getCharts + " | grep 'rke2-snapshot-validation-webhook', "
	traefik            = getCharts + " | grep 'rke2-traefik', "
	harvesterCloud     = getCharts + " | grep 'harvester-cloud-provider', "
	harvesterCsi       = getCharts + " | grep 'harvester-csi-driver', "
	rancherVsphereCpi  = getCharts + " | grep 'vsphere-cpi', "
	rancherVsphereCsi  = getCharts + " | grep 'vsphere-csi' "
)

var _ = Describe("Charts Version Upgrade:", func() {
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

	description := "Verifies chart versions for several components on rke2:\n1-calico" +
		"\n2-canal\n3-cilium\n4-coredns\n5-flannel\n6-ingress Controller\n7-metrics Server" +
		"\n8-multus\n9-runtime Classes\n10-snapshot Controller\n11-snapshot Validation Webhook" +
		"\n12-traefik\n13-harvester Cloud Provider\n14-harvester Csi Driver" +
		"\n15-rancher Vsphere Cpi\n16-rancher Vsphere Csi"

	cmd := calico + canal + cilium + coredns + flannel + ingressController + metrics +
		multus + runtimeClasses + snapshotController + snapshotValidation + traefik +
		harvesterCloud + harvesterCsi + rancherVsphereCpi + rancherVsphereCsi

	It(description, func() {
		Template(TestTemplate{
			TestCombination: &RunCmd{
				Run: []TestMapConfig{
					{
						Cmd:                  cmd,
						ExpectedValue:        TestMap.ExpectedValue,
						ExpectedValueUpgrade: TestMap.ExpectedValueUpgrade,
					},
				},
			},
			InstallMode: ServiceFlag.InstallMode.String(),
			Description: ServiceFlag.TestTemplateConfig.Description,
		})
	})

	It("Verifies dns access", func() {
		testcase.TestDNSAccess(true, true)
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

	if cluster.Config.Product == "k3s" {
		It("Verifies Local Path Provisioner storage", func() {
			testcase.TestLocalPathProvisionerStorage(cluster, true, true)
		})

		It("Verifies LoadBalancer Service", func() {
			testcase.TestServiceLoadBalancer(true, true)
		})
	}

	It("Verifies top node and pods", func() {
		TestMap.Cmd = "kubectl top node : | grep 'CPU(cores)' -A1, kubectl top pods -A : | grep 'CPU(cores)' -A1"
		TestMap.ExpectedValue = "CPU,MEMORY"
		TestMap.ExpectedValueUpgrade = "CPU,MEMORY"
		Template(TestTemplate{
			TestCombination: &RunCmd{
				Run: []TestMapConfig{
					{
						Cmd:                  TestMap.Cmd,
						ExpectedValue:        TestMap.ExpectedValue,
						ExpectedValueUpgrade: TestMap.ExpectedValueUpgrade,
					},
				},
			},
		})
	})
})

var _ = AfterEach(func() {
	if CurrentSpecReport().Failed() {
		fmt.Printf("\nFAILED! %s\n\n", CurrentSpecReport().FullText())
	} else {
		fmt.Printf("\nPASSED! %s\n\n", CurrentSpecReport().FullText())
	}
})
