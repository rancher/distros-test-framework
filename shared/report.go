package shared

import (
	"fmt"
	"strings"

	"github.com/rancher/distros-test-framework/pkg/customflag"
)

type summaryReportData struct {
	registries    string
	airgapInfo    string
	configYaml    string
	osReleaseData string
	summaryData   strings.Builder
}

// SummaryReportData retrieves the config.yaml and os-release data from the cluster node and sends it to spec report.
func SummaryReportData(c *Cluster, flags *customflag.FlagConfig) (string, error) {
	var data summaryReportData

	if c.NumBastion == 0 {
		if nodeDataErr := nodeSummaryData(c, &data); nodeDataErr != nil {
			return "", fmt.Errorf("error retrieving node summary data: %w", nodeDataErr)
		}
	} else {
		if airgapDataErr := airgapNodeSummaryData(c, flags, &data); airgapDataErr != nil {
			return "", fmt.Errorf("error retrieving airgap summary data: %w", airgapDataErr)
		}
	}

	return data.summaryData.String(), nil
}

func nodeSummaryData(c *Cluster, data *summaryReportData) error {
	// os-release data from the first server node.
	res, err := RunCommandOnNode("cat /etc/os-release", c.ServerIPs[0])
	if err != nil {
		return fmt.Errorf("error retrieving os-release from server: %s, %w", c.ServerIPs[0], err)
	}
	data.osReleaseData = strings.TrimSpace(res)
	data.summaryData.WriteString("\n" + "**OS Release**" + "\n" + data.osReleaseData + "\n\n")

	// config.yaml from server node.
	cmd := fmt.Sprintf("cat /etc/rancher/%s/config.yaml", c.Config.Product)
	catConfigYaml, configYamlErr := RunCommandOnNode(cmd, c.ServerIPs[0])
	if configYamlErr != nil {
		return fmt.Errorf("error retrieving config.yaml from server: %s, %w", c.ServerIPs[0], configYamlErr)
	}
	data.configYaml = strings.TrimSpace(catConfigYaml)
	data.summaryData.WriteString("\n" + "**Config YAML**" + "\n" + data.configYaml)
	data.summaryData.WriteString("\n")

	// for now only gather selinux info from RPM based and not airgap nodes.
	if isRPMBasedOS(data.osReleaseData) {
		selinuxInfo := getSELinuxInfo(
			c.Config.Product,
			c.Config.InstallMethod,
			c.ServerIPs[0],
		)

		if selinuxInfo != "" {
			data.summaryData.WriteString("\n")
			data.summaryData.WriteString("\n" + "**SELinux Information**" + "\n" + selinuxInfo)
			data.summaryData.WriteString("\n\n")
		}
	}

	// Kernel version from server node.
	unameOutput, err := RunCommandOnNode("uname -r", c.ServerIPs[0])
	if err != nil {
		unameOutput = "Kernel version not found " + fmt.Sprintf("error: %v", err)
		data.summaryData.WriteString("\n" + "**Kernel Version**" + "\n" + unameOutput + "\n")

		LogLevel("warn", "Kernel version not found on node %s", c.ServerIPs[0])
	}
	unameOutput = strings.TrimSpace(unameOutput)
	if unameOutput != "" {
		data.summaryData.WriteString("\n" + "**Kernel Version**" + "\n" + unameOutput + "\n")
	}

	return nil
}

func airgapNodeSummaryData(c *Cluster, flags *customflag.FlagConfig, data *summaryReportData) error {
	// config.yaml from server via bastion node.
	cfgCmd := fmt.Sprintf("cat /etc/rancher/%s/config.yaml", c.Config.Product)
	cfg, err := remoteExec(c.Aws.KeyName, c.Aws.AwsUser, c.ServerIPs[0], c.BastionConfig.PublicIPv4Addr, cfgCmd)
	if err != nil {
		return fmt.Errorf("retrieving config.yaml: %w", err)
	}
	data.configYaml = strings.TrimSpace(cfg)

	// registries.yaml from bastion node id tag is privateregistry.
	if c.TestConfig.Tag == "privateregistry" {
		pvRg := getPrivateRegistries(c.BastionConfig.PublicIPv4Addr, data)
		if pvRg != nil {
			return fmt.Errorf("error retrieving private registries: %w", pvRg)
		}
	}

	// /etc/os-release from server via bastion node.
	osRelease, osReleaseErr := remoteExec(
		c.Aws.KeyName,
		c.Aws.AwsUser,
		c.ServerIPs[0],
		c.BastionConfig.PublicIPv4Addr,
		"cat /etc/os-release",
	)
	if osReleaseErr != nil {
		return fmt.Errorf("retrieving os-release: %w", osReleaseErr)
	}
	data.osReleaseData = strings.TrimSpace(osRelease)

	// Kernel version from server node via bastion node.
	unameOutput, unameErr := remoteExec(
		c.Aws.KeyName,
		c.Aws.AwsUser,
		c.ServerIPs[0],
		c.BastionConfig.PublicIPv4Addr,
		"uname -r",
	)
	if unameErr != nil {
		unameOutput = "Kernel version not found " + fmt.Sprint("error: %w", unameErr)
		data.summaryData.WriteString("\n" + "**Kernel Version**" + "\n" + unameOutput + "\n")

		LogLevel("warn", "Kernel version not found on node %s via bastion %s: %v",
			c.ServerIPs[0], c.BastionConfig.PublicIPv4Addr, unameErr)
	}
	unameOutput = strings.TrimSpace(unameOutput)

	// airgap info from environment variables/flags.
	data.airgapInfo = fmt.Sprintf(
		"\nurl: %s\nhost: %s\nusername: %s\npassword: %s",
		flags.AirgapFlag.ImageRegistryUrl,
		c.BastionConfig.PublicDNS,
		flags.AirgapFlag.RegistryUsername,
		flags.AirgapFlag.RegistryPassword,
	)
	data.summaryData.WriteString("\n" + "**Config YAML**" + "\n" + data.configYaml + "\n")
	data.summaryData.WriteString("\n" + "**OS Release**" + "\n" + data.osReleaseData + "\n")
	data.summaryData.WriteString("\n" + "**Kernel Version**" + "\n" + unameOutput + "\n")
	data.summaryData.WriteString("\n" + "**Airgap-Info**" + data.airgapInfo + "\n")

	return nil
}

