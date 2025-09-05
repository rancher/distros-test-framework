package support

import (
	"fmt"
	"net/url"
	"slices"
	"strings"
	"sync"

	"github.com/rancher/distros-test-framework/internal/resources"

	"github.com/rancher/distros-test-framework/internal/pkg/customflag"

	. "github.com/onsi/gomega"
)

const (
	PrivateRegistry       = "private_registry"
	SystemDefaultRegistry = "system_default_registry"
	Tarball               = "tarball"
)

func BuildAirgapCluster(cluster *resources.Cluster) {
	Expect(cluster.Status).To(Equal("cluster created"))
	Expect(cluster.ServerIPs).ShouldNot(BeEmpty())

	if cluster.Bastion.PublicIPv4Addr != "" {
		resources.LogLevel("info", "Bastion Public IP: %v", cluster.Bastion.PublicIPv4Addr)
		resources.LogLevel("info", "Bastion Public DNS: %v", cluster.Bastion.PublicDNS)
	}
	resources.LogLevel("info", "Server Node IPs: %v", cluster.ServerIPs)

	LogAgentNodeIPs(cluster.NumAgents, cluster.AgentIPs, false)

	if cluster.Config.Product == "rke2" {
		LogAgentNodeIPs(cluster.NumWinAgents, cluster.WinAgentIPs, true)
	}
}

func InstallOnAirgapServers(cluster *resources.Cluster, airgapMethod string) {
	if airgapMethod == SystemDefaultRegistry && !strings.Contains(cluster.Config.ServerFlags, "system-default-registry") {
		cluster.Config.ServerFlags += "\nsystem-default-registry: " + cluster.Bastion.PublicDNS
	}

	for idx, serverIP := range cluster.ServerIPs {
		// Installing product on primary server aka server-1, saving the token.
		if idx == 0 {
			resources.LogLevel("info", "Installing %v on server-1...", cluster.Config.Product)
			cmd := fmt.Sprintf(
				"sudo chmod +x install_product.sh; "+
					`sudo ./install_product.sh "%v" "" "" "server" "%v" "%v"`,
				cluster.Config.Product, serverIP, cluster.Config.ServerFlags)
			_, err := CmdForPrivateNode(cluster, cmd, serverIP)
			Expect(err).To(BeNil(), err)

			cmd = fmt.Sprintf("sudo cat /var/lib/rancher/%v/server/token", cluster.Config.Product)
			token, err = CmdForPrivateNode(cluster, cmd, serverIP)
			Expect(err).To(BeNil(), err)
			Expect(token).NotTo(BeEmpty())
			resources.LogLevel("debug", "token: %v", token)
		}

		// Installing product on additional servers.
		if idx > 0 {
			resources.LogLevel("info", "Installing %v on server-%v...", cluster.Config.Product, idx+1)
			cmd := fmt.Sprintf(
				"sudo chmod +x install_product.sh; "+
					`sudo ./install_product.sh "%v" "%v" "%v" "server" "%v" "%v"`,
				cluster.Config.Product, cluster.ServerIPs[0], token, serverIP, cluster.Config.ServerFlags)
			_, err := CmdForPrivateNode(cluster, cmd, serverIP)
			Expect(err).To(BeNil(), err)
		}
	}

	resources.LogLevel("info", "Process kubeconfig from primary server node: %v", cluster.ServerIPs[0])
	err := processKubeconfigOnBastion(cluster)
	if err != nil {
		resources.LogLevel("error", "unable to get kubeconfig\n%w", err)
	}
	resources.LogLevel("info", "Process kubeconfig: Complete!")
}

