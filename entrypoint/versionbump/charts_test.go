package versionbump

import (
	"fmt"

	"github.com/rancher/distros-test-framework/pkg/assert"
	. "github.com/rancher/distros-test-framework/pkg/customflag"
	. "github.com/rancher/distros-test-framework/pkg/template"
	"github.com/rancher/distros-test-framework/pkg/testcase"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/ginkgo/v2"
)

// const (
// 	calico             = getCharts + "rke2-calico* | grep rke2-calico "
// 	canal              = getCharts + "rke2-canal.yaml | grep rke2-canal "
// 	coredns            = getCharts + "rke2-coredns.yaml | grep rke2-coredns "
// 	cilium             = getCharts + "rke2-cilium.yaml | grep rke2-cilium "
// 	flannel            = getCharts + "rke2-flannel.yaml | grep rke2-flannel "
// 	ingressController  = getCharts + "rke2-ingress-nginx.yaml | grep rke2-ingress-nginx "
// 	metricsServer      = getCharts + "rke2-metrics-server.yaml | grep rke2-metrics-server "
// 	multus             = getCharts + "rke2-multus.yaml | grep rke2-multus "
// 	runtimeClasses     = getCharts + "rke2-runtimeclasses.yaml | grep rke2-runtimeclasses "
// 	snapshotController = getCharts + "rke2-snapshot-controller* | grep rke2-snapshot "
// 	snapshotValidation = getCharts + "rke2-snapshot-validation-webhook.yaml | grep rke2-snapshot-validation-webhook "
// 	traefik            = getCharts + "rke2-traefik* | grep rke2-traefik "
// 	harvesterCloud     = getCharts + "/harvester-cloud-provider.yaml | grep cloud-provider "
// 	harvesterCsi       = getCharts + "/harvester-csi-driver.yaml | grep csi "
// 	rancherVsphereCpi  = getCharts + "/rancher-vsphere-cpi.yaml | grep cpi "
// 	rancherVsphereCsi   = getCharts + "/rancher-vsphere-csi.yaml | grep csi "
// )

var _ = Describe("Components Version Upgrade:", func() {
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

	// It("Verifies Charts Versions", func() {
	// 	testcase.TestChartsVersions(true, true)
	// })

	locationCharts, err := shared.RunCommandOnNode("find /var/lib/rancher/rke2/data/v*", cluster.ServerIPs[0])
	if err != nil {
		fmt.Print("failed to get location of charts versions")
		return
	}
	getCharts := fmt.Sprintf("sudo cat %s/charts", locationCharts)

	calico := fmt.Sprintf("%s/rke2-calico* | grep rke2-calico ", getCharts)
	canal := fmt.Sprintf("%s/rke2-canal.yaml | grep rke2-canal ", getCharts)
	coredns := fmt.Sprintf("%s/rke2-coredns.yaml | grep rke2-coredns ", getCharts)
	cilium := fmt.Sprintf("%s/rke2-cilium.yaml | grep rke2-cilium ", getCharts)
	flannel := fmt.Sprintf("%s/rke2-flannel.yaml | grep rke2-flannel ", getCharts)
	ingressController := fmt.Sprintf("%s/rke2-ingress-nginx.yaml | grep rke2-ingress-nginx ", getCharts)
	metricsServer := fmt.Sprintf("%s/rke2-metrics-server.yaml | grep rke2-metrics-server ", getCharts)
	multus := fmt.Sprintf("%s/rke2-multus.yaml | grep rke2-multus ", getCharts)
	runtimeClasses := fmt.Sprintf("%s/rke2-runtimeclasses.yaml | grep rke2-runtimeclasses ", getCharts)
	snapshotController := fmt.Sprintf("%s/rke2-snapshot-controller* | grep rke2-snapshot ", getCharts)
	snapshotValidation := fmt.Sprintf("%s/rke2-snapshot-validation-webhook.yaml | grep rke2-snapshot ", getCharts)
	traefik := fmt.Sprintf("%s/rke2-traefik* | grep rke2-traefik ", getCharts)
	harvesterCloud := fmt.Sprintf("%s/harvester-cloud-provider.yaml | grep cloud-provider ", getCharts)
	harvesterCsi := fmt.Sprintf("%s/harvester-csi-driver.yaml | grep csi ", getCharts)
	rancherVsphereCpi := fmt.Sprintf("%s/rancher-vsphere-cpi.yaml | grep cpi ", getCharts)
	rancherVsphereCsi := fmt.Sprintf("%s/rancher-vsphere-csi.yaml | grep csi ", getCharts)

	// runc := fmt.Sprintf("(find /var/lib/rancher/%s/data/ -type f -name runc -exec {} --version \\;) , ", cluster.Config.Product)
	// crictl := "sudo /var/lib/rancher/rke2/bin/crictl -v, "

	// test decription and cmds generated based on product rke2
	// coredns := getCharts + " : | grep 'hardened-coredns' -A1, "
	// etcd := getCharts + " : | grep 'hardened-etcd' -A1, "
	// cniPlugins := "sudo /var/lib/rancher/rke2/bin/crictl -r unix:///run/k3s/containerd/containerd.sock images : | grep 'cni-plugins' , "
	description := "Verifies chart versions for several components on rke2:\n1-calico" +
		"\n2-canal\n3-cilium\n4-coredns\n5-flannel\n6-ingress Controller\n7-metrics Server" +
		"\n8-multus\n9-runtime Classes\n10-snapshot Controller\n11-snapshot Validation Webhook" +
		"\n12-traefik\n13-harvester Cloud Provider\n14-harvester Csi Driver" +
		"\n15-rancher Vsphere Cpi\n16-rancher Vsphere Csi"

	cmd := calico + canal + cilium + coredns + flannel + ingressController + metricsServer +
		multus + runtimeClasses + snapshotController + snapshotValidation + traefik +
		harvesterCloud + harvesterCsi + rancherVsphereCpi + rancherVsphereCsi

	// test decription and cmds updated based on product k3s
	// if cluster.Config.Product == "k3s" {
	// 	crictl = "sudo /usr/local/bin/crictl -v, "
	// 	cniPlugins = "/var/lib/rancher/k3s/data/current/bin/cni, "
	// 	coredns = getCharts + " : | grep 'mirrored-coredns' -A1, "
	// 	etcd = "sudo journalctl -u k3s | grep etcd-version, "
	// 	description = "Verifies bump versions for several components on k3s:\n1-coredns" +
	// 		"\n2-metrics Server\n3-etcd\n4-cni Plugins\n5-containerd\n6-runc\n7-crictl\n8-traefik\n9-local path provisioner\n10-klipper LB"

	// 	cmd = coredns + metricsServer + etcd + cniPlugins + containerd + runc + crictl + traefik + localPath + klipperLB
	// }

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
