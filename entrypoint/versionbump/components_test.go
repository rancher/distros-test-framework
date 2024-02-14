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
	flannel           = "kubectl get node -o yaml : | grep flannel, "
	calico            = "kubectl get node -o yaml : | grep calico, "
	ingressController = "kubectl get node -o yaml : | grep nginx-ingress-controller, "
	coredns           = "kubectl get node -o yaml : | grep coredns | awk '{print $7 }', "
	metricsServer     = "kubectl get node -o yaml : | grep metrics-server, "
	etcdRke2          = "kubectl get node -o yaml : | grep etcd, "
	containerd        = "kubectl get node -o yaml : | grep  containerd, "
	etcdK3S           = "sudo journalctl -u k3s | grep 'etcd-version' | awk -F'\"' " +
		"'{ for(i=1; i<=NF; ++i) if($i == \"etcd-version\") print $(i+2) }', "
	cniPlugins = " var/lib/rancher/k3s/data/current/bin/cni |  awk '{print $1; exit}' , "
	traefik    = "kubectl get node -o yaml : | grep traefik, "
	localPath  = "kubectl get node -o yaml : | grep local-path, "
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
	description := "Verifies bump versions for several components on Rke2 in this order:\n1-canal\n2-flannel\n" +
		"3-calico\n4-ingressController\n5-coredns\n6-metricsServer\n7-etcd\n8-containerd\n9-runc"
	cmd := flannel + calico + ingressController + coredns + metricsServer + etcdRke2 + containerd + runc

	if cfg.Product == "k3s" {
		description = "Verifies bump versions for several components on k3s in this order:\n1-flannel\n2-coredns\n3-metricsServer\n" +
			"4-etcd\n5-containerd\n6-cni plugins\n7-traefik\n8-local path storage\n9-runc"
		cmd = flannel + coredns + metricsServer + etcdK3S + containerd + cniPlugins + traefik + localPath + runc
	}

	It(description, func() {
		Template(TestTemplate{
			TestCombination: &RunCmd{
				Run: []TestMap{
					{
						Cmd:                  cmd,
						ExpectedValue:        TestMapTemplate.ExpectedValue,
						ExpectedValueUpgrade: TestMapTemplate.ExpectedValueUpgrade,
						SkipValidation:       true,
					},
				},
			},
			InstallMode: customflag.ServiceFlag.InstallMode.String(),
			TestConfig: &TestConfig{
				TestFunc:       ConvertToTestCase(customflag.ServiceFlag.TestConfig.TestFuncs),
				ApplyWorkload:  customflag.ServiceFlag.TestConfig.ApplyWorkload,
				DeleteWorkload: customflag.ServiceFlag.TestConfig.DeleteWorkload,
				WorkloadName:   customflag.ServiceFlag.TestConfig.WorkloadName,
			},
			Description: customflag.ServiceFlag.TestConfig.Description,
		})
	})

	It("Verifies Ingress", func() {
		testcase.TestIngress(true, true)
	})

	It("Verifies dns access", func() {
		testcase.TestDnsAccess(true, true)
	})

	if cfg.Product == "k3s" {
		It("Verifies Local Path Provisioner storage", func() {
			testcase.TestLocalPathProvisionerStorage(true, true)
		})

		It("Verifies LoadBalancer Service", func() {
			testcase.TestServiceLoadBalancer(true, true)
		})
	}

	It("Validate ETCD health", func() {
		healthCheck := fmt.Sprintf("sudo etcdctl --cert=/var/lib/rancher/%s/server/tls/etcd/server-client.crt"+
			" --key=/var/lib/rancher/%s/server/tls/etcd/server-client.key "+
			" --cacert=/var/lib/rancher/%s/server/tls/etcd/server-ca.crt endpoint health", cfg.Product, cfg.Product, cfg.Product)
		Template(TestTemplate{
			TestCombination: &RunCmd{
				Run: []TestMap{
					{
						Cmd:                  healthCheck,
						ExpectedValue:        "is healthy: successfully ",
						ExpectedValueUpgrade: "is healthy: successfully ",
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
