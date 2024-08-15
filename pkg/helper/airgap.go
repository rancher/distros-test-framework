package helper

import (
	"fmt"
	"strings"
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
	cmd := fmt.Sprintf("sudo chmod +x private_registry.sh && sudo ./private_registry.sh %v %v",
		flags.AirgapFlag.RegistryUsername, flags.AirgapFlag.RegistryPassword)
	_, err = shared.RunCommandOnNode(cmd, cluster.GeneralConfig.BastionIP)
	Expect(err).To(BeNil())

	hostname, err := shared.RunCommandOnNode("hostname -f", cluster.GeneralConfig.BastionIP)
	Expect(err).To(BeNil())

	cmd = fmt.Sprintf("sudo cat %v-images.txt", cluster.Config.Product)
	res, err := shared.RunCommandOnNode(cmd, cluster.GeneralConfig.BastionIP)
	Expect(err).To(BeNil())

	images := strings.Split(res, "\n")
	Expect(len(images)).NotTo(BeZero())

	log.Info("Running Docker pull/tag/push/validate on bastion node...")
	for _, image := range images {
		DockerActions(cluster, flags, hostname, image)
	}
	log.Info("Docker operations completed")

	pwd, err := shared.RunCommandOnNode("pwd", cluster.GeneralConfig.BastionIP)
	Expect(err).To(BeNil())

	regMap := map[string]string{
		"$PRIVATE_REG": hostname,
		"$USERNAME":    flags.AirgapFlag.RegistryUsername,
		"$PASSWORD":    flags.AirgapFlag.RegistryPassword,
		"$HOMEDIR":     pwd,
	}

	updateRegistry(cluster, regMap)
}

func DockerActions(cluster *shared.Cluster, flags *customflag.FlagConfig, hostname, image string) {
	log.Info("Docker pull/tag/push started for image: " + image)
	cmd := fmt.Sprintf(
		"sudo docker pull %[1]v && "+
			"sudo docker image tag %[1]v %[2]v/%[1]v && "+
			"sudo docker login %[2]v -u \"%[3]v\" -p \"%[4]v\" && "+
			"sudo docker push %[2]v/%[1]v",
		image, hostname,
		flags.AirgapFlag.RegistryUsername, flags.AirgapFlag.RegistryPassword)

	_, err := shared.RunCommandOnNode(cmd, cluster.GeneralConfig.BastionIP)
	Expect(err).To(BeNil())
	log.Info("Docker pull/tag/push completed for image: " + image)
}

func getArtifacts(cluster *shared.Cluster, flags *customflag.FlagConfig) (res string, err error) {
	cmd := fmt.Sprintf(
		"sudo chmod +x get_artifacts.sh && "+
			"sudo ./get_artifacts.sh %v %v %v %v",
		cluster.Config.Product, cluster.Config.Version,
		cluster.Config.Arch, flags.AirgapFlag.TarballType)
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

func updateRegistry(cluster *shared.Cluster, regMap map[string]string) {
	regPath := shared.BasePath() + "/modules/airgap/setup/registries.yaml"
	shared.ReplaceFileContents(regPath, regMap)

	err := shared.RunScp(
		cluster, cluster.GeneralConfig.BastionIP,
		[]string{regPath}, []string{"~/registries.yaml"})
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
			CopyAssets(cluster, nodeIP)
			CopyRegistry(cluster, nodeIP)
			makeExecs(cluster, nodeIP)
		}()
	}
	wg.Wait()
}

func CopyAssets(cluster *shared.Cluster, ip string) {
	cmd := fmt.Sprintf(
		"sudo chmod 400 /tmp/%v.pem && ", cluster.AwsEc2.KeyName)
	if cluster.Config.Product == "rke2" {
		cmd += fmt.Sprintf(
			"sudo %v -r artifacts %v@%v:~/ && ",
			ssPrefix("scp", cluster.AwsEc2.KeyName),
			cluster.AwsEc2.AwsUser, ip)
	}
	cmd += fmt.Sprintf(
		"sudo %[1]v ./%[2]v %[3]v@%[4]v:~/%[2]v && "+
			"sudo %[1]v certs/* %[3]v@%[4]v:~/ && "+
			"sudo %[1]v ./install_product.sh %[3]v@%[4]v:~/install_product.sh && "+
			"sudo %[1]v ./%[2]v-install.sh %[3]v@%[4]v:~/%[2]v-install.sh",
		ssPrefix("scp", cluster.AwsEc2.KeyName),
		cluster.Config.Product,
		cluster.AwsEc2.AwsUser, ip)

	log.Info(cmd)
	_, err := shared.RunCommandOnNode(cmd, cluster.GeneralConfig.BastionIP)
	Expect(err).To(BeNil())

	log.Info("Copying files complete")
}

func CopyRegistry(cluster *shared.Cluster, ip string) {
	cmd := fmt.Sprintf(
		"sudo %v ./registries.yaml %v@%v:~/registries.yaml",
		ssPrefix("scp", cluster.AwsEc2.KeyName),
		cluster.AwsEc2.AwsUser, ip)

	log.Info(cmd)
	_, err := shared.RunCommandOnNode(cmd, cluster.GeneralConfig.BastionIP)
	Expect(err).To(BeNil())

	cmd = fmt.Sprintf(
		"sudo mkdir -p /etc/rancher/%[2]v ; "+
			"sudo cp registries.yaml /etc/rancher/%[2]v",
		cluster.AwsEc2.KeyName, cluster.Config.Product)
	_, err = CmdForPrivateNode(cluster, cmd, ip)
	Expect(err).To(BeNil())
}

func CmdForPrivateNode(cluster *shared.Cluster, cmd, ip string) (res string, err error) {
	log.Info(cmd)
	serverCmd := fmt.Sprintf(
		"%v %v@%v '%v'",
		ssPrefix("ssh", cluster.AwsEc2.KeyName),
		cluster.AwsEc2.AwsUser, ip, cmd)
	log.Info(serverCmd)
	res, err = shared.RunCommandOnNode(serverCmd, cluster.GeneralConfig.BastionIP)

	return res, err
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
