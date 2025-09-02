package shared

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

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
	data.summaryData.WriteString("\n" + "**OS Release**" + "\n\n")
	data.summaryData.WriteString("```yaml\n")
	data.summaryData.WriteString(data.osReleaseData)
	data.summaryData.WriteString("\n```\n")

	// config.yaml from server node.
	cmd := fmt.Sprintf("cat /etc/rancher/%s/config.yaml", c.Config.Product)
	catConfigYaml, configYamlErr := RunCommandOnNode(cmd, c.ServerIPs[0])
	if configYamlErr != nil {
		return fmt.Errorf("error retrieving config.yaml from server: %s, %w", c.ServerIPs[0], configYamlErr)
	}
	data.configYaml = strings.TrimSpace(catConfigYaml)
	data.summaryData.WriteString("\n" + "**Config YAML**" + "\n")
	data.summaryData.WriteString("```yaml\n")
	data.summaryData.WriteString(data.configYaml)
	data.summaryData.WriteString("\n```\n")

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
		data.summaryData.WriteString("\n" + "**Kernel Version**" + "\n")
		data.summaryData.WriteString("```yaml\n")
		data.summaryData.WriteString(unameOutput)
		data.summaryData.WriteString("\n```\n")
	}

	// TODO: ADD split roles data once on airgap is supported.
	if c.Config.SplitRoles.Add {
		splitRoleData := getSplitRoleData(&c.Config, c.ServerIPs)
		data.summaryData.WriteString(splitRoleData)
	}

	return nil
}

//nolint:funlen // yep, but this makes more clear being one function.
func airgapNodeSummaryData(c *Cluster, flags *customflag.FlagConfig, data *summaryReportData) error {
	// config.yaml from server via bastion node.
	cfgCmd := fmt.Sprintf("cat /etc/rancher/%s/config.yaml", c.Config.Product)
	cfg, err := remoteExec(c.SSH.KeyName, c.SSH.User, c.ServerIPs[0], c.BastionConfig.PublicIPv4Addr, cfgCmd)
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
		c.SSH.KeyName,
		c.SSH.User,
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
		c.SSH.KeyName,
		c.SSH.User,
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

	data.summaryData.WriteString("\n" + "**Config YAML**" + "\n" + "\n")
	data.summaryData.WriteString("```yaml\n")
	data.summaryData.WriteString(data.configYaml)
	data.summaryData.WriteString("\n```\n")

	data.summaryData.WriteString("\n" + "**OS Release**" + "\n" + "\n")
	data.summaryData.WriteString("```yaml\n")
	data.summaryData.WriteString(data.osReleaseData)
	data.summaryData.WriteString("\n```\n")

	data.summaryData.WriteString("\n" + "**Kernel Version**" + "\n" + "\n")
	data.summaryData.WriteString("```yaml\n")
	data.summaryData.WriteString(unameOutput)
	data.summaryData.WriteString("\n```\n")

	data.summaryData.WriteString("\n" + "**Airgap-Info**" + "\n")
	data.summaryData.WriteString("```yaml\n")
	data.summaryData.WriteString(data.airgapInfo)
	data.summaryData.WriteString("\n```\n")

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
	data.summaryData.WriteString("\n" + "Registries" + "\n" + "\n")
	data.summaryData.WriteString("```yaml\n")
	data.summaryData.WriteString(data.registries)
	data.summaryData.WriteString("\n```\n")

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
	var selinuxData strings.Builder

	cmdOutput := func(title, command string) {
		output, err := RunCommandOnNode(command, nodeIP)
		if err != nil {
			LogLevel("debug", "Failed to get %s: %v", title, err)
			output = "Failed to retrieve " + strings.ToLower(title)
		}

		output = strings.TrimSpace(output)
		if output != "" {
			selinuxData.WriteString(fmt.Sprintf("\n\n**%s**\n\n", title))
			selinuxData.WriteString("```yaml\n")
			selinuxData.WriteString(output)
			selinuxData.WriteString("\n```\n")
		}
	}

	cmdOutput("SELinux Status:",
		"sestatus 2>/dev/null || echo 'sestatus command not found'")

	cmdOutput("SELinux Packages:",
		"rpm -qa | grep selinux 2>/dev/null || echo 'No SELinux packages found'")

	if product == "rke2" && installMethod == "rpm" {
		cmdOutput("RKE2 SELinux Package:",
			"rpm -q rke2-selinux 2>/dev/null || echo 'rke2-selinux package not installed'")
	}

	cmdOutput("Container SELinux Package:",
		"rpm -q container-selinux 2>/dev/null || echo 'container-selinux package not installed'")

	return selinuxData.String()
}

