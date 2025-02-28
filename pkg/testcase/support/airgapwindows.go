package support

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/rancher/distros-test-framework/pkg/customflag"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/gomega"
)

func InstallOnAirgapAgentsWindows(cluster *shared.Cluster, airgapMethod string) {
	serverIP := cluster.ServerIPs[0]
	agentFlags := os.Getenv("worker_flags")
	if airgapMethod == SystemDefaultRegistry && !strings.Contains(agentFlags, "system-default-registry") {
		agentFlags += "`nsystem-default-registry: " + cluster.BastionConfig.PublicDNS
	}
	if strings.Contains(cluster.ServerIPs[0], ":") {
		serverIP = shared.EncloseSqBraces(serverIP)
	}

	for idx, agentIP := range cluster.WinAgentIPs {
		shared.LogLevel("info", "Install %v on Windows agent-%v...", cluster.Config.Product, idx+1)
		cmd := fmt.Sprintf(
			`powershell .\windows_install.ps1 "%v" "%v" "%v" "%v" "%v"`,
			serverIP, token, agentIP, airgapMethod, agentFlags)
		_, err := CmdForPrivateNode(cluster, cmd, agentIP)
		Expect(err).To(BeNil(), err)
	}
}

// CopyAssetsOnNodesWindows copies all the assets from bastion to Windows nodes.
func CopyAssetsOnNodesWindows(cluster *shared.Cluster, airgapMethod string) (err error) {
	nodeIPs := cluster.WinAgentIPs
	errChan := make(chan error, len(nodeIPs))
	var wg sync.WaitGroup

	for _, nodeIP := range nodeIPs {
		wg.Add(1)
		go func(nodeIP string) {
			defer wg.Done()
			shared.LogLevel("debug", "Copying %v assets on Windows node IP: %s", cluster.Config.Product, nodeIP)
			err = copyAssetsOnWindows(cluster, airgapMethod, nodeIP)
			if err != nil {
				errChan <- shared.ReturnLogError("error copying assets on airgap node: %v\n, err: %w", nodeIP, err)
			}
			switch airgapMethod {
			case "private_registry":
				shared.LogLevel("debug", "Copying registry.yaml on Windows node IP: %s", nodeIP)
				err = copyRegistryOnWindows(cluster, nodeIP)
				if err != nil {
					errChan <- shared.ReturnLogError("error copying registry to airgap node: %v\n, err: %w", nodeIP, err)
				}
			case "system_default_registry":
				shared.LogLevel("debug", "Trust CA Certs on Windows node IP: %s", nodeIP)
				err = trustCertOnWindows(cluster, nodeIP)
				if err != nil {
					errChan <- shared.ReturnLogError("error trusting ssl cert on airgap node: %v\n, err: %w", nodeIP, err)
				}
			}
		}(nodeIP)
	}
	wg.Wait()
	close(errChan)

	for err := range errChan {
		if err != nil {
			return err
		}
	}

	return nil
}

// TODO: For system-default-registry.
func trustCertOnWindows(cluster *shared.Cluster, ip string) (err error) {
	shared.LogLevel("debug", "%v", cluster.Config.Product)
	shared.LogLevel("debug", "%v", ip)
	return err
}

func copyRegistryOnWindows(cluster *shared.Cluster, ip string) (err error) {
	cmd := fmt.Sprintf(
		"sudo %v registries-windows.yaml %v@%v:C:/Users/Administrator",
		ShCmdPrefix("scp", cluster.Aws.KeyName),
		"Administrator", ip)
	_, err = shared.RunCommandOnNode(cmd, cluster.BastionConfig.PublicIPv4Addr)
	if err != nil {
		return fmt.Errorf("error scp-ing registries.yaml on airgapped node: %v, \nerr: %w", ip, err)
	}

	return err
}

func copyAssetsOnWindows(cluster *shared.Cluster, airgapMethod, ip string) (err error) {
	windowsUser := "Administrator"
	cmd := fmt.Sprintf(
		"sudo chmod 400 /tmp/%v.pem && ", cluster.Aws.KeyName)

	cmd += fmt.Sprintf(
		"sudo %v artifacts-windows/* %v@%v:C:/Users/Administrator && ",
		ShCmdPrefix("scp", cluster.Aws.KeyName),
		windowsUser, ip)

	if airgapMethod != "tarball" {
		cmd += fmt.Sprintf(
			"sudo %v certs/* %v@%v:C:/Users/Administrator && ",
			ShCmdPrefix("scp", cluster.Aws.KeyName),
			windowsUser, ip)
	}

	cmd += fmt.Sprintf(
		"sudo %v windows_install.ps1 %v@%v:C:/Users/Administrator",
		ShCmdPrefix("scp", cluster.Aws.KeyName),
		windowsUser, ip)
	_, err = shared.RunCommandOnNode(cmd, cluster.BastionConfig.PublicIPv4Addr)

	return err
}

// GetArtifactsWindows executes get_artifacts.sh script for Windows.
func GetArtifactsWindows(cluster *shared.Cluster, tarballType string) (res string, err error) {
	serverFlags := os.Getenv("server_flags")
	cmd := fmt.Sprintf(
		"sudo chmod +x get_artifacts.sh && "+
			`sudo ./get_artifacts.sh "%v" "%v" "windows" "%v" "%v" "%v"`,
		cluster.Config.Product, cluster.Config.Version,
		cluster.Config.Arch, serverFlags, tarballType)
	res, err = shared.RunCommandOnNode(cmd, cluster.BastionConfig.PublicIPv4Addr)

	return res, err
}

// UpdateRegistryFileWindows updates registries.yaml file and copies to bastion node for Windows.
func UpdateRegistryFileWindows(cluster *shared.Cluster, flags *customflag.FlagConfig) (err error) {
	regMap := map[string]string{
		"$PRIVATE_REG": cluster.BastionConfig.PublicDNS,
		"$USERNAME":    flags.AirgapFlag.RegistryUsername,
		"$PASSWORD":    flags.AirgapFlag.RegistryPassword,
		"$HOMEDIR":     "C:/Users/Administrator",
	}

	path := shared.BasePath() + "/modules/airgap/setup"
	err = shared.CopyFileContents(path+"/registries.yaml.example", path+"/registries-windows.yaml")
	if err != nil {
		return fmt.Errorf("error copying registries-windows.yaml contents: %w", err)
	}

	err = shared.ReplaceFileContents(path+"/registries-windows.yaml", regMap)
	if err != nil {
		return fmt.Errorf("error replacing registries.yaml contents: %w", err)
	}

	err = shared.RunScp(
		cluster, cluster.BastionConfig.PublicIPv4Addr,
		[]string{path + "/registries-windows.yaml"}, []string{"~/registries-windows.yaml"})
	if err != nil {
		return fmt.Errorf("error scp-ing registries-windows.yaml on bastion: %w", err)
	}

	return nil
}

func HasWindowsAgent(cluster *shared.Cluster) bool {
	if cluster.Config.Product == "rke2" && cluster.NumWinAgents > 0 {
		return true
	}
	return false
}
