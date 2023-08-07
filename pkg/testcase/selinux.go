package testcase

import (
	"fmt"
	"log"

	. "github.com/onsi/ginkgo/v2"
	"github.com/rancher/distros-test-framework/factory"
	"github.com/rancher/distros-test-framework/pkg/assert"
	"github.com/rancher/distros-test-framework/shared"
)

// TestSelinuxEnabled Validates that containerd is running with selinux enabled in the config
func TestSelinuxEnabled() {
	product, err := shared.GetProduct()
	if err != nil {
		log.Println(err)
	}

	ips := shared.FetchNodeExternalIP()
	selinuxConfigAssert := "selinux: true"
	selinuxContainerdAssert := "enable_selinux = true"

	for _, ip := range ips {
		assert.CheckComponentCmdNode("cat /etc/rancher/"+product+"/config.yaml", ip, selinuxConfigAssert)
		assert.CheckComponentCmdNode("sudo cat /var/lib/rancher/"+product+"/agent/etc/containerd/config.toml", ip, selinuxContainerdAssert)
	}
}

// TestSelinuxVersions Validates container-selinux version, rke2-selinux version and rke2-selinux version
func TestSelinuxVersions() {
	product, err := shared.GetProduct()
	if err != nil {
		log.Println(err)
	}
	cluster := factory.GetCluster(GinkgoT())

	serverCmd := "rpm -qa container-selinux " + product + "-server " + product + "-selinux"
	if product == "k3s" {
		serverCmd = "rpm -qa container-selinux " + product + "-selinux"
	}

	serverAsserts := []string{"container-selinux", product + "-selinux", product + "-server"}
	if product == "k3s" {
		serverAsserts = []string{"container-selinux", product + "-selinux"}
	}

	agentAsserts := []string{"container-selinux", product + "-selinux"}

	if cluster.NumServers > 0 {
		for _, serverIP := range cluster.ServerIPs {
			assert.CheckComponentCmdNode(serverCmd, serverIP, serverAsserts...)
		}
	}

	if cluster.NumAgents > 0 {
		for _, agentIP := range cluster.AgentIPs {
			assert.CheckComponentCmdNode("rpm -qa container-selinux "+product+"-selinux", agentIP, agentAsserts...)
		}
	}
}

// Validate directories to ensure they have the correct selinux contexts created

// TestSelinuxSpcT Validate that containers don't run with spc_t
func TestSelinuxSpcT() {
	cluster := factory.GetCluster(GinkgoT())
	securityAssert := "spc_t"

	for _, serverIP := range cluster.ServerIPs {
		assert.CheckNotPresentOnNode("ps auxZ | grep metrics | grep -v grep", serverIP, securityAssert)
		break
	}
}

// TestUninstallPolicy Validate that uninstallation will remove the rke2-selinux or k3s-selinux policy
func TestUninstallPolicy() {
	product, err := shared.GetProduct()
	if err != nil {
		log.Println(err)
	}
	cluster := factory.GetCluster(GinkgoT())

	var asserts []string

	if product == "rke2" {
		asserts = []string{"container-selinux", product + "-selinux", product + "-server"}
	} else {
		asserts = []string{"container-selinux", product + "-selinux"}
	}

	for _, serverIP := range cluster.ServerIPs {
		if product == "rke2" {
			fmt.Println("Uninstalling RKE2 on ", serverIP)
			shared.RunCommandOnNode("sudo "+product+"-uninstall.sh", serverIP)
			assert.CheckNotPresentOnNode("rpm -qa container-selinux "+product+"-server "+product+"-selinux", serverIP, asserts...)

		} else {
			fmt.Println("Uninstalling K3S on ", serverIP)
			shared.RunCommandOnNode(product+"-uninstall.sh", serverIP)
			assert.CheckNotPresentOnNode("rpm -qa container-selinux "+product+"-selinux", serverIP, asserts...)
		}
	}

	for _, agentIP := range cluster.AgentIPs {
		if product == "rke2" {
			fmt.Println("Uninstalling RKE2 on ", agentIP)
			shared.RunCommandOnNode("sudo "+product+"-uninstall.sh", agentIP)
			assert.CheckNotPresentOnNode("rpm -qa container-selinux "+product+"-server "+product+"-selinux", agentIP, asserts...)
		} else {
			fmt.Println("Uninstalling K3S on ", agentIP)
			shared.RunCommandOnNode(product+"-agent-uninstall.sh", agentIP)
			assert.CheckNotPresentOnNode("rpm -qa container-selinux "+product+"-selinux", agentIP, asserts...)
		}
	}
}
