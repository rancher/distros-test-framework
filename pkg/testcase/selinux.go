package testcase

import (
	"fmt"
	"log"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/distros-test-framework/factory"
	"github.com/rancher/distros-test-framework/pkg/assert"
	"github.com/rancher/distros-test-framework/shared"
)

// TestSelinuxEnabled Validates that containerd is running with selinux enabled in the config
func TestSelinuxEnabled() {
	product, err := shared.GetProduct()
	if err != nil {
		return
	}

	ips := shared.FetchNodeExternalIP()
	selinuxConfigAssert := "selinux: true"
	selinuxContainerdAssert := "enable_selinux = true"

	for _, ip := range ips {
		err := assert.CheckComponentCmdNode("cat /etc/rancher/"+
			product+"/config.yaml", ip, selinuxConfigAssert)
		Expect(err).NotTo(HaveOccurred())
		errCont := assert.CheckComponentCmdNode("sudo cat /var/lib/rancher/"+
			product+"/agent/etc/containerd/config.toml", ip, selinuxContainerdAssert)
		Expect(errCont).NotTo(HaveOccurred())
	}
}

// TestSelinuxVersions Validates container-selinux version, rke2-selinux version and rke2-selinux version
func TestSelinuxVersions() {
	cluster := factory.AddCluster(GinkgoT())
	product, err := shared.GetProduct()
	if err != nil {
		return
	}

	var serverCmd string
	var serverAsserts []string
	agentAsserts := []string{"container-selinux", product + "-selinux"}

	switch product {
	case "k3s":
		serverCmd = "rpm -qa container-selinux k3s-selinux"
		serverAsserts = []string{"container-selinux", "k3s-selinux"}
	default:
		serverCmd = "rpm -qa container-selinux rke2-server rke2-selinux"
		serverAsserts = []string{"container-selinux", "rke2-selinux", "rke2-server"}
	}

	if cluster.NumServers > 0 {
		for _, serverIP := range cluster.ServerIPs {
			err := assert.CheckComponentCmdNode(serverCmd, serverIP, serverAsserts...)
			Expect(err).NotTo(HaveOccurred())
		}
	}

	if cluster.NumAgents > 0 {
		for _, agentIP := range cluster.AgentIPs {
			err := assert.CheckComponentCmdNode("rpm -qa container-selinux "+product+"-selinux", agentIP, agentAsserts...)
			Expect(err).NotTo(HaveOccurred())
		}
	}
}

// Validate directories to ensure they have the correct selinux contexts created

// TestSelinuxSpcT Validate that containers don't run with spc_t
func TestSelinuxSpcT() {
	cluster := factory.AddCluster(GinkgoT())

	for _, serverIP := range cluster.ServerIPs {
		res, err := shared.RunCommandOnNode("ps auxZ | grep metrics | grep -v grep", serverIP)
		Expect(err).NotTo(HaveOccurred())
		Expect(res).ShouldNot(ContainSubstring("spc_t"))
	}
}

// TestUninstallPolicy Validate that un-installation will remove the rke2-selinux or k3s-selinux policy
func TestUninstallPolicy() {
	product, err := shared.GetProduct()
	if err != nil {
		log.Println(err)
	}
	cluster := factory.AddCluster(GinkgoT())
	var serverUninstallCmd string
	var agentUninstallCmd string

	switch product {
	case "k3s":
		serverUninstallCmd = "k3s-uninstall.sh"
		agentUninstallCmd = "k3s-agent-uninstall.sh"

	default:
		serverUninstallCmd = "sudo rke2-uninstall.sh"
		agentUninstallCmd = "sudo rke2-uninstall.sh"
	}

	for _, serverIP := range cluster.ServerIPs {
		fmt.Println("Uninstalling "+product+" on server: ", serverIP)

		_, err := shared.RunCommandOnNode(serverUninstallCmd, serverIP)
		Expect(err).NotTo(HaveOccurred())

		res, errSel := shared.RunCommandOnNode("rpm -qa container-selinux "+product+"-server "+product+"-selinux", serverIP)
		Expect(errSel).NotTo(HaveOccurred())
		Expect(res).Should(BeEmpty())
	}

	for _, agentIP := range cluster.AgentIPs {
		fmt.Println("Uninstalling "+product+" on agent: ", agentIP)

		_, err := shared.RunCommandOnNode(agentUninstallCmd, agentIP)
		Expect(err).NotTo(HaveOccurred())

		res, errSel := shared.RunCommandOnNode("rpm -qa container-selinux "+product+"-selinux", agentIP)
		Expect(errSel).NotTo(HaveOccurred())
		Expect(res).Should(BeEmpty())
	}
}
