package shared

import (
	"fmt"
	"os"
	"sync"

	"github.com/rancher/distros-test-framework/pkg/customflag"
)

// SetupAirgapRegistry sets bastion node for airgap registry.
func SetupAirgapRegistry(cluster *Cluster, flags *customflag.FlagConfig, airgapMethod string) error {
	LogLevel("info", "Downloading %v artifacts...", cluster.Config.Product)
	_, err := GetArtifacts(cluster, flags.AirgapFlag.TarballType)
	if err != nil {
		return fmt.Errorf("error downloading artifacts: %w", err)
	}

	switch airgapMethod {
	case "private_registry":
		LogLevel("info", "Adding private registry...")
		err = privateRegistry(cluster, flags)
		if err != nil {
			return fmt.Errorf("error adding private registry: %w", err)
		}
	case "system_default_registry":
		LogLevel("info", "Adding system default registry...")
		err = systemDefaultRegistry(cluster)
		if err != nil {
			return fmt.Errorf("error adding system default registry: %w", err)
		}
	}

	LogLevel("info", "Executing Docker pull/tag/push...")
	err = dockerActions(cluster, flags)
	if err != nil {
		return fmt.Errorf("error performing docker actions: %w", err)
	}

	return err
}

// privateRegistry executes private_registry.sh script.
func privateRegistry(cluster *Cluster, flags *customflag.FlagConfig) (err error) {
	cmd := fmt.Sprintf(
		"sudo chmod +x private_registry.sh && "+
			`sudo ./private_registry.sh "%v" "%v" "%v"`,
		flags.AirgapFlag.RegistryUsername, flags.AirgapFlag.RegistryPassword,
		cluster.BastionConfig.PublicDNS)
	res, err := RunCommandOnNode(cmd, cluster.BastionConfig.PublicIPv4Addr)
	if err != nil {
		LogLevel("error", "failed execution of private_registry.sh: %v", res)
	}

	return err
}

