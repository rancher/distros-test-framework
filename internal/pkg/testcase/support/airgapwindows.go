package support

import (
	"fmt"
	"os"
	"strings"
	"sync"

	. "github.com/onsi/gomega"

	"github.com/rancher/distros-test-framework/internal/resources"

	"github.com/rancher/distros-test-framework/internal/pkg/customflag"

	"github.com/rancher/distros-test-framework/internal/provisioning/driver"
)

func InstallOnAirgapAgentsWindows(cluster *driver.Cluster, airgapMethod string) {
	serverIP := cluster.ServerIPs[0]
	agentFlags := os.Getenv("worker_flags")
	if airgapMethod == SystemDefaultRegistry && !strings.Contains(agentFlags, "system-default-registry") {
		agentFlags += "`nsystem-default-registry: " + cluster.Bastion.PublicDNS
	}
	if strings.Contains(cluster.ServerIPs[0], ":") {
		serverIP = resources.EncloseSqBraces(serverIP)
	}

	for idx, agentIP := range cluster.WinAgentIPs {
		resources.LogLevel("info", "Install %v on Windows agent-%v...", cluster.Config.Product, idx+1)
		cmd := fmt.Sprintf(
			`powershell .\windows_install.ps1 "%v" "%v" "%v" "%v" "%v"`,
			serverIP, token, agentIP, airgapMethod, agentFlags)
		_, err := CmdForPrivateNode(cluster, cmd, agentIP)
		Expect(err).To(BeNil())
	}

	resources.LogLevel("info", "Waiting while Windows node joins...")
	nodeCount := cluster.NumServers + cluster.NumAgents + cluster.NumWinAgents
	Eventually(func(g Gomega) {
		res, err := GetNodesViaBastion(cluster)
		g.Expect(res).NotTo(BeEmpty())
		g.Expect(err).NotTo(HaveOccurred())

		nodes, err := resources.GetNodes(false)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(nodes).NotTo(BeEmpty())
		g.Expect(len(nodes)).To(Equal(nodeCount))
	}, "300s", "15s").Should(Succeed(), "Node count is not matching")
}

// ConfiguresRegistryWindows downloads Windows image file, reads and pushes to registry.
func ConfigureRegistryWindows(cluster *driver.Cluster, flags *customflag.FlagConfig) (err error) {
	resources.LogLevel("info", "Downloading %v artifacts for Windows...", cluster.Config.Product)
	_, err = GetArtifacts(cluster, "windows", flags.AirgapFlag.ImageRegistryUrl, flags.AirgapFlag.TarballType)
	if err != nil {
		return fmt.Errorf("error downloading %v artifacts: %w", cluster.Config.Product, err)
	}

	resources.LogLevel("info", "Perform image pull/tag/push/inspect...")
	err = podmanCmds(cluster, "windows", flags)
	if err != nil {
		return fmt.Errorf("error running podman commands: %w", err)
	}

	return nil
}

// CopyAssetsOnNodesWindows copies all the assets from bastion to Windows nodes.
func CopyAssetsOnNodesWindows(cluster *driver.Cluster, airgapMethod string) error {
	errChan := make(chan error, len(cluster.WinAgentIPs))
	var wg sync.WaitGroup

	for _, nodeIP := range cluster.WinAgentIPs {
		wg.Add(1)
		go func(ip string) {
			defer wg.Done()

			resources.LogLevel("debug", "Copying %v assets on Windows node IP: %s", cluster.Config.Product, ip)
			if err := copyAssetsOnWindows(cluster, airgapMethod, ip); err != nil {
				errChan <- resources.ReturnLogError("error copying assets on airgap node: %v\n, err: %w", ip, err)
				return
			}

			if airgapMethod == "private_registry" {
				resources.LogLevel("debug", "Copying registry.yaml on Windows node IP: %s", ip)
				if err := copyRegistryOnWindows(cluster, ip); err != nil {
					errChan <- resources.ReturnLogError("error copying registry to airgap node: %v\n, err: %w", ip, err)
					return
				}
			}
		}(nodeIP)
	}

	go func() {
		wg.Wait()
		close(errChan)
	}()

	for err := range errChan {
		if err != nil {
			return err
		}
	}

	return nil
}

func copyRegistryOnWindows(cluster *driver.Cluster, ip string) (err error) {
	cmd := fmt.Sprintf(
		"sudo %v registries-windows.yaml %v@%v:C:/Users/Administrator",
		ShCmdPrefix("scp", cluster.SSH.KeyName),
		"Administrator", ip)
	_, err = resources.RunCommandOnNode(cmd, cluster.Bastion.PublicIPv4Addr)
	if err != nil {
		return fmt.Errorf("error scp-ing registries.yaml on airgapped node: %v, \nerr: %w", ip, err)
	}

	return err
}

func copyAssetsOnWindows(cluster *driver.Cluster, airgapMethod, ip string) (err error) {
	windowsUser := "Administrator"
	cmd := fmt.Sprintf(
		"sudo chmod 400 /tmp/%v.pem && ", cluster.SSH.KeyName)

	cmd += fmt.Sprintf(
		"sudo %v artifacts-windows/* %v@%v:C:/Users/Administrator && ",
		ShCmdPrefix("scp", cluster.SSH.KeyName),
		windowsUser, ip)

	if airgapMethod != "tarball" {
		cmd += fmt.Sprintf(
			"sudo %v certs/* %v@%v:C:/Users/Administrator && ",
			ShCmdPrefix("scp", cluster.SSH.KeyName),
			windowsUser, ip)
	}

	cmd += fmt.Sprintf(
		"sudo %v rke2-install.ps1 windows_install.ps1 %v@%v:C:/Users/Administrator",
		ShCmdPrefix("scp", cluster.SSH.KeyName),
		windowsUser, ip)
	resources.LogLevel("debug", "Copy assets cmd Windows: %v", cmd)
	res, err := resources.RunCommandOnNode(cmd, cluster.Bastion.PublicIPv4Addr)
	if err != nil {
		resources.LogLevel("error", "error running cmd: %v, got res: %v", cmd, res)
	}

	return err
}

// UpdateRegistryFileWindows updates registries.yaml file and copies to bastion node for Windows.
func UpdateRegistryFileWindows(cluster *driver.Cluster, flags *customflag.FlagConfig) (err error) {
	regMap := map[string]string{
		"$PRIVATE_REG": cluster.Bastion.PublicDNS,
		"$USERNAME":    flags.AirgapFlag.RegistryUsername,
		"$PASSWORD":    flags.AirgapFlag.RegistryPassword,
		"$HOMEDIR":     "C:/Users/Administrator",
	}

	path := resources.BasePath() + "/infrastructure/legacy/airgap/setup"
	err = resources.CopyFileContents(path+"/registries.yaml.example", path+"/registries-windows.yaml")
	if err != nil {
		return fmt.Errorf("error copying registries-windows.yaml contents: %w", err)
	}

	err = resources.ReplaceFileContents(path+"/registries-windows.yaml", regMap)
	if err != nil {
		return fmt.Errorf("error replacing registries.yaml contents: %w", err)
	}

	err = resources.RunScp(
		cluster, cluster.Bastion.PublicIPv4Addr,
		[]string{path + "/registries-windows.yaml"}, []string{"~/registries-windows.yaml"})
	if err != nil {
		return fmt.Errorf("error scp-ing registries-windows.yaml on bastion: %w", err)
	}

	return nil
}

func HasWindowsAgent(cluster *driver.Cluster) bool {
	if cluster.Config.Product == "rke2" && cluster.NumWinAgents > 0 {
		return true
	}
	return false
}
