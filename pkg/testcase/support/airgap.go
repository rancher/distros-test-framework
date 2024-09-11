package support

import (
	"fmt"
	"os"
	"sync"

	"github.com/rancher/distros-test-framework/pkg/customflag"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/gomega"
)

func SetupPrivateRegistry(cluster *shared.Cluster, flags *customflag.FlagConfig) {
	shared.LogLevel("info", "Downloading %v artifacts...", cluster.Config.Product)
	_, err := getArtifacts(cluster, flags)
	Expect(err).To(BeNil())

	bastionAsPrivateRegistry(cluster, flags)
	dockerActions(cluster, flags)

	pwd, err := shared.RunCommandOnNode("pwd", cluster.GeneralConfig.BastionIP)
	Expect(err).To(BeNil())

	regMap := map[string]string{
		"$PRIVATE_REG": cluster.GeneralConfig.BastionDNS,
		"$USERNAME":    flags.AirgapFlag.RegistryUsername,
		"$PASSWORD":    flags.AirgapFlag.RegistryPassword,
		"$HOMEDIR":     pwd,
	}

	updateRegistryFile(cluster, regMap)
}

func bastionAsPrivateRegistry(cluster *shared.Cluster, flags *customflag.FlagConfig) {
	shared.LogLevel("info", "Setting up bastion node as private registry...")
	cmd := fmt.Sprintf(
		"sudo chmod +x private_registry.sh && "+
			`sudo ./private_registry.sh "%v" "%v" "%v"`,
		flags.AirgapFlag.RegistryUsername, flags.AirgapFlag.RegistryPassword,
		cluster.GeneralConfig.BastionDNS)
	_, err := shared.RunCommandOnNode(cmd, cluster.GeneralConfig.BastionIP)
	Expect(err).To(BeNil())
}

func dockerActions(cluster *shared.Cluster, flags *customflag.FlagConfig) {
	shared.LogLevel("info", "Running Docker ops script on bastion...")
	cmd := "sudo chmod +x docker_ops.sh && " +
		fmt.Sprintf(
			`sudo ./docker_ops.sh "%v" "%v" "%v" "%v"`,
			cluster.Config.Product, cluster.GeneralConfig.BastionDNS,
			flags.AirgapFlag.RegistryUsername, flags.AirgapFlag.RegistryPassword)
	_, err := shared.RunCommandOnNode(cmd, cluster.GeneralConfig.BastionIP)
	Expect(err).To(BeNil())
	shared.LogLevel("info", "Docker pull/tag/push completed!")
}

func CopyAssetsOnNodes(cluster *shared.Cluster) {
	shared.LogLevel("info", "Copying assets on all the nodes...")
	nodeIPs := cluster.ServerIPs
	nodeIPs = append(nodeIPs, cluster.AgentIPs...)

	var wg sync.WaitGroup
	for _, nodeIP := range nodeIPs {
		wg.Add(1)
		nodeIP := nodeIP
		go func() {
			defer wg.Done()
			copyAssets(cluster, nodeIP)
			copyRegistry(cluster, nodeIP)
			makeExecs(cluster, nodeIP)
		}()
	}
	wg.Wait()
	shared.LogLevel("info", "Copying assets complete!")
}

func copyAssets(cluster *shared.Cluster, ip string) {
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
	shared.LogLevel("info", "cmd: %v", cmd)
	_, err := shared.RunCommandOnNode(cmd, cluster.GeneralConfig.BastionIP)
	Expect(err).To(BeNil())
}

func copyRegistry(cluster *shared.Cluster, ip string) {
	cmd := fmt.Sprintf(
		"sudo %v registries.yaml %v@%v:~/",
		ssPrefix("scp", cluster.AwsEc2.KeyName),
		cluster.AwsEc2.AwsUser, ip)
	_, err := shared.RunCommandOnNode(cmd, cluster.GeneralConfig.BastionIP)
	Expect(err).To(BeNil())

	cmd = fmt.Sprintf(
		"sudo mkdir -p /etc/rancher/%[1]v && "+
			"sudo cp registries.yaml /etc/rancher/%[1]v",
		cluster.Config.Product)
	_, err = CmdForPrivateNode(cluster, cmd, ip)
	Expect(err).To(BeNil())
}

func CmdForPrivateNode(cluster *shared.Cluster, cmd, ip string) (res string, err error) {
	serverCmd := fmt.Sprintf(
		"%v %v@%v '%v'",
		ssPrefix("ssh", cluster.AwsEc2.KeyName),
		cluster.AwsEc2.AwsUser, ip, cmd)
	shared.LogLevel("info", serverCmd)
	res, err = shared.RunCommandOnNode(serverCmd, cluster.GeneralConfig.BastionIP)

	return res, err
}

func getArtifacts(cluster *shared.Cluster, flags *customflag.FlagConfig) (res string, err error) {
	serverFlags := os.Getenv("server_flags")
	cmd := fmt.Sprintf(
		"sudo chmod +x get_artifacts.sh && "+
			`sudo ./get_artifacts.sh "%v" "%v" "%v" "%v" "%v"`,
		cluster.Config.Product, cluster.Config.Version,
		cluster.Config.Arch, serverFlags, flags.AirgapFlag.TarballType)
	res, err = shared.RunCommandOnNode(cmd, cluster.GeneralConfig.BastionIP)

	return res, err
}

func makeExecs(cluster *shared.Cluster, ip string) {
	cmd := fmt.Sprintf("sudo chmod +x %v-install.sh", cluster.Config.Product)
	if cluster.Config.Product == "k3s" {
		cmd += "; sudo cp k3s /usr/local/bin/k3s; " +
			"sudo chmod +x /usr/local/bin/k3s"
	}
	_, err := CmdForPrivateNode(cluster, cmd, ip)
	Expect(err).To(BeNil())
}

func ssPrefix(cmdType, keyName string) (cmd string) {
	if cmdType != "scp" && cmdType != "ssh" {
		shared.LogLevel("error", "Invalid shell command type: %v", cmdType)
	}
	cmd = cmdType + fmt.Sprintf(
		" -i /tmp/%v.pem -o StrictHostKeyChecking=no -o IdentitiesOnly=yes",
		keyName)

	return cmd
}

func updateRegistryFile(cluster *shared.Cluster, regMap map[string]string) {
	regPath := shared.BasePath() + "/modules/airgap/setup/registries.yaml"
	err := shared.ReplaceFileContents(regPath, regMap)
	Expect(err).To(BeNil(), err)

	err = shared.RunScp(
		cluster, cluster.GeneralConfig.BastionIP,
		[]string{regPath}, []string{"~/registries.yaml"})
	Expect(err).To(BeNil(), err)
}
