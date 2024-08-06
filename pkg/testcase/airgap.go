package testcase

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/rancher/distros-test-framework/pkg/customflag"
	"github.com/rancher/distros-test-framework/pkg/logger"
	"github.com/rancher/distros-test-framework/shared"

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

	CopyAssetsOnNodes(cluster)
	// Setting up private instances
	var token string
	for idx, serverIP := range cluster.ServerIPs {
		// cmd = fmt.Sprintf("sudo ssh-keyscan %v >> ~/.ssh/known_hosts", serverIP)		
		// copy files
		//log.Infof("Copying files to server node-%v", idx+1)
		//CopyAssets(cluster, serverIP)

		// update registries.yml
		//PublishRegistry(cluster, serverIP)
		
		// make executables
		// cmd := fmt.Sprintf(
		// 	"sudo mv ./%[1]v /usr/local/bin/%[1]v ; " + 
		// 	"sudo chmod +x /usr/local/bin/%[1]v ; " +
		// 	"sudo chmod +x %[1]v-install.sh",
		// 	cluster.Config.Product)

		// _,err := CmdForPrivateNode(cluster, cmd, serverIP)
		// Expect(err).To(BeNil())

		if idx == 0 {
			log.Infof("Installing %v on server node-1", cluster.Config.Product)
			// install product
			// ./install_product.sh product serverIP token nodeType privateIP ipv6IP flags
			cmd := fmt.Sprintf(
				"sudo chmod +x install_product.sh ; " +
				"sudo ./install_product.sh %v \"\" \"\" \"server\" \"%v\" \"\" \"\"",
				cluster.Config.Product, serverIP)
			_, err := CmdForPrivateNode(cluster, cmd, serverIP)
			//log.Info(res)
			Expect(err).To(BeNil())

			// log.Info("Waiting for 2 minutes for server 1 to be up...")
			// time.Sleep(2 * time.Minute)
		
			// get token
			log.Info("Retrieving token from server node-1")
			cmd = fmt.Sprintf("sudo cat /var/lib/rancher/%v/server/token", cluster.Config.Product)
			token, err = CmdForPrivateNode(cluster, cmd, serverIP)
			Expect(err).To(BeNil())
			Expect(token).NotTo(BeEmpty())
			log.Info(token)
		}

		if idx >= 1 {
			log.Infof("Installing %v on server node-%v", cluster.Config.Product, idx+1)
			cmd := fmt.Sprintf(
				"sudo chmod +x install_product.sh ; " +
				"sudo ./install_product.sh %v \"%v\" \"%v\" \"server\" \"%v\" \"\" \"\"",
				cluster.Config.Product, cluster.ServerIPs[0], token, serverIP)
			res, err := CmdForPrivateNode(cluster, cmd, serverIP)
			log.Info(res)
			Expect(err).To(BeNil())

			// log.Infof("Waiting for 2 minutes for server %v to be up...", idx+1)
			// time.Sleep(2 * time.Minute)
		}
	}

	for idx, agentIP := range cluster.AgentIPs {
		//log.Infof("Copying files to agent node-%v", idx+1)
		// copy files
		//CopyAssets(cluster, agentIP)

		// publish registries.yml
		//PublishRegistry(cluster, agentIP)
			
		// make executables
		// cmd := fmt.Sprintf(
		// 	"sudo mv ./%[1]v /usr/local/bin/%[1]v ; " + 
		// 	"sudo chmod +x /usr/local/bin/%[1]v ; " +
		// 	"sudo chmod +x %[1]v-install.sh",
		// 	cluster.Config.Product)

		// _,err := CmdForPrivateNode(cluster, cmd, agentIP)
		// Expect(err).To(BeNil())

		CopyAssetsOnNodes(cluster)

		log.Infof("Installing %v on agent node-%v", cluster.Config.Product, idx+1)
		// install product
		// ./install_product.sh product serverIP token nodeType privateIP ipv6IP flags
		cmd := fmt.Sprintf(
			"sudo chmod +x install_product.sh ; " +
			"sudo ./install_product.sh %v \"%v\" \"%v\" \"agent\" \"%v\" \"\" \"\"",
			cluster.Config.Product, cluster.ServerIPs[0], token, agentIP)
		_, err := CmdForPrivateNode(cluster, cmd, agentIP)
		//log.Info(res)
		Expect(err).To(BeNil())

		// log.Infof("Waiting for 1 minute for agent-%v to be up...", idx+1)
		// time.Sleep(1 * time.Minute)
	}

	// display nodes,pods
	log.Info("Waiting for 2 minutes for cluster to be running...")
	time.Sleep(2 * time.Minute)
	cmd := fmt.Sprintf(
		"PATH=$PATH:/var/lib/rancher/%[1]v/bin:/opt/%[1]v/bin; " + 
		"KUBECONFIG=/etc/rancher/%[1]v/%[1]v.yaml ", 
		cluster.Config.Product)
	cmd += "kubectl get nodes,pods -A -o wide"
	
	clusterInfo, err := CmdForPrivateNode(cluster, cmd, cluster.ServerIPs[0])
	Expect(err).To(BeNil())

	log.Infoln(clusterInfo)
	
}

