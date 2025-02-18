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

const (
	PrivateRegistry       = "private_registry"
	SystemDefaultRegistry = "system_default_registry"
	Tarball               = "tarball"
)

func BuildAirgapCluster(cluster *shared.Cluster) {
	Expect(cluster.Status).To(Equal("cluster created"))
	Expect(cluster.ServerIPs).ShouldNot(BeEmpty())

	if cluster.BastionConfig.PublicIPv4Addr != "" {
		shared.LogLevel("info", "Bastion Public IP: %v", cluster.BastionConfig.PublicIPv4Addr)
		shared.LogLevel("info", "Bastion Public DNS: %v", cluster.BastionConfig.PublicDNS)
	}
	shared.LogLevel("info", "Server Node IPs: %v", cluster.ServerIPs)

	LogAgentNodeIPs(cluster.NumAgents, cluster.AgentIPs, false)

	if cluster.Config.Product == "rke2" {
		LogAgentNodeIPs(cluster.NumWinAgents, cluster.WinAgentIPs, true)
	}
}

func InstallOnAirgapServers(cluster *shared.Cluster, airgapMethod string) {
	serverFlags := os.Getenv("server_flags")
	if airgapMethod == SystemDefaultRegistry && !strings.Contains(serverFlags, "system-default-registry") {
		serverFlags += "\nsystem-default-registry: " + cluster.BastionConfig.PublicDNS
	}

	for idx, serverIP := range cluster.ServerIPs {
		// Installing product on primary server aka server-1, saving the token.
		if idx == 0 {
			shared.LogLevel("info", "Installing %v on server-1...", cluster.Config.Product)
			cmd := fmt.Sprintf(
				"sudo chmod +x install_product.sh; "+
					`sudo ./install_product.sh "%v" "" "" "server" "%v" "%v"`,
				cluster.Config.Product, serverIP, serverFlags)
			_, err := CmdForPrivateNode(cluster, cmd, serverIP)
			Expect(err).To(BeNil(), err)

			cmd = fmt.Sprintf("sudo cat /var/lib/rancher/%v/server/token", cluster.Config.Product)
			token, err = CmdForPrivateNode(cluster, cmd, serverIP)
			Expect(err).To(BeNil(), err)
			Expect(token).NotTo(BeEmpty())
			shared.LogLevel("debug", "token: %v", token)
		}

		// Installing product on additional servers.
		if idx > 0 {
			shared.LogLevel("info", "Installing %v on server-%v...", cluster.Config.Product, idx+1)
			cmd := fmt.Sprintf(
				"sudo chmod +x install_product.sh; "+
					`sudo ./install_product.sh "%v" "%v" "%v" "server" "%v" "%v"`,
				cluster.Config.Product, cluster.ServerIPs[0], token, serverIP, serverFlags)
			_, err := CmdForPrivateNode(cluster, cmd, serverIP)
			Expect(err).To(BeNil(), err)
		}
	}
}

func InstallOnAirgapAgents(cluster *shared.Cluster, airgapMethod string) {
	agentFlags := os.Getenv("worker_flags")
	if cluster.Config.Product == "rke2" {
		if airgapMethod == SystemDefaultRegistry && !strings.Contains(agentFlags, "system-default-registry") {
			agentFlags += "\nsystem-default-registry: " + cluster.BastionConfig.PublicDNS
		}
	}

	for idx, agentIP := range cluster.AgentIPs {
		shared.LogLevel("info", "Installing %v on agent-%v...", cluster.Config.Product, idx+1)
		cmd := fmt.Sprintf(
			"sudo chmod +x install_product.sh; "+
				`sudo ./install_product.sh "%v" "%v" "%v" "agent" "%v" "%v"`,
			cluster.Config.Product, cluster.ServerIPs[0], token, agentIP, agentFlags)
		_, err := CmdForPrivateNode(cluster, cmd, agentIP)
		Expect(err).To(BeNil(), err)
	}
}

