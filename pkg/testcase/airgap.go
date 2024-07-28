package testcase

import (
	"fmt"
	"strings"
	"time"

	"github.com/rancher/distros-test-framework/pkg/customflag"
	"github.com/rancher/distros-test-framework/pkg/logger"
	"github.com/rancher/distros-test-framework/shared"

	//. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var log = logger.AddLogger() 

func TestBuildPrivateCluster(cluster *shared.Cluster) {
	Expect(cluster.Status).To(Equal("cluster created"))
	Expect(cluster.ServerIPs).ShouldNot(BeEmpty())

	if cluster.GeneralConfig.BastionIP != "" {
		log.Infof("Bastion Node IP: %v", cluster.GeneralConfig.BastionIP)
	}
	log.Infof("Server Node IPs: %v", cluster.ServerIPs)

	checkAndPrintAgentNodeIPs(cluster.NumAgents, cluster.AgentIPs, false)

	if cluster.Config.Product == "rke2" {
		checkAndPrintAgentNodeIPs(cluster.NumWinAgents, cluster.WinAgentIPs, true)
	}
}

func TestAirgapPrivateRegistry(cluster *shared.Cluster, flags *customflag.FlagConfig) {
	// Setting up bastion
	// cmd := fmt.Sprintf("ssh-keyscan %v >> /root/.ssh/known_hosts", cluster.GeneralConfig.BastionIP)	
	// log.Info(cmd)
	// _, err := shared.RunCommandHost(cmd)
	// Expect(err).To(BeNil())
	SetupBastion(cluster, flags)

	// Setting up private instances
	var token string
	for idx, serverIP := range cluster.ServerIPs {
		// cmd = fmt.Sprintf("sudo ssh-keyscan %v >> ~/.ssh/known_hosts", serverIP)
		log.Info(idx)
		
		// copy files
		CopyAssets(cluster, serverIP)

		// update registries.yml
		PublishRegistry(cluster, serverIP)
		
		// make executables
		cmd := fmt.Sprintf(
			"sudo mv ./%[1]v /usr/local/bin/%[1]v ; " + 
			"sudo chmod +x /usr/local/bin/%[1]v ; " +
			"sudo chmod +x %[1]v-install.sh",
			cluster.Config.Product)

		_,err := CmdForPrivateNode(cluster, cmd, serverIP)
		Expect(err).To(BeNil())

		// install product
		// ./install_product.sh product serverIP token nodeType privateIP ipv6IP flags
		cmd = fmt.Sprintf(
			"sudo chmod +x install_product.sh ; " +
			"sudo ./install_product.sh %v \"\" \"\" \"server\" \"%v\" \"\" \"\"",
			cluster.Config.Product, serverIP)
		_, err = CmdForPrivateNode(cluster, cmd, serverIP)
		Expect(err).To(BeNil())

		if idx >= 1 {
			cmd = fmt.Sprintf(
				"sudo chmod +x install_product.sh ; " +
				"sudo ./install_product.sh %v \"%v\" \"%v\" \"server\" \"%v\" \"\" \"\"",
				cluster.Config.Product, cluster.ServerIPs[0], token, serverIP)
			_, err = CmdForPrivateNode(cluster, cmd, serverIP)
			Expect(err).To(BeNil())
		}

		// get token
		if idx == 0 {
			cmd = fmt.Sprintf("sudo cat /var/lib/rancher/%v/server/token", cluster.Config.Product)
			token, err = CmdForPrivateNode(cluster, cmd, serverIP)
			Expect(err).To(BeNil())
			Expect(token).NotTo(BeEmpty())
			log.Info(token)
		}
		
		time.Sleep(10 * time.Second)
	}

	for _, agentIP := range cluster.AgentIPs {
		// copy files
		CopyAssets(cluster, agentIP)

		// publish registries.yml
		PublishRegistry(cluster, agentIP)
			
		// make executables
		cmd := fmt.Sprintf(
			"sudo mv ./%[1]v /usr/local/bin/%[1]v ; " + 
			"sudo chmod +x /usr/local/bin/%[1]v ; " +
			"sudo chmod +x %[1]v-install.sh",
			cluster.Config.Product)

		_,err := CmdForPrivateNode(cluster, cmd, agentIP)
		Expect(err).To(BeNil())

		// install product
		// ./install_product.sh product serverIP token nodeType privateIP ipv6IP flags
		cmd = fmt.Sprintf(
			"sudo chmod +x install_product.sh ; " +
			"sudo ./install_product.sh %v \"%v\" \"%v\" \"agent\" \"%v\" \"\" \"\"",
			cluster.Config.Product, cluster.ServerIPs[0], token, agentIP)
		_, err = CmdForPrivateNode(cluster, cmd, agentIP)
		Expect(err).To(BeNil())

		time.Sleep(10 * time.Second)
	}

	// display nodes,pods
	cmd := fmt.Sprintf(
		"PATH=$PATH:/var/lib/rancher/%[1]v/bin:/opt/%[1]v/bin; " + 
		"KUBECONFIG=/etc/rancher/%[1]v/%[1]v.yaml ", 
		cluster.Config.Product)
	cmd += "kubectl get nodes,pods -A -o wide"
	if cluster.Config.Product == "rke2" {
		log.Info("Waiting for 5 minutes for rke2 cluster to be up...")
		time.Sleep(5 * time.Minute)
	}
	clusterInfo, err := CmdForPrivateNode(cluster, cmd, cluster.ServerIPs[0])
	Expect(err).To(BeNil())

	log.Infoln(clusterInfo)
	
}

func SetupBastion(cluster *shared.Cluster, flags *customflag.FlagConfig) {
	// Setup private registry
	cmd := fmt.Sprintf("sudo chmod +x private_registry.sh && sudo ./private_registry.sh %v %v", 
		flags.RegistryFlag.RegistryUsername, flags.RegistryFlag.RegistryPassword)
	_, err := shared.RunCommandOnNode(cmd, cluster.GeneralConfig.BastionIP)
	Expect(err).To(BeNil())

	hostname, err := shared.RunCommandOnNode("hostname -f", cluster.GeneralConfig.BastionIP)
	Expect(err).To(BeNil())
	
	cmd = fmt.Sprintf("sudo cat %v-images.txt", cluster.Config.Product)
	res, err := shared.RunCommandOnNode(cmd, cluster.GeneralConfig.BastionIP)
	Expect(err).To(BeNil())
	
	images := strings.Split(res, "\n")
	Expect(images).NotTo(BeEmpty())

	log.Info("Running Docker operations on bastion node...")
	for _, image := range images {
		DockerActions(cluster, flags, hostname, image)
	}
	log.Info("Docker operations completed")

	pwd, err := shared.RunCommandOnNode("pwd", cluster.GeneralConfig.BastionIP)
	Expect(err).To(BeNil())
	
	regMap := map[string]string{
		"$PRIVATE_REG": hostname,
		"$USERNAME": flags.RegistryFlag.RegistryUsername,
		"$PASSWORD": flags.RegistryFlag.RegistryPassword,
		"$HOMEDIR" : pwd,
	}

	updateRegistry(cluster, regMap)
}

func DockerActions(cluster *shared.Cluster, flags *customflag.FlagConfig, hostname, image string) {
	// pull image
	cmd := fmt.Sprintf("sudo docker pull %v", image)
	_, err := shared.RunCommandOnNode(cmd, cluster.GeneralConfig.BastionIP)
	Expect(err).To(BeNil())

	// tag image
	cmd = fmt.Sprintf("sudo docker image tag %[1]v %[2]v/%[1]v", image, hostname)
	_, err = shared.RunCommandOnNode(cmd, cluster.GeneralConfig.BastionIP)
	Expect(err).To(BeNil())

	// push image
	cmd = fmt.Sprintf("sudo docker login %[1]v -u \"%[2]v\" -p \"%[3]v\" && sudo docker push %[1]v/%[4]v",
	hostname, flags.RegistryFlag.RegistryUsername, flags.RegistryFlag.RegistryPassword, image)
	_, err = shared.RunCommandOnNode(cmd, cluster.GeneralConfig.BastionIP)
	Expect(err).To(BeNil())

	// validate image
	cmd = fmt.Sprintf("sudo docker image ls %v/%v",hostname, image)
	_, err = shared.RunCommandOnNode(cmd, cluster.GeneralConfig.BastionIP)
	Expect(err).To(BeNil())
	log.Info("Docker pull/tag/push completed for: " + image)

}

func updateRegistry(cluster *shared.Cluster, regMap map[string]string) {
	regPath := shared.BasePath() + "/modules/airgap/setup/registries.yaml"
	shared.ReplaceFileContents(regPath,regMap)
	
	err := shared.RunScp(
		cluster, cluster.GeneralConfig.BastionIP, 
		[]string{regPath}, []string{"~/registries.yaml"})
	Expect(err).To(BeNil())
}

func CopyAssets(cluster *shared.Cluster, ip string) {
	cmd := fmt.Sprintf(
		"sudo chmod 400 /tmp/%v.pem && ", cluster.AwsEc2.KeyName)	
	cmd += fmt.Sprintf(
		"sudo %[1]v ./%[2]v %[3]v@%[4]v:~/%[2]v && " + 
		"sudo %[1]v certs/* %[3]v@%[4]v:~/ && " +
		"sudo %[1]v ./install_product.sh %[3]v@%[4]v:~/install_product.sh && " +
		"sudo %[1]v ./%[2]v-install.sh %[3]v@%[4]v:~/%[2]v-install.sh",
		ssPrefix("scp", cluster.AwsEc2.KeyName),
		cluster.Config.Product, 
		cluster.AwsEc2.AwsUser, ip)

	log.Info(cmd)
	_, err := shared.RunCommandOnNode(cmd, cluster.GeneralConfig.BastionIP)
	Expect(err).To(BeNil())

	log.Info("Copying files complete")
}

func PublishRegistry(cluster *shared.Cluster, ip string) {
	cmd := fmt.Sprintf(
		"sudo %v ./registries.yaml %v@%v:~/registries.yaml", 
		ssPrefix("scp", cluster.AwsEc2.KeyName), 
		cluster.AwsEc2.AwsUser, ip)
	
	log.Info(cmd)
	_,err := shared.RunCommandOnNode(cmd, cluster.GeneralConfig.BastionIP)
	Expect(err).To(BeNil())

	cmd = fmt.Sprintf(
		"sudo chmod 400 /tmp/%[1]v.pem ; " +
		"sudo mkdir -p /etc/rancher/%[2]v ; " + 
		"sudo cp registries.yaml /etc/rancher/%[2]v",
		cluster.AwsEc2.KeyName, cluster.Config.Product)
	
	_, err = CmdForPrivateNode(cluster, cmd, ip)
	Expect(err).To(BeNil())
}

func CmdForPrivateNode(cluster *shared.Cluster, cmd, ip string) (res string, err error){
	log.Info(cmd)
	serverCmd := fmt.Sprintf(
		"%v %v@%v '%v'",
		ssPrefix("ssh", cluster.AwsEc2.KeyName),
		cluster.AwsEc2.AwsUser, ip, cmd)
		
	log.Info(serverCmd)
	res, err = shared.RunCommandOnNode(serverCmd, cluster.GeneralConfig.BastionIP)

	return res, err
}

func ssPrefix(ssCmdType, sshKey string) (cmd string) {
	if ssCmdType != "scp" || ssCmdType != "ssh" {
		log.Errorf("Invalid shell command type: %v", ssCmdType)
	}

	cmd = ssCmdType + fmt.Sprintf(
		" -i /tmp/%v.pem -o StrictHostKeyChecking=no -o IdentitiesOnly=yes",
		sshKey)

	return cmd
}