// getSplitRoleData retrieves the split roles data from the cluster nodes and formats it for the report.
//
//nolint:funlen // yep, but this makes more clear being one function.
func getSplitRoleData(config *clusterConfig, serverIps []string) string {
	var splitRoleData strings.Builder

	splitRoleData.WriteString("\n" + "\n")
	splitRoleData.WriteString("**Split Roles Configuration**:\n")
	splitRoleData.WriteString(fmt.Sprintf(
		"CP-only=%d,"+
			" CP-worker=%d, "+
			"Etcd-only=%d, "+
			"Etcd-CP=%d, "+
			"Etcd-worker=%d\n",
		config.SplitRoles.ControlPlaneOnly,
		config.SplitRoles.ControlPlaneWorker,
		config.SplitRoles.EtcdOnly,
		config.SplitRoles.EtcdCP,
		config.SplitRoles.EtcdWorker))
	splitRoleData.WriteString("\n")

	splitRoleData.WriteString("**Split Roles Data**:\n")
	splitRoleData.WriteString("\n")

	for _, ip := range serverIps {
		cmd := fmt.Sprintf("sudo cat /etc/rancher/%s/config.yaml.d/role_config.yaml", config.Product)
		configYaml, err := RunCommandOnNode(cmd, ip)
		if err != nil {
			LogLevel("warn", "Error retrieving config.yaml from node %s: %v", ip, err)
			splitRoleData.WriteString(fmt.Sprintf("**Node %s**: Error retrieving config\n\n", ip))
			continue
		}

		role, err := determineRoleFromConfig(configYaml)
		if err != nil {
			LogLevel("warn", "Error determining role for node %s: %v", ip, err)
			role = nodeTypeUnknown
		}

		splitRoleData.WriteString(fmt.Sprintf("**Node %s** (Role: %s)\n", ip, role))
		splitRoleData.WriteString("```yaml\n")
		splitRoleData.WriteString(strings.TrimSpace(configYaml))
		splitRoleData.WriteString("\n```\n")
	}

	return splitRoleData.String()
}

type NodeType string

const (
	nodeTypeAllRoles   NodeType = "all-roles"
	nodeTypeEtcdOnly   NodeType = "etcd-only"
	nodeTypeEtcdCP     NodeType = "etcd-cp"
	nodeTypeEtcdWorker NodeType = "etcd-worker"
	nodeTypeCPOnly     NodeType = "cp-only"
	nodeTypeCPWorker   NodeType = "cp-worker"
	nodeTypeWorkerOnly NodeType = "worker-only"
	nodeTypeUnknown    NodeType = "unknown"
)

type nodeConfig struct {
	DisableAPIServer         bool     `yaml:"disable-apiserver,omitempty"`
	DisableControllerManager bool     `yaml:"disable-controller-manager,omitempty"`
	DisableScheduler         bool     `yaml:"disable-scheduler,omitempty"`
	DisableETCD              bool     `yaml:"disable-etcd,omitempty"`
	NodeTaint                []string `yaml:"node-taint,omitempty"`
	NodeLabel                []string `yaml:"node-label,omitempty"`
}

// determineRoleFromConfig determines the node role based on the provided config content.
// also reflecting node_role script logic.
func determineRoleFromConfig(configContent string) (NodeType, error) {
	var config nodeConfig
	err := yaml.Unmarshal([]byte(configContent), &config)
	if err != nil {
		return nodeTypeUnknown, fmt.Errorf("error unmarshalling config content: %w", err)
	}

	hasEtcdLabel := hasLabel(config.NodeLabel, "role-etcd=true")
	hasCPLabel := hasLabel(config.NodeLabel, "role-control-plane=true")
	hasWorkerLabel := hasLabel(config.NodeLabel, "role-worker=true")
	noExecuteTaint := hasTaint(config.NodeTaint, "node-role.kubernetes.io/etcd:NoExecute")
	noScheduleTaint := hasTaint(config.NodeTaint, "node-role.kubernetes.io/control-plane:NoSchedule")

	switch {
	case config.DisableAPIServer && config.DisableControllerManager && config.DisableScheduler &&
		hasEtcdLabel && !hasCPLabel && !hasWorkerLabel && noExecuteTaint:
		return nodeTypeEtcdOnly, nil

	case !config.DisableAPIServer && !config.DisableControllerManager && !config.DisableScheduler &&
		hasEtcdLabel && hasCPLabel && !hasWorkerLabel && noExecuteTaint && noScheduleTaint:
		return nodeTypeEtcdCP, nil

	case config.DisableAPIServer && config.DisableControllerManager && config.DisableScheduler &&
		hasEtcdLabel && !hasCPLabel && hasWorkerLabel && !noExecuteTaint:
		return nodeTypeEtcdWorker, nil

	case config.DisableETCD && !noScheduleTaint &&
		!hasEtcdLabel && hasCPLabel && hasWorkerLabel:
		return nodeTypeCPWorker, nil

	case config.DisableETCD && noScheduleTaint &&
		!hasEtcdLabel && hasCPLabel && !hasWorkerLabel:
		return nodeTypeCPOnly, nil

	case !config.DisableETCD && !config.DisableAPIServer && !config.DisableControllerManager && !config.DisableScheduler &&
		hasEtcdLabel && hasCPLabel && hasWorkerLabel:
		return nodeTypeAllRoles, nil

	default:
		return nodeTypeWorkerOnly, nil
	}
}

func hasLabel(labels []string, label string) bool {
	for _, l := range labels {
		if l == label {
			return true
		}
	}

	return false
}

func hasTaint(taints []string, taint string) bool {
	for _, t := range taints {
		if t == taint {
			return true
		}
	}

	return false
}
