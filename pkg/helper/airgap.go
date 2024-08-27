package helper

import (
	"fmt"
	"os"
	"sync"

	"github.com/rancher/distros-test-framework/pkg/customflag"
	"github.com/rancher/distros-test-framework/pkg/logger"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/gomega"
)

var log = logger.AddLogger()

func SetupBastion(cluster *shared.Cluster, flags *customflag.FlagConfig) {
	log.Infof("Downloading %v artifacts...", cluster.Config.Product)
	_, err := getArtifacts(cluster, flags)
	Expect(err).To(BeNil())

	log.Info("Setting up private registry...")
	cmd := fmt.Sprintf(
		"sudo chmod +x private_registry.sh && " + 
		"sudo ./private_registry.sh %v %v %v",
		flags.AirgapFlag.RegistryUsername, flags.AirgapFlag.RegistryPassword,
		cluster.GeneralConfig.BastionDNS)
	_,err = shared.RunCommandOnNode(cmd, cluster.GeneralConfig.BastionIP)
	Expect(err).To(BeNil())
	
	log.Info("Running Docker ops script on bastion...")
	dockerActions(cluster, flags)
	log.Info("Docker pull/tag/push completed")
	
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

func dockerActions(cluster *shared.Cluster, flags *customflag.FlagConfig) {
	cmd := "sudo chmod +x docker_ops.sh && " +
		fmt.Sprintf(
			"sudo ./docker_ops.sh %v %v %v %v",
			cluster.Config.Product, cluster.GeneralConfig.BastionDNS,
			flags.AirgapFlag.RegistryUsername, flags.AirgapFlag.RegistryPassword)
	_,err := shared.RunCommandOnNode(cmd, cluster.GeneralConfig.BastionIP)
	Expect(err).To(BeNil())
}

func CopyAssetsOnNodes(cluster *shared.Cluster) {
	log.Info("Copying assets on all the nodes...")
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
}

func copyAssets(cluster *shared.Cluster, ip string) {
	cmd := fmt.Sprintf(
		"sudo chmod 400 /tmp/%v.pem && ", cluster.AwsEc2.KeyName)
	if cluster.Config.Product == "rke2" {
		cmd += fmt.Sprintf(
			"sudo %v -r artifacts %v@%v:~/ && ",
			ssPrefix("scp", cluster.AwsEc2.KeyName),
			cluster.AwsEc2.AwsUser, ip)
	}
	if cluster.Config.Product == "k3s" {
		cmd += fmt.Sprintf(
			"sudo %[1]v ./%[2]v %[3]v@%[4]v:~/%[2]v && ",
			ssPrefix("scp", cluster.AwsEc2.KeyName),
			cluster.Config.Product,
			cluster.AwsEc2.AwsUser, ip)
	}
	cmd += fmt.Sprintf(
		"sudo %[1]v certs/* %[3]v@%[4]v:~/ && "+
		"sudo %[1]v ./install_product.sh %[3]v@%[4]v:~/install_product.sh && "+
		"sudo %[1]v ./%[2]v-install.sh %[3]v@%[4]v:~/%[2]v-install.sh",
		ssPrefix("scp", cluster.AwsEc2.KeyName),
		cluster.Config.Product,
		cluster.AwsEc2.AwsUser, ip)
	_, err := shared.RunCommandOnNode(cmd, cluster.GeneralConfig.BastionIP)
	Expect(err).To(BeNil())

	log.Info("Copying files complete")
}

func copyRegistry(cluster *shared.Cluster, ip string) {
	cmd := fmt.Sprintf(
		"sudo %v ./registries.yaml %v@%v:~/registries.yaml",
		ssPrefix("scp", cluster.AwsEc2.KeyName),
		cluster.AwsEc2.AwsUser, ip)
	_, err := shared.RunCommandOnNode(cmd, cluster.GeneralConfig.BastionIP)
	Expect(err).To(BeNil())

	cmd = fmt.Sprintf(
		"sudo cp registries.yaml /etc/rancher/%v",
		cluster.Config.Product)
	_, err = CmdForPrivateNode(cluster, cmd, ip)
	Expect(err).To(BeNil())
}

func CmdForPrivateNode(cluster *shared.Cluster, cmd, ip string) (res string, err error) {
	serverCmd := fmt.Sprintf(
		"%v %v@%v '%v'",
		ssPrefix("ssh", cluster.AwsEc2.KeyName),
		cluster.AwsEc2.AwsUser, ip, cmd)
	log.Info(serverCmd)
	res, err = shared.RunCommandOnNode(serverCmd, cluster.GeneralConfig.BastionIP)

	return res, err
}

func getArtifacts(cluster *shared.Cluster, flags *customflag.FlagConfig) (res string, err error) {
	serverFlags := os.Getenv("server_flags")
	cmd := fmt.Sprintf(
		"sudo chmod +x get_artifacts.sh && "+
		"sudo ./get_artifacts.sh \"%v\" \"%v\" \"%v\" \"%v\" \"%v\"",
		cluster.Config.Product, cluster.Config.Version,
		cluster.Config.Arch, serverFlags, flags.AirgapFlag.TarballType)
	res, err = shared.RunCommandOnNode(cmd, cluster.GeneralConfig.BastionIP)

	return res, err
}

func makeExecs(cluster *shared.Cluster, ip string) {
	cmd := fmt.Sprintf(
		"sudo mv ./%[1]v /usr/local/bin/%[1]v ; "+
		"sudo chmod +x /usr/local/bin/%[1]v ; "+
		"sudo chmod +x %[1]v-install.sh",
		cluster.Config.Product)

	_, err := CmdForPrivateNode(cluster, cmd, ip)
	Expect(err).To(BeNil())
}


func ssPrefix(cmdType, keyName string) (cmd string) {
	if cmdType != "scp" && cmdType != "ssh" {
		log.Errorf("Invalid shell command type: %v", cmdType)
	}

	cmd = cmdType + fmt.Sprintf(
		" -i /tmp/%v.pem -o StrictHostKeyChecking=no -o IdentitiesOnly=yes",
		keyName)

	return cmd
}

func updateRegistryFile(cluster *shared.Cluster, regMap map[string]string) {
	regPath := shared.BasePath() + "/modules/airgap/setup/registries.yaml"
	shared.ReplaceFileContents(regPath, regMap)

	err := shared.RunScp(
		cluster, cluster.GeneralConfig.BastionIP,
		[]string{regPath}, []string{"~/registries.yaml"})
	Expect(err).To(BeNil())
}