func InstallOnAirgapAgents(cluster *resources.Cluster, airgapMethod string) {
	if cluster.Config.Product == "rke2" {
		if airgapMethod == SystemDefaultRegistry &&
			!strings.Contains(cluster.Config.WorkerFlags, "system-default-registry") {
			cluster.Config.WorkerFlags += "\nsystem-default-registry: " + cluster.Bastion.PublicDNS
		}
	}

	for idx, agentIP := range cluster.AgentIPs {
		resources.LogLevel("info", "Installing %v on agent-%v...", cluster.Config.Product, idx+1)
		cmd := fmt.Sprintf(
			"sudo chmod +x install_product.sh; "+
				`sudo ./install_product.sh "%v" "%v" "%v" "agent" "%v" "%v"`,
			cluster.Config.Product, cluster.ServerIPs[0], token, agentIP, cluster.Config.WorkerFlags)
		_, err := CmdForPrivateNode(cluster, cmd, agentIP)
		Expect(err).To(BeNil(), err)
	}
}

// SetupAirgapRegistry sets bastion node for airgap registry.
func SetupAirgapRegistry(cluster *resources.Cluster, flags *customflag.FlagConfig, airgapMethod string) (err error) {
	resources.LogLevel("info", "Downloading %v artifacts...", cluster.Config.Product)
	_, err = GetArtifacts(cluster, "linux", flags.AirgapFlag.ImageRegistryUrl, flags.AirgapFlag.TarballType)
	if err != nil {
		return fmt.Errorf("error downloading %v artifacts: %w", cluster.Config.Product, err)
	}

	switch airgapMethod {
	case "private_registry":
		resources.LogLevel("info", "Adding private registry...")
		err = privateRegistry(cluster, flags)
		if err != nil {
			return fmt.Errorf("error adding private registry: %w", err)
		}
	case "system_default_registry":
		resources.LogLevel("info", "Adding system default registry...")
		err = systemDefaultRegistry(cluster)
		if err != nil {
			return fmt.Errorf("error adding system default registry: %w", err)
		}
	default:
		resources.LogLevel("error", "Invalid airgap method or not yet implemented: %s", airgapMethod)
	}

	resources.LogLevel("info", "Perform image pull/tag/push...")
	err = podmanCmds(cluster, "linux", flags)
	if err != nil {
		return fmt.Errorf("error performing docker actions: %w", err)
	}

	return err
}

// privateRegistry executes private_registry.sh script.
func privateRegistry(cluster *resources.Cluster, flags *customflag.FlagConfig) (err error) {
	cmd := fmt.Sprintf(
		"sudo chmod +x private_registry.sh && "+
			`sudo ./private_registry.sh "%v" "%v" "%v"`,
		flags.AirgapFlag.RegistryUsername, flags.AirgapFlag.RegistryPassword,
		cluster.Bastion.PublicDNS)
	res, err := resources.RunCommandOnNode(cmd, cluster.Bastion.PublicIPv4Addr)
	if err != nil {
		resources.LogLevel("error", "failed execution of private_registry.sh: %v", res)
	}

	return err
}

// systemDefaultRegistry executes system_default_registry.sh script.
func systemDefaultRegistry(cluster *resources.Cluster) (err error) {
	cmd := fmt.Sprintf(
		"sudo chmod +x system_default_registry.sh && "+
			`sudo ./system_default_registry.sh "%v"`,
		cluster.Bastion.PublicDNS)
	_, err = resources.RunCommandOnNode(cmd, cluster.Bastion.PublicIPv4Addr)

	return err
}

// podmanCmds executes podman_cmds.sh script.
func podmanCmds(cluster *resources.Cluster, platform string, flags *customflag.FlagConfig) (err error) {
	cmd := "sudo chmod +x podman_cmds.sh && " +
		fmt.Sprintf(`sudo ./podman_cmds.sh "%v" "%v" "%v" "%v" "%v" "%v"`,
			cluster.Config.Product, platform, cluster.Bastion.PublicDNS,
			flags.AirgapFlag.RegistryUsername, flags.AirgapFlag.RegistryPassword,
			flags.AirgapFlag.ImageRegistryUrl)
	_, err = resources.RunCommandOnNode(cmd, cluster.Bastion.PublicIPv4Addr)

	return err
}

