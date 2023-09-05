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
		log.Println(err)
	}

	ips := shared.FetchNodeExternalIP()
	selinuxConfigAssert := "selinux: true"
	selinuxContainerdAssert := "enable_selinux = true"

	for _, ip := range ips {
		err := assert.CheckComponentCmdNode("cat /etc/rancher/"+product+"/config.yaml", ip, selinuxConfigAssert)
		Expect(err).NotTo(HaveOccurred())
		errCont := assert.CheckComponentCmdNode("sudo cat /var/lib/rancher/"+product+"/agent/etc/containerd/config.toml", ip, selinuxContainerdAssert)
		Expect(errCont).NotTo(HaveOccurred())
	}
}

// TestSelinuxVersions Validates container-selinux version, rke2-selinux version and rke2-selinux version
func TestSelinuxVersions() {
	product, err := shared.GetProduct()
	if err != nil {
		log.Println(err)
	}
	cluster := factory.AddCluster(GinkgoT())

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

	serverUninstallCmd := "sudo " + product + "-uninstall.sh"
	if product == "k3s" {
		serverUninstallCmd = product + "-uninstall.sh"
	}

	agentUninstallCmd := "sudo " + product + "-uninstall.sh"
	if product == "k3s" {
		agentUninstallCmd = product + "-agent-uninstall.sh"
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
