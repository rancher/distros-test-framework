package shared

import (
	"fmt"
	"os"
	"sync"

	"github.com/rancher/distros-test-framework/pkg/customflag"
)

// SetupPrivateRegistry sets bastion node as private registry.
func SetupPrivateRegistry(cluster *Cluster, flags *customflag.FlagConfig) error {
	LogLevel("info", "Downloading %v artifacts...", cluster.Config.Product)
	_, err := getArtifacts(cluster, flags)
	if err != nil {
		return fmt.Errorf("error downloading artifacts: %w", err)
	}

	LogLevel("info", "Adding private registry...")
	err = bastionAsPrivateRegistry(cluster, flags)
	if err != nil {
		return fmt.Errorf("error adding private registry: %w", err)
	}

	LogLevel("info", "Executing Docker pull/tag/push...")
	err = dockerActions(cluster, flags)
	if err != nil {
		return fmt.Errorf("error performing docker actions: %w", err)
	}

	pwd, err := RunCommandOnNode("pwd", cluster.BastionConfig.PublicIPv4Addr)
	if err != nil {
		return fmt.Errorf("error running pwd command: %w", err)
	}
	regMap := map[string]string{
		"$PRIVATE_REG": cluster.BastionConfig.PublicDNS,
		"$USERNAME":    flags.AirgapFlag.RegistryUsername,
		"$PASSWORD":    flags.AirgapFlag.RegistryPassword,
		"$HOMEDIR":     pwd,
	}

	LogLevel("info", "Updating and copying registries.yaml on bastion...")
	err = updateRegistryFile(cluster, regMap)
	if err != nil {
		return fmt.Errorf("error updating/copying registries.yaml: %w", err)
	}

	return err
}

// bastionAsPrivateRegistry executes private_registry.sh script.
func bastionAsPrivateRegistry(cluster *Cluster, flags *customflag.FlagConfig) (err error) {
	cmd := fmt.Sprintf(
		"sudo chmod +x private_registry.sh && "+
			`sudo ./private_registry.sh "%v" "%v" "%v"`,
		flags.AirgapFlag.RegistryUsername, flags.AirgapFlag.RegistryPassword,
		cluster.BastionConfig.PublicDNS)
	_, err = RunCommandOnNode(cmd, cluster.BastionConfig.PublicIPv4Addr)

	return err
}

// dockerActions executes docker_ops.sh script.
func dockerActions(cluster *Cluster, flags *customflag.FlagConfig) (err error) {
	if flags.AirgapFlag.ImageRegistryUrl != "" {
		LogLevel("info", "Images will be pulled from registry url: %v", flags.AirgapFlag.ImageRegistryUrl)
	}
	cmd := "sudo chmod +x docker_ops.sh && " +
		fmt.Sprintf(
			`sudo ./docker_ops.sh "%v" "%v" "%v" "%v" "%v"`,
			cluster.Config.Product, cluster.BastionConfig.PublicDNS,
			flags.AirgapFlag.RegistryUsername, flags.AirgapFlag.RegistryPassword,
			flags.AirgapFlag.ImageRegistryUrl)
	_, err = RunCommandOnNode(cmd, cluster.BastionConfig.PublicIPv4Addr)

	return err
}

