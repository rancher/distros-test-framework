package testcase

import (
	"fmt"
	"log"
	"strings"

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

// https://github.com/k3s-io/k3s/blob/master/install.sh
// https://github.com/rancher/rke2/blob/master/install.sh
// Based on this info, this is the way to validate the correct context

// TestSelinuxContext Validates directories to ensure they have the correct selinux contexts created
func TestSelinuxContext() {
	cluster := factory.AddCluster(GinkgoT())
	product, err := shared.GetProduct()
	if err != nil {
		log.Println(err)
	}

	if cluster.NumServers > 0 {
		for _, ip := range cluster.ServerIPs {
			var context map[string]string
			context, err := getContext(product, ip)
			Expect(err).NotTo(HaveOccurred())

			fmt.Print("\nThese are the whole commands to use in this context validation\n")
			fmt.Print("Command to run || Context expected\n")
			for cmdsToRun, contExpected := range context {
				fmt.Println(cmdsToRun + " || " + contExpected)
			}

			for cmd, expectedContext := range context {
				res, err := shared.RunCommandOnNode(cmd, ip)
				fmt.Println("\nRunning cmd:", cmd, "\nExpected context:", expectedContext)
				fmt.Println("Result: \n", res)
				if res != "" {
					Expect(res).Should(ContainSubstring(expectedContext), "Error on cmd %v \n Context %v \nnot found on ", cmd, expectedContext, res)
					Expect(err).NotTo(HaveOccurred())
				}
			}
		}
	}
}

func getVersion(osRelease, ip string) string {
	if strings.Contains(osRelease, "VERSION_ID") {
		res, err := shared.RunCommandOnNode("cat /etc/os-release | grep 'VERSION_ID'", ip)
		Expect(err).NotTo(HaveOccurred())
		parts := strings.Split(res, "=")
		if len(parts) == 2 {
			// Get version
			version := strings.Trim(parts[1], "\"")
			// if dot exist get the first number
			if dotIndex := strings.Index(version, "."); dotIndex != -1 {
				version = version[:dotIndex]
			}
			return version
		}
	}
	return ""
}

func getContext(product, ip string) (cmdCtx, error) {
	res, err := shared.RunCommandOnNode("cat /etc/os-release", ip)
	if err != nil {
		return nil, err
	}

	fmt.Println("OS Release: \n", res)
	policyMapping := map[string]string{
		"ID_LIKE='suse' VARIANT_ID='sle-micro'": "sle_micro",
		"ID_LIKE='suse'":                        "micro_os",
		"ID_LIKE='coreos'":                      "coreos",
		"VARIANT_ID='coreos'":                   "coreos",
	}

	for k, v := range policyMapping {
		if strings.Contains(res, k) {
			return selectPolicy(product, v), nil
		}
	}

	version := getVersion(res, ip)
	versionMapping := map[string]string{
		"7": "centos7",
		"8": "centos8",
		"9": "centos9",
	}

	if policy, ok := versionMapping[version]; ok {
		return selectPolicy(product, policy), nil
	}

	return nil, fmt.Errorf("unable to determine policy for %s on os: %s", ip, res)
}

func selectPolicy(product, osType string) cmdCtx {
	key := fmt.Sprintf("%s_%s", product, osType)

	for _, config := range conf {
		if config.distroName == key {
			fmt.Printf("\nUsing '%s' policy for this %s cluster.\n", osType, product)
			return config.cmdCtx
		}
	}

	fmt.Printf("Configuration for %s not found!\n", key)
	return nil
}

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
	var serverCmd string

	switch product {
	case "k3s":
		serverUninstallCmd = "k3s-uninstall.sh"
		agentUninstallCmd = "k3s-agent-uninstall.sh"
		serverCmd = "rpm -qa container-selinux k3s-selinux"

	default:
		serverUninstallCmd = "sudo rke2-uninstall.sh"
		agentUninstallCmd = "sudo rke2-uninstall.sh"
		serverCmd = "rpm -qa container-selinux rke2-server rke2-selinux"
	}

	for _, serverIP := range cluster.ServerIPs {
		fmt.Println("Uninstalling "+product+" on server: ", serverIP)

		_, err := shared.RunCommandOnNode(serverUninstallCmd, serverIP)
		Expect(err).NotTo(HaveOccurred())

		res, errSel := shared.RunCommandOnNode(serverCmd, serverIP)
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
