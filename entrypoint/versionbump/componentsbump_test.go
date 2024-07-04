//go:build components

package versionbump

import (
	"fmt"

	"github.com/rancher/distros-test-framework/pkg/assert"
	"github.com/rancher/distros-test-framework/pkg/customflag"
	. "github.com/rancher/distros-test-framework/pkg/template"
	"github.com/rancher/distros-test-framework/pkg/testcase"

	. "github.com/onsi/ginkgo/v2"
)

const (
	kgn               = "kubectl get node -o yaml"
	canalFlannel      = kgn + " : | grep 'hardened-flannel' -A1, "
	calico            = kgn + " : | grep 'hardened-calico' -A1, "
	ingressController = kgn + " : | grep 'nginx-ingress-controller' -A1, "
	metricsServer     = kgn + " : | grep 'metrics-server' -A1, "
	containerd        = kgn + " : | grep containerd, "
	traefik           = kgn + " : | grep traefik  -A1, "
	localPath         = kgn + " : | grep local-path -A1, "
	klipperLB         = kgn + " : | grep klipper -A5, "
	cniPlugins        = "/var/lib/rancher/k3s/data/current/bin/cni, "
)

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
			assert.PodAssertReady(),
			assert.PodAssertStatus())
	})

	runc := fmt.Sprintf("(find /var/lib/rancher/%s/data/ -type f -name runc -exec {} --version \\;)", cluster.Config.Product)

	// test decription and cmds generated based on product rke2
	coredns := kgn + " : | grep 'hardened-coredns' -A1, "
	etcd := kgn + " : | grep 'hardened-etcd' -A1, "
	description := "Verifies bump versions for several components on rke2:\n1-canal(flannel)\n2-calico" +
		"\n3-ingressController\n4-coredns\n5-metricsServer\n6-etcd\n7-containerd\n8-runc"
	cmd := canalFlannel + calico + ingressController + coredns + metricsServer + etcd + containerd + runc

	// test decription and cmds updated based on product k3s
	if cluster.Config.Product == "k3s" {
		coredns = kgn + " : | grep 'mirrored-coredns' -A1, "
		etcd = "sudo journalctl -u k3s | grep etcd-version, "
		description = "Verifies bump versions for several components on k3s:\n1-coredns\n2-metricsServer" +
			"\n3-etcd\n4-cni plugins\n5-traefik\n6-local path storage\n7-containerd\n8-Klipper\n9-runc"
		cmd = coredns + metricsServer + etcd + cniPlugins + traefik + localPath + containerd + klipperLB + runc
	}

	It(description, func() {
		Template(TestTemplate{
			TestCombination: &RunCmd{
				Run: []TestMap{
					{
						Cmd:                  cmd,
						ExpectedValue:        TestMapTemplate.ExpectedValue,
						ExpectedValueUpgrade: TestMapTemplate.ExpectedValueUpgrade,
					},
				},
			},
			InstallMode: customflag.ServiceFlag.InstallMode.String(),
			Description: customflag.ServiceFlag.TestConfig.Description,
			DebugMode:   customflag.ServiceFlag.TestConfig.DebugMode,
		})
	})

	It("Verifies dns access", func() {
		testcase.TestDnsAccess(true, true)
	})

	It("Verifies ClusterIP Service", func() {
		testcase.TestServiceClusterIp(true, true)
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
		Template(TestTemplate{
			TestCombination: &RunCmd{
				Run: []TestMap{
					{
						Cmd:                  "kubectl top node : | grep 'CPU(cores)' -A1, kubectl top pods -A : | grep 'CPU(cores)' -A1",
						ExpectedValue:        "CPU,MEMORY",
						ExpectedValueUpgrade: "CPU,MEMORY",
					},
				},
			},
		})
	})
})

var _ = AfterEach(func() {
	if CurrentSpecReport().Failed() {
		fmt.Printf("\nFAILED! %s\n", CurrentSpecReport().FullText())
	} else {
		fmt.Printf("\nPASSED! %s\n", CurrentSpecReport().FullText())
	}
})
