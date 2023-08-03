package testcase

import (
	"github.com/rancher/distros-test-framework/pkg/assert"
	"github.com/rancher/distros-test-framework/factory"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/ginkgo/v2"
)


// Validate that containerd is running with selinux enabled in the config
func TestSelinuxEnabled(product string) {
	ips := shared.FetchNodeExternalIP()
	selinuxConfigAssert := "selinux: true"
	selinuxContainerdAssert := "enable_selinux = true"

	for _, ip := range ips {
		assert.CheckComponentCmdNode("cat /etc/rancher/"+product+"/config.yaml", ip, selinuxConfigAssert)
		assert.CheckComponentCmdNode("sudo cat /var/lib/rancher/"+product+"/agent/etc/containerd/config.toml", ip, selinuxContainerdAssert)
	}
}

// Validate container-selinux version, rke2-selinux version and rke2-selinux version
func TestSelinuxVersions(product string) {
	cluster := factory.GetCluster(GinkgoT())

	serverAsserts := []string{"container-selinux", product + "-selinux", product + "-server"}
	agentAsserts := []string{"container-selinux", product + "-selinux"}

	if cluster.NumServers > 0 {
		for _, serverIP := range cluster.ServerIPs {
			assert.CheckComponentCmdNode("rpm -qa container-selinux " + product + "-server " + product + "-selinux", serverIP, serverAsserts...)
		}
	}

	if cluster.NumAgents > 0 {
		for _, agentIP := range cluster.AgentIPs {
			assert.CheckComponentCmdNode("rpm -qa container-selinux " + product + "-selinux", agentIP, agentAsserts...)
		}
	}
}

// Validate directories to ensure they have the correct selinux contexts created

// Validate that containers don't run with spc_t
func TestSelinuxSpcT() {
	cluster := factory.GetCluster(GinkgoT())
	securityAssert := "spc_t"

	for _, serverIP := range cluster.ServerIPs {
		assert.CheckNotPresentOnNode("ps auxZ | grep metrics | grep -v grep", serverIP, securityAssert)
		break
	}
}

// Validate that uninstallation will remove the rke2-selinux or k3s-selinux policy
func TestUninstallPolicy(product string) {

	ips := shared.FetchNodeExternalIP()
	asserts := []string{"container-selinux", product + "-selinux", product + "-server"}

	for _, ip := range ips {
		shared.RunCommandOnNode("sudo "+product+"-uninstall.sh", ip)
		assert.CheckNotPresentOnNode("rpm -qa container-selinux" + product + "-server " + product + "-selinux", ip, asserts...)
	}
}