// SetupAirgapRegistry sets bastion node for airgap registry.
func SetupAirgapRegistry(cluster *shared.Cluster, flags *customflag.FlagConfig, airgapMethod string) error {
	shared.LogLevel("info", "Downloading %v artifacts...", cluster.Config.Product)
	_, err := GetArtifacts(cluster, flags.AirgapFlag.TarballType)
	if err != nil {
		return fmt.Errorf("error downloading artifacts: %w", err)
	}

	switch airgapMethod {
	case "private_registry":
		shared.LogLevel("info", "Adding private registry...")
		err = privateRegistry(cluster, flags)
		if err != nil {
			return fmt.Errorf("error adding private registry: %w", err)
		}
	case "system_default_registry":
		shared.LogLevel("info", "Adding system default registry...")
		err = systemDefaultRegistry(cluster)
		if err != nil {
			return fmt.Errorf("error adding system default registry: %w", err)
		}
	}

	shared.LogLevel("info", "Executing Docker pull/tag/push...")
	err = dockerActions(cluster, flags)
	if err != nil {
		return fmt.Errorf("error performing docker actions: %w", err)
	}

	return err
}

// privateRegistry executes private_registry.sh script.
func privateRegistry(cluster *shared.Cluster, flags *customflag.FlagConfig) (err error) {
	cmd := fmt.Sprintf(
		"sudo chmod +x private_registry.sh && "+
			`sudo ./private_registry.sh "%v" "%v" "%v"`,
		flags.AirgapFlag.RegistryUsername, flags.AirgapFlag.RegistryPassword,
		cluster.BastionConfig.PublicDNS)
	res, err := shared.RunCommandOnNode(cmd, cluster.BastionConfig.PublicIPv4Addr)
	if err != nil {
		shared.LogLevel("error", "failed execution of private_registry.sh: %v", res)
	}

	return err
}

// systemDefaultRegistry executes system_default_registry.sh script.
func systemDefaultRegistry(cluster *shared.Cluster) (err error) {
	cmd := fmt.Sprintf(
		"sudo chmod +x system_default_registry.sh && "+
			`sudo ./system_default_registry.sh "%v"`,
		cluster.BastionConfig.PublicDNS)
	_, err = shared.RunCommandOnNode(cmd, cluster.BastionConfig.PublicIPv4Addr)

	return err
}

// dockerActions executes docker_ops.sh script.
func dockerActions(cluster *shared.Cluster, flags *customflag.FlagConfig) (err error) {
	if flags.AirgapFlag.ImageRegistryUrl != "" {
		shared.LogLevel("info", "Images will be pulled from registry url: %v", flags.AirgapFlag.ImageRegistryUrl)
	}
	cmd := "sudo chmod +x docker_ops.sh && " +
		fmt.Sprintf(
			`sudo ./docker_ops.sh "%v" "%v" "%v" "%v" "%v"`,
			cluster.Config.Product, cluster.BastionConfig.PublicDNS,
			flags.AirgapFlag.RegistryUsername, flags.AirgapFlag.RegistryPassword,
			flags.AirgapFlag.ImageRegistryUrl)
	_, err = shared.RunCommandOnNode(cmd, cluster.BastionConfig.PublicIPv4Addr)

	return err
}