// CopyAssetsOnNodes copies all the assets from bastion to private nodes.
func CopyAssetsOnNodes(cluster *Cluster) error {
	nodeIPs := cluster.ServerIPs
	nodeIPs = append(nodeIPs, cluster.AgentIPs...)

	errChan := make(chan error, len(nodeIPs))
	var err error
	var wg sync.WaitGroup

	for _, nodeIP := range nodeIPs {
		wg.Add(1)
		go func(nodeIP string) {
			defer wg.Done()
			err = copyAssets(cluster, nodeIP)
			if err != nil {
				errChan <- ReturnLogError("error copying assets on airgap node: %v\n, err: %w", nodeIP, err)
			}
			err = copyRegistry(cluster, nodeIP)
			if err != nil {
				errChan <- ReturnLogError("error copying registry to airgap node: %v\n, err: %w", nodeIP, err)
			}
			err = makeExecs(cluster, nodeIP)
			if err != nil {
				errChan <- ReturnLogError("error making asset exec on airgap node: %v\n, err: %w", nodeIP, err)
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

// copyAssets copies assets from bastion to private node.
func copyAssets(cluster *Cluster, ip string) (err error) {
	cmd := fmt.Sprintf(
		"sudo chmod 400 /tmp/%v.pem && ", cluster.AwsEc2.KeyName)
	switch cluster.Config.Product {
	case "rke2":
		cmd += fmt.Sprintf(
			"sudo %v -r artifacts %v@%v:~/ && ",
			ssPrefix("scp", cluster.AwsEc2.KeyName),
			cluster.AwsEc2.AwsUser, ip)
	case "k3s":
		cmd += fmt.Sprintf(
			"sudo %v %v* %v@%v:~/ && ",
			ssPrefix("scp", cluster.AwsEc2.KeyName),
			cluster.Config.Product,
			cluster.AwsEc2.AwsUser, ip)
	}
	cmd += fmt.Sprintf(
		"sudo %v certs/* install_product.sh %v-install.sh %v@%v:~/",
		ssPrefix("scp", cluster.AwsEc2.KeyName),
		cluster.Config.Product,
		cluster.AwsEc2.AwsUser, ip)
	_, err = RunCommandOnNode(cmd, cluster.BastionConfig.PublicIPv4Addr)

	return err
}

// copyRegistry copies registries.yaml from bastion on private node.
func copyRegistry(cluster *Cluster, ip string) (err error) {
	cmd := fmt.Sprintf(
		"sudo %v registries.yaml %v@%v:~/",
		ssPrefix("scp", cluster.AwsEc2.KeyName),
		cluster.AwsEc2.AwsUser, ip)
	_, err = RunCommandOnNode(cmd, cluster.BastionConfig.PublicIPv4Addr)
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
func CmdForPrivateNode(cluster *Cluster, cmd, ip string) (res string, err error) {
	serverCmd := fmt.Sprintf(
		"%v %v@%v '%v'",
		ssPrefix("ssh", cluster.AwsEc2.KeyName),
		cluster.AwsEc2.AwsUser, ip, cmd)
	LogLevel("debug", "Cmd on bastion node: "+serverCmd)
	res, err = RunCommandOnNode(serverCmd, cluster.BastionConfig.PublicIPv4Addr)

	return res, err
}

// getArtifacts executes get_artifacts.sh scripts.
func getArtifacts(cluster *Cluster, flags *customflag.FlagConfig) (res string, err error) {
	serverFlags := os.Getenv("server_flags")
	cmd := fmt.Sprintf(
		"sudo chmod +x get_artifacts.sh && "+
			`sudo ./get_artifacts.sh "%v" "%v" "%v" "%v" "%v"`,
		cluster.Config.Product, cluster.Config.Version,
		cluster.Config.Arch, serverFlags, flags.AirgapFlag.TarballType)
	res, err = RunCommandOnNode(cmd, cluster.BastionConfig.PublicIPv4Addr)

	return res, err
}

// makeExecs gives permission to files that makes them executables.
func makeExecs(cluster *Cluster, ip string) (err error) {
	cmd := fmt.Sprintf("sudo chmod +x %v-install.sh", cluster.Config.Product)
	if cluster.Config.Product == "k3s" {
		cmd += "; sudo cp k3s /usr/local/bin/k3s; " +
			"sudo chmod +x /usr/local/bin/k3s"
	}
	_, err = CmdForPrivateNode(cluster, cmd, ip)

	return err
}

// updateRegistryFile updates registries.yaml file and copies to bastion node.
func updateRegistryFile(cluster *Cluster, regMap map[string]string) (err error) {
	regPath := BasePath() + "/modules/airgap/setup/registries.yaml"
	err = ReplaceFileContents(regPath, regMap)
	if err != nil {
		return fmt.Errorf("error replacing registries.yaml contents: %w", err)
	}

	err = RunScp(
		cluster, cluster.BastionConfig.PublicIPv4Addr,
		[]string{regPath}, []string{"~/registries.yaml"})
	if err != nil {
		return fmt.Errorf("error scp-ing registries.yaml on bastion: %w", err)
	}

	return nil
}

// ssPrefix adds prefix to shell commands.
func ssPrefix(cmdType, keyName string) (cmd string) {
	if cmdType != "scp" && cmdType != "ssh" {
		LogLevel("error", "Invalid shell command type: %v", cmdType)
	}
	cmd = cmdType + fmt.Sprintf(
		" -i /tmp/%v.pem -o StrictHostKeyChecking=no -o IdentitiesOnly=yes",
		keyName)

	return cmd
}
