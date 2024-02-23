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
	flannelRke2       = "kubectl get node -o yaml : | grep 'hardened-flannel' -A1, "
	flannelK3s        = "/var/lib/rancher/k3s/data/current/bin/flannel, "
	calico            = "kubectl get node -o yaml : | grep 'hardened-calico' -A1, "
	ingressController = "kubectl get node -o yaml : | grep 'nginx-ingress-controller' -A1, "
	corednsRke2       = "kubectl get node -o yaml : | grep 'hardened-coredns' -A1, "
	coreDnsk3s        = "kubectl get node -o yaml : | grep 'mirrored-coredns' -A1, "
	metricsServer     = "kubectl get node -o yaml : | grep 'metrics-server' -A1, "
	etcdRke2          = "kubectl get node -o yaml : | grep 'hardened-etcd' -A1, "
	containerd        = "kubectl get node -o yaml : | grep  containerd, "
	cniPlugins        = "/var/lib/rancher/k3s/data/current/bin/cni, "
	traefik           = "kubectl get node -o yaml : | grep traefik  -A1, "
	localPath         = "kubectl get node -o yaml : | grep local-path -A1, "
	etcdK3s           = `sudo journalctl -u k3s | grep etcd-version,`
	klipperLB         = "kubectl get node -o yaml : | grep klipper -A5, "
)

var _ = Describe("Components Version Upgrade:", func() {
	It("Start Up with no issues", func() {
		testcase.TestBuildCluster(GinkgoT())
	})

	It("Validate Node", func() {
		testcase.TestNodeStatus(
			assert.NodeAssertReadyStatus(),
			nil)
	})

	It("Validate Pod", func() {
		testcase.TestPodStatus(
			assert.PodAssertRestart(),
			assert.PodAssertReady(),
			assert.PodAssertStatus())
	})

	runc := fmt.Sprintf("(find /var/lib/rancher/%s/data/ -type f -name runc -exec {} --version \\;)", cfg.Product)
	description := "Verifies bump versions for several components on Rke2:\n1-canal\n2-flannel\n" +
		"3-calico\n4-ingressController\n5-coredns\n6-metricsServer\n7-etcd\n8-containerd\n9-runc"
	cmd := flannelRke2 + calico + ingressController + corednsRke2 + metricsServer + etcdRke2 + containerd + runc

	if cfg.Product == "k3s" {
		description = "Verifies bump versions for several components on k3s:\n1-flannel\n2-coredns\n3-metricsServer\n" +
			"4-etcd\n5-cni plugins\n6-traefik\n7-local path storage\n8-containerd\n9-Klipper\n10-runc"
		cmd = flannelK3s + coreDnsk3s + metricsServer + etcdK3s + cniPlugins + traefik + localPath + containerd + klipperLB + runc
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

	It("Verifies Ingress", func() {
		testcase.TestIngress(true, false)
	})

	It("Verifies dns access", func() {
		testcase.TestDnsAccess(true, false)
	})

	if cfg.Product == "rke2" {
		It("Update Config YAML for multus+canal and get bumps", func() {
			Template(TestTemplate{
				TestCombination: &RunCmd{
					Run: []TestMap{
						{
							Cmd:           "kubectl get node -o yaml : | grep multus-cni -A1",
							ExpectedValue: "hardened-multus ",
						},
						{
							Cmd: "sudo sed -i '/cni:/d' /etc/rancher/rke2/config.yaml && " +
								" sudo sed -i '/multus/d' /etc/rancher/rke2/config.yaml && sudo sed -i '/canal/d' /etc/rancher/rke2/config.yaml " +
								" && echo -e \"cni: cilium\" | sudo tee -a /etc/rancher/rke2/config.yaml " +
								" && cat /etc/rancher/rke2/config.yaml",
							ExpectedValue: "cilium",
						},
						{
							Cmd:           "sudo sed -i '/^$/d' /etc/rancher/rke2/config.yaml && cat /etc/rancher/rke2/config.yaml && sleep 20",
							ExpectedValue: "cilium",
						},
						{
							Cmd:           "sudo systemctl restart rke2-server && if [ $? -eq 0 ]; then echo \"ok\"; fi",
							ExpectedValue: "ok",
						},
						{
							Cmd:           "sleep 10 && if [ $? -eq 0 ]; then echo \"ok\"; fi",
							ExpectedValue: "ok",
						},
						{
							Cmd:           "kubectl get node -o yaml : | grep mirrored-cilium  -A1",
							ExpectedValue: "mirrored-cilium ",
						},
						{
							Cmd:           "kubectl get node -o yaml : | grep hardened-cni-plugins -A1",
							ExpectedValue: "hardened-cni-plugins ",
						},
					},
				},
				DebugMode: customflag.ServiceFlag.TestConfig.DebugMode,
			})
		})
	}

	if cfg.Product == "k3s" {
		It("Verifies Local Path Provisioner storage", func() {
			testcase.TestLocalPathProvisionerStorage(true, true)
		})

		It("Verifies LoadBalancer Service", func() {
			testcase.TestServiceLoadBalancer(true, true)
		})
	}

	It("Validate ETCD health after all bumps", func() {
		healthCheck := fmt.Sprintf("sudo  ETCDCTL_API=3 /usr/local/bin/etcdctl  --cert=/var/lib/rancher/%s/server/tls/etcd/server-client.crt"+
			" --key=/var/lib/rancher/%s/server/tls/etcd/server-client.key "+
			" --cacert=/var/lib/rancher/%s/server/tls/etcd/server-ca.crt endpoint health", cfg.Product, cfg.Product, cfg.Product)
		Template(TestTemplate{
			TestCombination: &RunCmd{
				Run: []TestMap{
					{
						Cmd:           healthCheck,
						ExpectedValue: "is healthy: successfully ",
					},
				},
			},
			DebugMode: customflag.ServiceFlag.TestConfig.DebugMode,
		})
	})

	It("Print results", func() {
		assert.PrintResults()
	})
})

var _ = AfterEach(func() {
	if CurrentSpecReport().Failed() {
		fmt.Printf("\nFAILED! %s\n", CurrentSpecReport().FullText())
	} else {
		fmt.Printf("\nPASSED! %s\n", CurrentSpecReport().FullText())
	}
})