func getPrivateRegistries(bastionIP string, data *summaryReportData) error {
	// registries.yaml on bastion node itself.
	privateRegistries, regErr := RunCommandOnNode("cat registries.yaml", bastionIP)
	if regErr != nil {
		return fmt.Errorf("error retrieving registries.yaml from bastion node: %s, %w",
			bastionIP, regErr)
	}
	data.registries = strings.TrimSpace(privateRegistries)
	data.summaryData.WriteString("\n" + "Registries" + "\n" + data.registries + "\n")

	return nil
}

// remoteExec little helper that executes a command on a remote server.
func remoteExec(keyName, user, serverIP, bastionIP, cmd string) (string, error) {
	chmod := fmt.Sprintf("sudo chmod 400 /tmp/%s.pem", keyName)
	ssh := fmt.Sprintf(
		"ssh -i /tmp/%s.pem -o StrictHostKeyChecking=no -o IdentitiesOnly=yes %s@%s",
		keyName, user, serverIP,
	)
	targetCmd := fmt.Sprintf("%s && %s '%s'", chmod, ssh, cmd)

	return RunCommandOnNode(targetCmd, bastionIP)
}

// isRPMBasedOS checks if the operating system is RPM-based by examining the OS release data.
func isRPMBasedOS(osReleaseData string) bool {
	rpmBasedDistros := []string{
		"rhel", "centos", "fedora", "rocky", "opensuse", "sles", "micro",
	}

	osData := strings.ToLower(osReleaseData)
	for _, distro := range rpmBasedDistros {
		if strings.Contains(osData, "id="+distro) ||
			strings.Contains(osData, "id=\""+distro+"\"") ||
			strings.Contains(osData, "id_like="+distro) ||
			strings.Contains(osData, "id_like=\""+distro+"\"") {
			return true
		}
	}

	return false
}

// getSELinuxInfo retrieves SELinux status and package information from an RPM-based system.
func getSELinuxInfo(product, installMethod, nodeIP string) string {
	var selinuxInfo strings.Builder

	sestatus, err := RunCommandOnNode("sestatus 2>/dev/null || echo 'sestatus command not found'", nodeIP)
	if err != nil {
		LogLevel("debug", "Failed to get sestatus: %v", err)
		sestatus = "Failed to retrieve sestatus"
	}
	sestatus = strings.TrimSpace(sestatus)
	if sestatus != "" {
		selinuxInfo.WriteString("\n" + "\n")
		selinuxInfo.WriteString("**SELinux Status:**\n" + sestatus + "\n\n")
		selinuxInfo.WriteString("\n")
	}

	// SELinux packages
	selinuxPkgs, err := RunCommandOnNode("rpm -qa | grep selinux 2>/dev/null || "+
		" echo 'No SELinux packages found'", nodeIP)
	if err != nil {
		LogLevel("debug", "Failed to get SELinux packages: %v", err)
		selinuxPkgs = "Failed to retrieve SELinux packages"
	}
	selinuxPkgs = strings.TrimSpace(selinuxPkgs)
	if selinuxPkgs != "" {
		selinuxInfo.WriteString("\n" + "\n" + "**SELinux Packages:**\n" + selinuxPkgs + "\n" + "\n")
		selinuxInfo.WriteString("\n")
	}

	if product == "rke2" && installMethod == "rpm" {
		rke2Selinux, err := RunCommandOnNode("rpm -q rke2-selinux 2>/dev/null || "+
			" echo 'rke2-selinux package not installed'", nodeIP)
		if err != nil {
			LogLevel("debug", "Failed to get rke2-selinux version: %v", err)
			rke2Selinux = "Failed to retrieve rke2-selinux version"
		}
		rke2Selinux = strings.TrimSpace(rke2Selinux)
		if rke2Selinux != "" {
			selinuxInfo.WriteString("\n" + "**RKE2 SELinux Package:**\n" + rke2Selinux + "\n\n")
			selinuxInfo.WriteString("\n")
		}
	}

	// specific container-selinux version
	containerSelinux, err := RunCommandOnNode("rpm -q container-selinux 2>/dev/null || "+
		" echo 'container-selinux package not installed'", nodeIP)
	if err != nil {
		LogLevel("debug", "Failed to get container-selinux version: %v", err)
		containerSelinux = "Failed to retrieve container-selinux version"
	}
	containerSelinux = strings.TrimSpace(containerSelinux)
	if containerSelinux != "" {
		selinuxInfo.WriteString("\n" + "\n" + "**Container SELinux Package:**\n" + containerSelinux + "\n" + "\n")
		selinuxInfo.WriteString("\n")
	}

	return selinuxInfo.String()
}