// systemDefaultRegistry executes system_default_registry.sh script.
func systemDefaultRegistry(cluster *Cluster) (err error) {
	cmd := fmt.Sprintf(
		"sudo chmod +x system_default_registry.sh && "+
			`sudo ./system_default_registry.sh "%v"`,
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
func CopyAssetsOnNodes(cluster *Cluster, airgapMethod string, tarballType *string) error {
	nodeIPs := cluster.ServerIPs
	nodeIPs = append(nodeIPs, cluster.AgentIPs...)
	errChan := make(chan error, len(nodeIPs))
	var err error
	var wg sync.WaitGroup

	for _, nodeIP := range nodeIPs {
		wg.Add(1)
		go func(nodeIP string) {
			defer wg.Done()
			LogLevel("debug", "Copying %v assets on node IP: %s", cluster.Config.Product, nodeIP)
			err = copyAssets(cluster, airgapMethod, nodeIP)
			if err != nil {
				errChan <- ReturnLogError("error copying assets on airgap node: %v\n, err: %w", nodeIP, err)
			}
			switch airgapMethod {
			case "private_registry":
				LogLevel("debug", "Copying registry.yaml on node IP: %s", nodeIP)
				err = copyRegistry(cluster, nodeIP)
				if err != nil {
					errChan <- ReturnLogError("error copying registry to airgap node: %v\n, err: %w", nodeIP, err)
				}
			case "system_default_registry":
				LogLevel("debug", "Trust CA Certs on node IP: %s", nodeIP)
				err = trustCert(cluster, nodeIP)
				if err != nil {
					errChan <- ReturnLogError("error trusting ssl cert on airgap node: %v\n, err: %w", nodeIP, err)
				}
			case "tarball":
				LogLevel("debug", "Copying tarball on node IP: %s", nodeIP)
				err = copyTarball(cluster, nodeIP, *tarballType)
				if err != nil {
					errChan <- ReturnLogError("error copying tarball on airgap node: %v\n, err: %w", nodeIP, err)
				}
			}
			LogLevel("debug", "Make %s executable on node IP: %s", cluster.Config.Product, nodeIP)
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

func copyTarball(cluster *Cluster, ip, tarballType string) (err error) {
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
func trustCert(cluster *Cluster, ip string) (err error) {
	// TODO: Implement for rhel, sles
	cmd := "sudo cp domain.crt /usr/local/share/ca-certificates/domain.crt && " +
		"sudo update-ca-certificates"
	_, err = CmdForPrivateNode(cluster, cmd, ip)

	return err
}

// copyAssets copies assets from bastion to private node.
func copyAssets(cluster *Cluster, airgapMethod, ip string) (err error) {
	cmd := fmt.Sprintf(
		"sudo chmod 400 /tmp/%v.pem && ", cluster.Aws.KeyName)

	switch cluster.Config.Product {
	case "rke2":
		cmd += fmt.Sprintf(
			"sudo %v -r artifacts %v@%v:~/ && ",
			ssPrefix("scp", cluster.Aws.KeyName),
			cluster.Aws.AwsUser, ip)
	case "k3s":
		cmd += fmt.Sprintf(
			"sudo %v %v* %v@%v:~/ && ",
			ssPrefix("scp", cluster.Aws.KeyName),
			cluster.Config.Product,
			cluster.Aws.AwsUser, ip)
	}

	if airgapMethod != "tarball" {
		cmd += fmt.Sprintf(
			"sudo %v certs/* %v@%v:~/ && ",
			ssPrefix("scp", cluster.Aws.KeyName),
			cluster.Aws.AwsUser, ip)
	}

	cmd += fmt.Sprintf(
		"sudo %v install_product.sh %v-install.sh %v@%v:~/",
		ssPrefix("scp", cluster.Aws.KeyName),
		cluster.Config.Product,
		cluster.Aws.AwsUser, ip)
	_, err = RunCommandOnNode(cmd, cluster.BastionConfig.PublicIPv4Addr)

	return err
}

// copyRegistry copies registries.yaml from bastion on to private node.
func copyRegistry(cluster *Cluster, ip string) (err error) {
	cmd := fmt.Sprintf(
		"sudo %v registries.yaml %v@%v:~/",
		ssPrefix("scp", cluster.Aws.KeyName),
		cluster.Aws.AwsUser, ip)
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
		ssPrefix("ssh", cluster.Aws.KeyName),
		cluster.Aws.AwsUser, ip, cmd)
	LogLevel("debug", "Cmd on bastion node: %v", serverCmd)
	res, err = RunCommandOnNode(serverCmd, cluster.BastionConfig.PublicIPv4Addr)

	return res, err
}

// GetArtifacts executes get_artifacts.sh script.
func GetArtifacts(cluster *Cluster, tarballType string) (res string, err error) {
	serverFlags := os.Getenv("server_flags")
	cmd := fmt.Sprintf(
		"sudo chmod +x get_artifacts.sh && "+
			`sudo ./get_artifacts.sh "%v" "%v" "%v" "%v" "%v"`,
		cluster.Config.Product, cluster.Config.Version,
		cluster.Config.Arch, serverFlags, tarballType)
	res, err = RunCommandOnNode(cmd, cluster.BastionConfig.PublicIPv4Addr)

	return res, err
}

// makeExecs gives permission to files that makes them executable.
func makeExecs(cluster *Cluster, ip string) (err error) {
	cmd := fmt.Sprintf("sudo chmod +x %v-install.sh", cluster.Config.Product)
	if cluster.Config.Product == "k3s" {
		cmd += "; sudo cp k3s /usr/local/bin/k3s; " +
			"sudo chmod +x /usr/local/bin/k3s"
	}
	_, err = CmdForPrivateNode(cluster, cmd, ip)

	return err
}

// UpdateRegistryFile updates registries.yaml file and copies to bastion node.
func UpdateRegistryFile(cluster *Cluster, flags *customflag.FlagConfig) (err error) {
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

	path := BasePath() + "/modules/airgap/setup"
	err = CopyFileContents(path+"/registries.yaml.example", path+"/registries.yaml")
	if err != nil {
		return fmt.Errorf("error copying registries.yaml contents: %w", err)
	}

	err = ReplaceFileContents(path+"/registries.yaml", regMap)
	if err != nil {
		return fmt.Errorf("error replacing registries.yaml contents: %w", err)
	}

	err = RunScp(
		cluster, cluster.BastionConfig.PublicIPv4Addr,
		[]string{path + "/registries.yaml"}, []string{"~/registries.yaml"})
	if err != nil {
		return fmt.Errorf("error scp-ing registries.yaml on bastion: %w", err)
	}

	return nil
}

// DisplayAirgapClusterDetails executes and prints kubectl get nodes,pods on bastion.
func DisplayAirgapClusterDetails(cluster *Cluster) {
	LogLevel("info", "Bastion login: ssh -i %v.pem %v@%v",
		cluster.Aws.KeyName, cluster.Aws.AwsUser,
		cluster.BastionConfig.PublicIPv4Addr)

	cmd := fmt.Sprintf(
		"PATH=$PATH:/var/lib/rancher/%[1]v/bin:/opt/%[1]v/bin; "+
			"KUBECONFIG=/etc/rancher/%[1]v/%[1]v.yaml ",
		cluster.Config.Product)
	cmd += "kubectl get nodes,pods -A -o wide"

	LogLevel("info", "Display cluster details from airgap server-1: %v", cmd)
	clusterInfo, err := CmdForPrivateNode(cluster, cmd, cluster.ServerIPs[0])
	if err != nil {
		LogLevel("error", "Error getting airgap cluster details: %v", err)
	}
	LogLevel("info", "\n%v", clusterInfo)
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