// CopyAssetsOnNodes copies all the assets from bastion to private nodes.
func CopyAssetsOnNodes(cluster *shared.Cluster, airgapMethod string, tarballType *string) error {
	nodeIPs := cluster.ServerIPs
	nodeIPs = append(nodeIPs, cluster.AgentIPs...)
	errChan := make(chan error, len(nodeIPs))
	var err error
	var wg sync.WaitGroup

	for _, nodeIP := range nodeIPs {
		wg.Add(1)
		go func(nodeIP string) {
			defer wg.Done()
			shared.LogLevel("debug", "Copying %v assets on node IP: %s", cluster.Config.Product, nodeIP)
			err = copyAssets(cluster, airgapMethod, nodeIP)
			if err != nil {
				errChan <- shared.ReturnLogError("error copying assets on airgap node: %v\n, err: %w", nodeIP, err)
			}
			switch airgapMethod {
			case "private_registry":
				shared.LogLevel("debug", "Copying registry.yaml on node IP: %s", nodeIP)
				err = copyRegistry(cluster, nodeIP)
				if err != nil {
					errChan <- shared.ReturnLogError("error copying registry to airgap node: %v\n, err: %w", nodeIP, err)
				}
			case "system_default_registry":
				shared.LogLevel("debug", "Trust CA Certs on node IP: %s", nodeIP)
				err = trustCert(cluster, nodeIP)
				if err != nil {
					errChan <- shared.ReturnLogError("error trusting ssl cert on airgap node: %v\n, err: %w", nodeIP, err)
				}
			case "tarball":
				shared.LogLevel("debug", "Copying tarball on node IP: %s", nodeIP)
				err = copyTarball(cluster, nodeIP, *tarballType)
				if err != nil {
					errChan <- shared.ReturnLogError("error copying tarball on airgap node: %v\n, err: %w", nodeIP, err)
				}
			}
			shared.LogLevel("debug", "Make %s executable on node IP: %s", cluster.Config.Product, nodeIP)
			err = makeExecs(cluster, nodeIP)
			if err != nil {
				errChan <- shared.ReturnLogError("error making asset exec on airgap node: %v\n, err: %w", nodeIP, err)
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

func copyTarball(cluster *shared.Cluster, ip, tarballType string) (err error) {
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
func trustCert(cluster *shared.Cluster, ip string) (err error) {
	// TODO: Implement for rhel, sles
	cmd := "sudo cp domain.crt /usr/local/share/ca-certificates/domain.crt && " +
		"sudo update-ca-certificates"
	_, err = CmdForPrivateNode(cluster, cmd, ip)

	return err
}

// copyAssets copies assets from bastion to private node.
func copyAssets(cluster *shared.Cluster, airgapMethod, ip string) (err error) {
	cmd := fmt.Sprintf(
		"sudo chmod 400 /tmp/%v.pem && ", cluster.Aws.KeyName)

	switch cluster.Config.Product {
	case "rke2":
		cmd += fmt.Sprintf(
			"sudo %v -r artifacts %v@%v:~/ && ",
			ShCmdPrefix("scp", cluster.Aws.KeyName),
			cluster.Aws.AwsUser, ip)
	case "k3s":
		cmd += fmt.Sprintf(
			"sudo %v %v* %v@%v:~/ && ",
			ShCmdPrefix("scp", cluster.Aws.KeyName),
			cluster.Config.Product,
			cluster.Aws.AwsUser, ip)
	}

	if airgapMethod != "tarball" {
		cmd += fmt.Sprintf(
			"sudo %v certs/* %v@%v:~/ && ",
			ShCmdPrefix("scp", cluster.Aws.KeyName),
			cluster.Aws.AwsUser, ip)
	}

	cmd += fmt.Sprintf(
		"sudo %v install_product.sh %v-install.sh %v@%v:~/",
		ShCmdPrefix("scp", cluster.Aws.KeyName),
		cluster.Config.Product,
		cluster.Aws.AwsUser, ip)
	_, err = shared.RunCommandOnNode(cmd, cluster.BastionConfig.PublicIPv4Addr)

	return err
}

// copyRegistry copies registries.yaml from bastion on to private node.
func copyRegistry(cluster *shared.Cluster, ip string) (err error) {
	cmd := fmt.Sprintf(
		"sudo %v registries.yaml %v@%v:~/",
		ShCmdPrefix("scp", cluster.Aws.KeyName),
		cluster.Aws.AwsUser, ip)
	_, err = shared.RunCommandOnNode(cmd, cluster.BastionConfig.PublicIPv4Addr)
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
func CmdForPrivateNode(cluster *shared.Cluster, cmd, ip string) (res string, err error) {
	serverCmd := fmt.Sprintf(
		"%v %v@%v '%v'",
		ShCmdPrefix("ssh", cluster.Aws.KeyName),
		cluster.Aws.AwsUser, ip, cmd)
	shared.LogLevel("debug", "Cmd on bastion node: %v", serverCmd)
	res, err = shared.RunCommandOnNode(serverCmd, cluster.BastionConfig.PublicIPv4Addr)

	return res, err
}

// GetArtifacts executes get_artifacts.sh script.
func GetArtifacts(cluster *shared.Cluster, tarballType string) (res string, err error) {
	serverFlags := os.Getenv("server_flags")
	cmd := fmt.Sprintf(
		"sudo chmod +x get_artifacts.sh && "+
			`sudo ./get_artifacts.sh "%v" "%v" "linux" "%v" "%v" "%v"`,
		cluster.Config.Product, cluster.Config.Version,
		cluster.Config.Arch, serverFlags, tarballType)
	res, err = shared.RunCommandOnNode(cmd, cluster.BastionConfig.PublicIPv4Addr)

	return res, err
}

// GetWindowsArtifacts executes get_artifacts.sh script for windows agent.
func GetWindowsArtifacts(cluster *shared.Cluster, tarballType string) (res string, err error) {
	serverFlags := os.Getenv("server_flags")
	cmd := fmt.Sprintf(
		"sudo chmod +x get_artifacts.sh && "+
			`sudo ./get_artifacts.sh "%v" "%v" "windows" "%v" "%v" "%v"`,
		cluster.Config.Product, cluster.Config.Version,
		cluster.Config.Arch, serverFlags, tarballType)
	res, err = shared.RunCommandOnNode(cmd, cluster.BastionConfig.PublicIPv4Addr)

	return res, err
}

// makeExecs gives permission to files that makes them executable.
func makeExecs(cluster *shared.Cluster, ip string) (err error) {
	cmd := fmt.Sprintf("sudo chmod +x %v-install.sh", cluster.Config.Product)
	if cluster.Config.Product == "k3s" {
		cmd += "; sudo cp k3s /usr/local/bin/k3s; " +
			"sudo chmod +x /usr/local/bin/k3s"
	}
	_, err = CmdForPrivateNode(cluster, cmd, ip)

	return err
}

// UpdateRegistryFile updates registries.yaml file and copies to bastion node.
func UpdateRegistryFile(cluster *shared.Cluster, flags *customflag.FlagConfig) (err error) {
	pwd, err := shared.RunCommandOnNode("pwd", cluster.BastionConfig.PublicIPv4Addr)
	if err != nil {
		return fmt.Errorf("error running pwd command: %w", err)
	}

	regMap := map[string]string{
		"$PRIVATE_REG": cluster.BastionConfig.PublicDNS,
		"$USERNAME":    flags.AirgapFlag.RegistryUsername,
		"$PASSWORD":    flags.AirgapFlag.RegistryPassword,
		"$HOMEDIR":     pwd,
	}

	path := shared.BasePath() + "/modules/airgap/setup"
	err = shared.CopyFileContents(path+"/registries.yaml.example", path+"/registries.yaml")
	if err != nil {
		return fmt.Errorf("error copying registries.yaml contents: %w", err)
	}

	err = shared.ReplaceFileContents(path+"/registries.yaml", regMap)
	if err != nil {
		return fmt.Errorf("error replacing registries.yaml contents: %w", err)
	}

	err = shared.RunScp(
		cluster, cluster.BastionConfig.PublicIPv4Addr,
		[]string{path + "/registries.yaml"}, []string{"~/registries.yaml"})
	if err != nil {
		return fmt.Errorf("error scp-ing registries.yaml on bastion: %w", err)
	}

	return nil
}

// LogClusterInfoUsingBastion executes and prints kubectl get nodes,pods on bastion.
func LogClusterInfoUsingBastion(cluster *shared.Cluster) {
	shared.LogLevel("info", "Bastion login: ssh -i %v.pem %v@%v",
		cluster.Aws.KeyName, cluster.Aws.AwsUser,
		cluster.BastionConfig.PublicIPv4Addr)

	cmd := fmt.Sprintf(
		"PATH=$PATH:/var/lib/rancher/%[1]v/bin:/opt/%[1]v/bin; "+
			"KUBECONFIG=/etc/rancher/%[1]v/%[1]v.yaml ",
		cluster.Config.Product)
	cmd += "kubectl get nodes,pods -A -o wide"

	shared.LogLevel("info", "Display cluster details server-1: %v", cmd)
	clusterInfo, err := CmdForPrivateNode(cluster, cmd, cluster.ServerIPs[0])
	if err != nil {
		shared.LogLevel("error", "Error getting airgap cluster details: %v", err)
	}
	shared.LogLevel("info", "\n%v", clusterInfo)
}

// ShCmdPrefix adds prefix to shell commands.
func ShCmdPrefix(cmdType, keyName string) (cmd string) {
	if cmdType != "scp" && cmdType != "ssh" {
		shared.LogLevel("error", "Invalid shell command type: %v", cmdType)
	}
	cmd = cmdType + fmt.Sprintf(
		" -i /tmp/%v.pem -o StrictHostKeyChecking=no -o IdentitiesOnly=yes",
		keyName)

	return cmd
}