func SetupBastion(cluster *shared.Cluster, flags *customflag.FlagConfig) {
	log.Infof("Downloading %v artifacts...", cluster.Config.Product)
	_, err := getArtifacts(cluster, flags)
	Expect(err).To(BeNil())

	log.Info("Setting up private registry...")
	// Setup private registry
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
	Expect(images).NotTo(BeEmpty())

	log.Info("Running Docker pull/tag/push/validate on bastion node...")
	// var wg sync.WaitGroup

	// for _,image := range images {
	// 	wg.Add(1)
	// 	image := image
	// 	go func() {
	// 		defer wg.Done()
	// 		DockerActionsV2(cluster, flags, hostname, image)
	// 	}()
	// }
	// wg.Wait()
	for _, image := range images {
		DockerActions(cluster, flags, hostname, image)
	}
	log.Info("Docker operations completed")

	pwd, err := shared.RunCommandOnNode("pwd", cluster.GeneralConfig.BastionIP)
	Expect(err).To(BeNil())
	
	regMap := map[string]string{
		"$PRIVATE_REG": hostname,
		"$USERNAME": flags.AirgapFlag.RegistryUsername,
		"$PASSWORD": flags.AirgapFlag.RegistryPassword,
		"$HOMEDIR" : pwd,
	}

	updateRegistry(cluster, regMap)
}

func DockerActionsV2(cluster *shared.Cluster, flags *customflag.FlagConfig, hostname, image string) {
	log.Info("Docker pull/tag/push started for image: " + image)
	cmd := fmt.Sprintf(
		"sudo docker pull %[1]v && " +
		"sudo docker image tag %[1]v %[2]v/%[1]v && " +
		"sudo docker login %[2]v -u \"%[3]v\" -p \"%[4]v\" && " + 
		"sudo docker push %[2]v/%[1]v",
		image, hostname, 
		flags.AirgapFlag.RegistryUsername, flags.AirgapFlag.RegistryPassword)
	
		_, err := shared.RunCommandOnNode(cmd, cluster.GeneralConfig.BastionIP)
	Expect(err).To(BeNil())
	// cmd = fmt.Sprintf("sudo docker image tag %[1]v %[2]v/%[1]v", image, hostname)
	// _, err = shared.RunCommandOnNode(cmd, cluster.GeneralConfig.BastionIP)
	// Expect(err).To(BeNil())

	// push image
	// cmd = fmt.Sprintf("sudo docker login %[1]v -u \"%[2]v\" -p \"%[3]v\" && sudo docker push %[1]v/%[4]v",
	// hostname, flags.AirgapFlag.RegistryUsername, flags.AirgapFlag.RegistryPassword, image)
	// _, err = shared.RunCommandOnNode(cmd, cluster.GeneralConfig.BastionIP)
	// Expect(err).To(BeNil())

	// validate image
	// cmd = fmt.Sprintf("sudo docker image ls %v/%v",hostname, image)
	// _, err = shared.RunCommandOnNode(cmd, cluster.GeneralConfig.BastionIP)
	// Expect(err).To(BeNil())
	log.Info("Docker pull/tag/push completed for image: " + image)

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
	hostname, flags.AirgapFlag.RegistryUsername, flags.AirgapFlag.RegistryPassword, image)
	_, err = shared.RunCommandOnNode(cmd, cluster.GeneralConfig.BastionIP)
	Expect(err).To(BeNil())

	// validate image
	cmd = fmt.Sprintf("sudo docker image ls %v/%v",hostname, image)
	_, err = shared.RunCommandOnNode(cmd, cluster.GeneralConfig.BastionIP)
	Expect(err).To(BeNil())
	log.Info("Docker pull/tag/push completed for image: " + image)
}

func getArtifacts(cluster *shared.Cluster, flags *customflag.FlagConfig) (res string, err error){
	cmd := fmt.Sprintf(
		"sudo chmod +x get_artifacts.sh && " +
		"sudo ./get_artifacts.sh %v %v %v %v",
		cluster.Config.Product, cluster.Config.Version, 
		cluster.Config.Arch, flags.AirgapFlag.TarballType)
	res, err = shared.RunCommandOnNode(cmd, cluster.GeneralConfig.BastionIP)
	
	return res, err
}

func makeExecs(cluster *shared.Cluster, ip string) {
	cmd := fmt.Sprintf(
		"sudo mv ./%[1]v /usr/local/bin/%[1]v ; " + 
		"sudo chmod +x /usr/local/bin/%[1]v ; " +
		"sudo chmod +x %[1]v-install.sh",
		cluster.Config.Product)

	_,err := CmdForPrivateNode(cluster, cmd, ip)
	Expect(err).To(BeNil())
}

func updateRegistry(cluster *shared.Cluster, regMap map[string]string) {
	regPath := shared.BasePath() + "/modules/airgap/setup/registries.yaml"
	shared.ReplaceFileContents(regPath,regMap)
	
	err := shared.RunScp(
		cluster, cluster.GeneralConfig.BastionIP, 
		[]string{regPath}, []string{"~/registries.yaml"})
	Expect(err).To(BeNil())
}

func CopyAssetsOnNodes(cluster *shared.Cluster) {
	log.Info("Copying assets on all the nodes...")
	nodeIPs := cluster.ServerIPs
	nodeIPs = append(nodeIPs,cluster.AgentIPs...)
	
	var wg sync.WaitGroup

	for _,nodeIP := range nodeIPs {
		wg.Add(1)
		nodeIP := nodeIP
		go func() {
			defer wg.Done()
			CopyAssets(cluster, nodeIP)
			PublishRegistry(cluster, nodeIP)
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

func ssPrefix(cmdType, keyName string) (cmd string) {
	if cmdType != "scp" && cmdType != "ssh" {
		log.Errorf("Invalid shell command type: %v", cmdType)
	}

	cmd = cmdType + fmt.Sprintf(
		" -i /tmp/%v.pem -o StrictHostKeyChecking=no -o IdentitiesOnly=yes",
		keyName)

	return cmd
}