// CopyAssetsOnNodes copies all the assets from bastion to private nodes.
func CopyAssetsOnNodes(cluster *resources.Cluster, airgapMethod string, tarballType *string) (err error) {
	nodeIPs := cluster.ServerIPs
	nodeIPs = append(nodeIPs, cluster.AgentIPs...)
	errChan := make(chan error, len(nodeIPs))
	var wg sync.WaitGroup

	for _, nodeIP := range nodeIPs {
		wg.Add(1)
		go func(nodeIP string) {
			defer wg.Done()
			resources.LogLevel("debug", "Copying %v assets on node IP: %s", cluster.Config.Product, nodeIP)
			err = copyAssets(cluster, airgapMethod, nodeIP)
			if err != nil {
				errChan <- resources.ReturnLogError("error copying assets on airgap node: %v\n, err: %w", nodeIP, err)
			}
			switch airgapMethod {
			case "private_registry":
				resources.LogLevel("debug", "Copying registry.yaml on node IP: %s", nodeIP)
				err = copyRegistry(cluster, nodeIP)
				if err != nil {
					errChan <- resources.ReturnLogError("error copying registry to airgap node: %v\n, err: %w", nodeIP, err)
				}
			case "system_default_registry":
				resources.LogLevel("debug", "Trust CA Certs on node IP: %s", nodeIP)
				err = trustCert(cluster, nodeIP)
				if err != nil {
					errChan <- resources.ReturnLogError("error trusting ssl cert on airgap node: %v\n, err: %w", nodeIP, err)
				}
			case "tarball":
				resources.LogLevel("debug", "Copying tarball on node IP: %s", nodeIP)
				err = copyTarball(cluster, *tarballType, nodeIP)
				if err != nil {
					errChan <- resources.ReturnLogError("error copying tarball on airgap node: %v\n, err: %w", nodeIP, err)
				}
			default:
				resources.LogLevel("error", "Invalid airgap method: %s", airgapMethod)
			}
			resources.LogLevel("debug", "Make %s executable on node IP: %s", cluster.Config.Product, nodeIP)
			err = makeExecutable(cluster, nodeIP)
			if err != nil {
				errChan <- resources.ReturnLogError("error making asset exec on airgap node: %v\n, err: %w", nodeIP, err)
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

func copyTarball(cluster *resources.Cluster, tarballType, ip string) (err error) {
	imgDir := "/var/lib/rancher/" + cluster.Config.Product + "/agent/images"
	cmd := "sudo mkdir -p " + imgDir + "; "
	if cluster.Config.Product == "rke2" {
		cmd += fmt.Sprintf("sudo cp artifacts/*.%v %v", tarballType, imgDir)
	} else {
		cmd += fmt.Sprintf("sudo cp *.%v %v", tarballType, imgDir)
	}
	_, err = CmdForPrivateNode(cluster, cmd, ip)

	return err
}

// trustCert copied certs from bastion and updates ca certs.
func trustCert(cluster *resources.Cluster, ip string) (err error) {
	// TODO: Implement for rhel, sles
	cmd := "sudo cp domain.crt /usr/local/share/ca-certificates/domain.crt && " +
		"sudo update-ca-certificates"
	_, err = CmdForPrivateNode(cluster, cmd, ip)

	return err
}

// copyAssets copies assets from bastion to private node.
func copyAssets(cluster *resources.Cluster, airgapMethod, ip string) (err error) {
	cmd := fmt.Sprintf(
		"sudo chmod 400 /tmp/%v.pem && ", cluster.SSH.KeyName)

	switch cluster.Config.Product {
	case "rke2":
		cmd += fmt.Sprintf(
			"sudo %v -r artifacts %v@%v:~/ && ",
			ShCmdPrefix("scp", cluster.SSH.KeyName),
			cluster.SSH.User, ip)
	case "k3s":
		cmd += fmt.Sprintf(
			"sudo %v %v* %v@%v:~/ && ",
			ShCmdPrefix("scp", cluster.SSH.KeyName),
			cluster.Config.Product,
			cluster.SSH.User, ip)
	}

	if airgapMethod != "tarball" {
		cmd += fmt.Sprintf(
			"sudo %v certs/* %v@%v:~/ && ",
			ShCmdPrefix("scp", cluster.SSH.KeyName),
			cluster.SSH.User, ip)
	}

	cmd += fmt.Sprintf(
		"sudo %v install_product.sh %v-install.sh %v@%v:~/",
		ShCmdPrefix("scp", cluster.SSH.KeyName),
		cluster.Config.Product,
		cluster.SSH.User, ip)
	_, err = resources.RunCommandOnNode(cmd, cluster.Bastion.PublicIPv4Addr)

	return err
}

// copyRegistry copies registries.yaml from bastion on to private node.
func copyRegistry(cluster *resources.Cluster, ip string) (err error) {
	cmd := fmt.Sprintf(
		"sudo %v registries.yaml %v@%v:~/",
		ShCmdPrefix("scp", cluster.SSH.KeyName),
		cluster.SSH.User, ip)
	_, err = resources.RunCommandOnNode(cmd, cluster.Bastion.PublicIPv4Addr)
	if err != nil {
		return fmt.Errorf("error scp-ing registries.yaml on airgapped node: %v, \nerr: %w", ip, err)
	}

	cmd = fmt.Sprintf(
		"sudo mkdir -p /etc/rancher/%[1]v && "+
			"sudo cp registries.yaml /etc/rancher/%[1]v",
		cluster.Config.Product)
	_, err = CmdForPrivateNode(cluster, cmd, ip)

	return err
}

// CmdForPrivateNode command to run on private node via bastion.
func CmdForPrivateNode(cluster *resources.Cluster, cmd, ip string) (res string, err error) {
	awsUser := cluster.SSH.User
	if HasWindowsAgent(cluster) {
		if slices.Contains(cluster.WinAgentIPs, ip) {
			awsUser = "Administrator"
		}
	}

	serverCmd := fmt.Sprintf(
		"%v %v@%v '%v'",
		ShCmdPrefix("ssh", cluster.SSH.KeyName),
		awsUser, ip, cmd)
	resources.LogLevel("debug", "Cmd on bastion node: %v", serverCmd)

	res, err = resources.RunCommandOnNode(serverCmd, cluster.Bastion.PublicIPv4Addr)
	if err != nil {
		return "", fmt.Errorf("error running command on private node %v: %w", ip, err)
	}

	return res, err
}

// GetArtifacts executes get_artifacts.sh script.
func GetArtifacts(cluster *resources.Cluster, platform, registryURL, tarballType string) (res string, err error) {
	version := cluster.Config.Version
	if platform == "" {
		platform = "linux"
	}
	if registryURL != "" {
		resources.LogLevel("info", "Getting artifacts from URL: %v", registryURL)
		version = url.QueryEscape(cluster.Config.Version)
	}
	cmd := fmt.Sprintf(
		"sudo chmod +x get_artifacts.sh && "+
			`sudo ./get_artifacts.sh "%v" "%v" "%v" "%v" "%v" "%v" "%v"`,
		cluster.Config.Product, version, platform,
		cluster.Config.Arch, registryURL, cluster.Config.ServerFlags, tarballType)
	res, err = resources.RunCommandOnNode(cmd, cluster.Bastion.PublicIPv4Addr)

	return res, err
}

// makeExecutable gives necessary permission to files through chmod.
func makeExecutable(cluster *resources.Cluster, ip string) (err error) {
	cmd := fmt.Sprintf("sudo chmod +x %v-install.sh", cluster.Config.Product)
	if cluster.Config.Product == "k3s" {
		cmd += "; sudo cp k3s /usr/local/bin/k3s; " +
			"sudo chmod +x /usr/local/bin/k3s"
	}
	_, err = CmdForPrivateNode(cluster, cmd, ip)

	return err
}

// UpdateRegistryFile updates registries.yaml file and copies to bastion node.
func UpdateRegistryFile(cluster *resources.Cluster, flags *customflag.FlagConfig) (err error) {
	pwd, err := resources.RunCommandOnNode("pwd", cluster.Bastion.PublicIPv4Addr)
	if err != nil {
		return fmt.Errorf("error running pwd command: %w", err)
	}

	regMap := map[string]string{
		"$PRIVATE_REG": cluster.Bastion.PublicDNS,
		"$USERNAME":    flags.AirgapFlag.RegistryUsername,
		"$PASSWORD":    flags.AirgapFlag.RegistryPassword,
		"$HOMEDIR":     pwd,
	}

	path := resources.BasePath() + "/infrastructure/legacy/airgap/setup"
	err = resources.CopyFileContents(path+"/registries.yaml.example", path+"/registries.yaml")
	if err != nil {
		return fmt.Errorf("error copying registries.yaml contents: %w", err)
	}

	err = resources.ReplaceFileContents(path+"/registries.yaml", regMap)
	if err != nil {
		return fmt.Errorf("error replacing registries.yaml contents: %w", err)
	}

	err = resources.RunScp(
		cluster, cluster.Bastion.PublicIPv4Addr,
		[]string{path + "/registries.yaml"}, []string{"~/registries.yaml"})
	if err != nil {
		return fmt.Errorf("error scp-ing registries.yaml on bastion: %w", err)
	}

	return nil
}

func processKubeconfigOnBastion(cluster *resources.Cluster) (err error) {
	var localNodeIP string
	kcFileName := cluster.Config.Product + "_kubeconf.yaml"
	serverIP := cluster.ServerIPs[0]
	if strings.Contains(serverIP, ":") {
		localNodeIP = `\[::1\]`
		serverIP = resources.EncloseSqBraces(serverIP)
	} else {
		localNodeIP = "127.0.0.1"
	}

	cmd := fmt.Sprintf(
		`sudo %[1]v %[2]v@%[3]v:/etc/rancher/%[4]v/%[4]v.yaml %[5]v && `,
		ShCmdPrefix("scp", cluster.SSH.KeyName),
		cluster.SSH.User, serverIP,
		cluster.Config.Product, kcFileName,
	)

	cmd += fmt.Sprintf(`sudo sed 's/%[1]v/%[2]v/g' $HOME/%[3]v > /tmp/%[3]v`,
		localNodeIP, serverIP, kcFileName,
	)

	_, err = resources.RunCommandOnNode(cmd, cluster.Bastion.PublicIPv4Addr)
	if err != nil {
		return err
	}

	return nil
}

// LogClusterInfoUsingBastion executes and prints kubectl get nodes,pods on bastion.
func LogClusterInfoUsingBastion(cluster *resources.Cluster) {
	resources.LogLevel("info", "Bastion login: ssh -i %v.pem %v@%v",
		cluster.SSH.KeyName, cluster.SSH.User,
		cluster.Bastion.PublicIPv4Addr)

	cmd := fmt.Sprintf(
		"KUBECONFIG=/tmp/%v_kubeconf.yaml kubectl get nodes,pods -A -o wide",
		cluster.Config.Product)
	resources.LogLevel("info", "Display cluster details from bastion: %v", cmd)
	clusterInfo, err := resources.RunCommandOnNode(cmd, cluster.Bastion.PublicIPv4Addr)
	if err != nil {
		resources.LogLevel("error", "Error getting airgap cluster details: %v", err)
	}
	resources.LogLevel("info", "\n%v", clusterInfo)
}

// ShCmdPrefix adds prefix to shell commands.
func ShCmdPrefix(cmdType, keyName string) (cmd string) {
	if cmdType != "scp" && cmdType != "ssh" {
		resources.LogLevel("error", "Invalid shell command type: %v", cmdType)
	}
	cmd = cmdType + fmt.Sprintf(
		" -i /tmp/%v.pem -o StrictHostKeyChecking=no -o IdentitiesOnly=yes",
		keyName)

	return cmd
}
