//go:build components

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
	kgn               = "kubectl get node -o yaml"
	metricsServer     = kgn + " : | grep 'metrics-server' -A1, "
	containerd        = kgn + " : | grep containerd -A1, "
	localPath         = kgn + " : | grep local-path -A1, "
	traefik           = kgn + " : | grep traefik  -A1, "
	klipperLB         = kgn + " : | grep klipper -A5"
	ingressController = kgn + " : | grep 'nginx-ingress-controller' -A1"
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
			assert.PodAssertReady())
	})

	runc := fmt.Sprintf("(find /var/lib/rancher/%s/data/ -type f -name runc -exec {} --version \\;) , ", cluster.Config.Product)
	crictl := "sudo /var/lib/rancher/rke2/bin/crictl -v, "

	// test decription and cmds generated based on product rke2
	coredns := kgn + " : | grep 'hardened-coredns' -A1, "
	etcd := kgn + " : | grep 'hardened-etcd' -A1, "
	cniPlugins := "sudo /var/lib/rancher/rke2/bin/crictl -r unix:///run/k3s/containerd/containerd.sock images : | grep 'cni-plugins' , "
	description := "Verifies bump versions for several components on rke2:\n1-coredns" +
		"\n2-metrics Server\n3-etcd\n4-containerd\n5-runc\n6-crictl\n7-ingress Controller"

	cmd := coredns + metricsServer + etcd + containerd + runc + crictl + ingressController

	// test decription and cmds updated based on product k3s
	if cluster.Config.Product == "k3s" {
		crictl = "sudo /usr/local/bin/crictl -v, "
		cniPlugins = "/var/lib/rancher/k3s/data/current/bin/cni, "
		coredns = kgn + " : | grep 'mirrored-coredns' -A1, "
		etcd = "sudo journalctl -u k3s | grep etcd-version, "
		description = "Verifies bump versions for several components on k3s:\n1-coredns" +
			"\n2-metrics Server\n3-etcd\n4-cni Plugins\n5-containerd\n6-runc\n7-crictl\n8-traefik\n9-local path provisioner\n10-klipper LB"

		cmd = coredns + metricsServer + etcd + cniPlugins + containerd + runc + crictl + traefik + localPath + klipperLB
	}

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
