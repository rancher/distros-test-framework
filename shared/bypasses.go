package shared

import (
	"fmt"
)

// BypassFunc defines the signature for system bypass functions
type BypassFunc func(config *QAInfraConfig, nodes []InfraNode) error

// applySystemBypasses applies various system-level bypasses before installation
func applySystemBypasses(config *QAInfraConfig) error {
	LogLevel("info", "Applying system bypasses for compatibility...")

	nodes, err := getAllNodesFromState(config.NodeSource)
	if err != nil {
		return fmt.Errorf("failed to get node information: %w", err)
	}

	// Define all bypass functions to apply
	// To add a new bypass:
	// 1. Create a function with signature: func(config *QAInfraConfig, nodes []InfraNode) error
	// 2. Add it to the bypasses slice below with a descriptive name
	bypasses := []struct {
		name string
		fn   BypassFunc
	}{
		{"NetworkManager Cloud Setup", disableNetworkManagerCloudSetupBypass},
		// Add more bypasses here as needed:
		// {"SELinux Configuration", configureSELinuxBypass},
		// {"Firewall Rules", configureFirewallBypass},
		// {"Custom Service Disable", disableCustomServiceBypass},
	}

	// Apply each bypass
	for _, bypass := range bypasses {
		LogLevel("info", "Applying bypass: %s", bypass.name)
		if err := bypass.fn(config, nodes); err != nil {
			LogLevel("warn", "Bypass '%s' failed: %v", bypass.name, err)
			// Continue with other bypasses instead of failing completely
		} else {
			LogLevel("info", "Bypass '%s' completed successfully", bypass.name)
		}
	}

	LogLevel("info", "Completed all system bypasses")
	return nil
}

func disableNetworkManagerCloudSetupBypass(config *QAInfraConfig, nodes []InfraNode) error {
	LogLevel("debug", "Disabling NetworkManager cloud setup services on %d nodes", len(nodes))

	for _, node := range nodes {
		if err := disableCloudSetupOnNode(config, node); err != nil {
			LogLevel("warn", "Failed to disable cloud setup on node %s: %v", node.Name, err)
			// Continue with other nodes instead of failing completely
		}
	}

	return nil
}

// disableCloudSetupOnNode disables cloud setup services on a single node
func disableCloudSetupOnNode(config *QAInfraConfig, node InfraNode) error {
	LogLevel("debug", "Disabling cloud setup on node: %s (%s)", node.Name, node.PublicIP)

	// Commands to disable NetworkManager cloud setup
	commands := []string{
		"sudo systemctl disable nm-cloud-setup.service 2>/dev/null || echo 'nm-cloud-setup.service not found or not enabled'",
		"sudo systemctl stop nm-cloud-setup.service 2>/dev/null || echo 'nm-cloud-setup.service not running'",
		"sudo systemctl disable nm-cloud-setup.timer 2>/dev/null || echo 'nm-cloud-setup.timer not found or not enabled'",
		"sudo systemctl stop nm-cloud-setup.timer 2>/dev/null || echo 'nm-cloud-setup.timer not running'",
	}

	// Add NetworkManager configuration for Canal/Flannel interfaces.
	networkManagerWorkaround := `
sudo mkdir -p /etc/NetworkManager/conf.d
sudo tee /etc/NetworkManager/conf.d/canal.conf > /dev/null << 'EOF'
[keyfile]
unmanaged-devices=interface-name:cali*;interface-name:tunl*;interface-name:vxlan.calico;interface-name:flannel*
EOF`
	commands = append(commands, networkManagerWorkaround)

	for _, cmd := range commands {
		sshCmd := fmt.Sprintf("ssh -o StrictHostKeyChecking=no -i %s %s@%s \"%s\"",
			config.SSHConfig.KeyPath, config.SSHConfig.User, node.PublicIP, cmd)

		if err := runCmd(config.RootDir, "bash", "-c", sshCmd); err != nil {
			LogLevel("debug", "Command failed on %s: %s", node.Name, cmd)
			// Don't return error for individual command failures, just log them
		}
	}

	return nil
}

// Example bypass function - shows how to add new bypasses
// Uncomment and modify as needed for your specific requirements
/*
func configureSELinuxBypass(config *QAInfraConfig, nodes []InfraNode) error {
	LogLevel("debug", "Configuring SELinux on %d nodes", len(nodes))

	for _, node := range nodes {
		// Example SELinux configuration commands
		commands := []string{
			"sudo setsebool -P container_manage_cgroup on",
			"sudo setsebool -P virt_use_nfs on",
		}

		for _, cmd := range commands {
			sshCmd := fmt.Sprintf("ssh -o StrictHostKeyChecking=no -i %s %s@%s \"%s\"",
				config.SSHConfig.KeyPath, config.SSHConfig.User, node.PublicIP, cmd)

			if err := runCmd(config.RootDir, "bash", "-c", sshCmd); err != nil {
				LogLevel("debug", "SELinux command failed on %s: %s", node.Name, cmd)
			}
		}
	}

	return nil
}
*/
