package support

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"

	. "github.com/onsi/gomega"
	"github.com/rancher/distros-test-framework/pkg/aws"
	"github.com/rancher/distros-test-framework/shared"
)

func BuildIPv6OnlyCluster(cluster *shared.Cluster) {
	shared.LogLevel("info", "Created nodes for %s cluster...", cluster.Config.Product)
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

func ConfigureIPv6OnlyNodes(cluster *shared.Cluster, awsClient *aws.Client) (err error) {
	nodeIPs := cluster.ServerIPs
	nodeIPs = append(nodeIPs, cluster.AgentIPs...)
	errChan := make(chan error, len(nodeIPs))
	var wg sync.WaitGroup

	for _, nodeIP := range nodeIPs {
		wg.Add(1)
		go func(nodeIP string) {
			defer wg.Done()
			shared.LogLevel("info", "Copying configure.sh script on node: %s", nodeIP)
			err = copyConfigureScript(cluster, nodeIP)
			if err != nil {
				errChan <- shared.ReturnLogError("error copying configure.sh script on node: %v\n, err: %w", nodeIP, err)
			}
			shared.LogLevel("info", "Processing configure.sh on node: %s", nodeIP)
			err = processConfigureFile(cluster, awsClient, nodeIP)
			if err != nil {
				errChan <- shared.ReturnLogError("error configuring node: %v\n, err: %w", nodeIP, err)
			}
			shared.LogLevel("info", "Copying install scripts on node: %s", nodeIP)
			err = copyInstallScripts(cluster, nodeIP)
			if err != nil {
				errChan <- shared.ReturnLogError("error configuring node: %v\n, err: %w", nodeIP, err)
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

var token string

func InstallOnIPv6Servers(cluster *shared.Cluster) {
	for idx, serverIP := range cluster.ServerIPs {
		// Installing product on primary server aka server-1, saving the token.
		if idx == 0 {
			shared.LogLevel("info", "Installing %v on server-1...", cluster.Config.Product)
			cmd := fmt.Sprintf(
				"sudo chmod +x %[1]v_master.sh; "+
					`sudo ./%[1]v_master.sh `+
					`"%[2]v" "%[3]v" "" "" "%[4]v" "%[5]v" "%[6]v" `+
					`"%[7]v" "" "%[8]v" "%[9]v" "%[10]v" "%[11]v" "%[12]v"`,
				cluster.Config.Product, os.Getenv("node_os"), "fake.fqdn.value",
				serverIP, os.Getenv("install_mode"), os.Getenv("install_version"),
				os.Getenv("install_channel"), os.Getenv("datastore_type"),
				os.Getenv("datastore_endpoint"), os.Getenv("server_flags"),
				os.Getenv("username"), os.Getenv("password"),
			)
			_, err := shared.CmdForPrivateNode(cluster, cmd, serverIP)
			Expect(err).To(BeNil(), err)

			cmd = fmt.Sprintf("sudo cat /var/lib/rancher/%v/server/token", cluster.Config.Product)
			token, err = shared.CmdForPrivateNode(cluster, cmd, serverIP)
			Expect(err).To(BeNil(), err)
			Expect(token).NotTo(BeEmpty())
			shared.LogLevel("debug", "token: %v", token)
		}
		// Installing product on additional server nodes.
		if idx > 0 {
			shared.LogLevel("info", "Installing %v on server-%v...", cluster.Config.Product, idx+1)
			cmd := fmt.Sprintf(
				"sudo chmod +x join_%[1]v_master.sh; "+
					`sudo ./join_%[1]v_master.sh `+
					`"%[2]v" "%[3]v" "%[4]v" "%[5]v" "" "" "%[6]v" "%[7]v" `+
					`"%[8]v" "%[9]v" "%[10]v" "%[11]v" "%[12]v" "%[13]v" "%[14]v"`,
				cluster.Config.Product, os.Getenv("node_os"), "fake.fqdn.value",
				cluster.ServerIPs[0], token, serverIP,
				os.Getenv("install_mode"), os.Getenv("install_version"),
				os.Getenv("install_channel"), os.Getenv("datastore_type"),
				os.Getenv("datastore_endpoint"), os.Getenv("server_flags"),
				os.Getenv("username"), os.Getenv("password"),
			)
			_, err := shared.CmdForPrivateNode(cluster, cmd, serverIP)
			Expect(err).To(BeNil(), err)
		}
	}
}

func InstallOnIPv6Agents(cluster *shared.Cluster) {
	// Installing product on agent nodes.
	for idx, agentIP := range cluster.AgentIPs {
		shared.LogLevel("info", "Installing %v on agent-%v...", cluster.Config.Product, idx+1)
		cmd := fmt.Sprintf(
			"sudo chmod +x join_%[1]v_agent.sh; "+
				`sudo ./join_%[1]v_agent.sh "%[2]v" "%[3]v" "%[4]v" "" "" "%[5]v" `+
				`"%[6]v" "%[7]v" "%[8]v" "%[9]v" "%[10]v" "%[11]v"`,
			cluster.Config.Product, os.Getenv("node_os"),
			cluster.ServerIPs[0], token, agentIP,
			os.Getenv("install_mode"), os.Getenv("install_version"),
			os.Getenv("install_channel"), os.Getenv("worker_flags"),
			os.Getenv("username"), os.Getenv("password"),
		)
		_, err := shared.CmdForPrivateNode(cluster, cmd, agentIP)
		Expect(err).To(BeNil(), err)
	}
}

// copyConfigureScript Copies configure.sh script on the nodes.
func copyConfigureScript(cluster *shared.Cluster, ip string) (err error) {
	cmd := fmt.Sprintf(
		"sudo chmod 400 /tmp/%v.pem && ", cluster.Aws.KeyName)

	cmd += fmt.Sprintf(
		"sudo %v configure.sh %v@%v:~/",
		shared.ShCmdPrefix("scp", cluster.Aws.KeyName),
		cluster.Aws.AwsUser, shared.EncloseSqBraces(ip))
	_, err = shared.RunCommandOnNode(cmd, cluster.BastionConfig.PublicIPv4Addr)
	if err != nil {
		return err
	}

	return nil
}

// copyInstallScripts Copies install scripts on the nodes.
func copyInstallScripts(cluster *shared.Cluster, ip string) (err error) {
	var installScript string
	cmd := fmt.Sprintf(
		"sudo chmod 400 /tmp/%v.pem && ", cluster.Aws.KeyName)

	if slices.Contains(cluster.ServerIPs, ip) {
		if slices.Index(cluster.ServerIPs, ip) == 0 {
			installScript = cluster.Config.Product + "_master.sh"
		} else {
			installScript = "join_" + cluster.Config.Product + "_master.sh"
		}
	}
	if slices.Contains(cluster.AgentIPs, ip) {
		installScript = "*_agent.sh"
	}
	if strings.Contains(ip, ":") {
		ip = shared.EncloseSqBraces(ip)
	}
	cmd += fmt.Sprintf(
		"sudo %v %v %v@%v:~/",
		shared.ShCmdPrefix("scp", cluster.Aws.KeyName),
		installScript, cluster.Aws.AwsUser, ip)
	_, err = shared.RunCommandOnNode(cmd, cluster.BastionConfig.PublicIPv4Addr)
	if err != nil {
		return err
	}

	return nil
}

// processConfigureFile Runs configure.sh script on the nodes.
func processConfigureFile(cluster *shared.Cluster, ec2 *aws.Client, ip string) (err error) {
	instanceID, err := ec2.GetInstanceIDByIP(ip)
	if err != nil {
		shared.LogLevel("error", "unable to get instance id for node: %s", ip)
		return err
	}
	shared.LogLevel("info", "Found instance id: %s for node: %s", instanceID, ip)

	cmd := fmt.Sprintf(
		"sudo chmod +x configure.sh && "+
			`sudo sh ./configure.sh "%v"`,
		instanceID)
	_, err = shared.CmdForPrivateNode(cluster, cmd, ip)
	if err != nil {
		return err
	}

	return nil
}
