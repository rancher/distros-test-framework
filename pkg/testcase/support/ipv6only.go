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

var token string

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
			shared.LogLevel("info", "Copying install script on node: %s", nodeIP)
			err = copyInstallScripts(cluster, nodeIP)
			if err != nil {
				errChan <- shared.ReturnLogError("error copying install script on node: %v\n, err: %w", nodeIP, err)
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

func InstallOnIPv6Servers(cluster *shared.Cluster) {
	for idx, serverIP := range cluster.ServerIPs {
		// Installing product on primary server aka server-1, saving the token.
		if idx == 0 {
			shared.LogLevel("info", "Installing %v on server-1...", cluster.Config.Product)
			cmd := buildInstallCmd(cluster, "master", "", serverIP)
			shared.LogLevel("debug", "Install cmd: %v", cmd)
			_, err := CmdForPrivateNode(cluster, cmd, serverIP)
			Expect(err).To(BeNil(), err)

			cmd = fmt.Sprintf("sudo cat /var/lib/rancher/%v/server/token", cluster.Config.Product)
			token, err = CmdForPrivateNode(cluster, cmd, serverIP)
			Expect(err).To(BeNil(), err)
			Expect(token).NotTo(BeEmpty())
			shared.LogLevel("debug", "token: %v", token)
		}
		// Installing product on additional server nodes.
		if idx > 0 {
			shared.LogLevel("info", "Installing %v on server-%v...", cluster.Config.Product, idx+1)
			cmd := buildInstallCmd(cluster, "master", token, serverIP)
			shared.LogLevel("debug", "Install cmd: %v", cmd)
			_, err := CmdForPrivateNode(cluster, cmd, serverIP)
			Expect(err).To(BeNil(), err)
		}
	}

	shared.LogLevel("info", "Process kubeconfig from primary server node: %v", cluster.ServerIPs[0])
	err := processKubeconfigOnBastion(cluster)
	if err != nil {
		shared.LogLevel("error", "unable to get kubeconfig\n%w", err)
	}
	shared.LogLevel("info", "Process kubeconfig: Complete!")
}

func InstallOnIPv6Agents(cluster *shared.Cluster) {
	// Installing product on agent nodes.
	for idx, agentIP := range cluster.AgentIPs {
		shared.LogLevel("info", "Installing %v on agent-%v...", cluster.Config.Product, idx+1)
		cmd := buildInstallCmd(cluster, "agent", token, agentIP)
		shared.LogLevel("debug", "Install cmd: %v", cmd)
		_, err := CmdForPrivateNode(cluster, cmd, agentIP)
		Expect(err).To(BeNil(), err)
	}
}

// copyConfigureScript Copies configure.sh script on the nodes.
func copyConfigureScript(cluster *shared.Cluster, ip string) (err error) {
	cmd := fmt.Sprintf(
		"sudo chmod 400 /tmp/%v.pem && ", cluster.SSH.KeyName)

	cmd += fmt.Sprintf(
		"sudo %v configure.sh %v@%v:~/",
		ShCmdPrefix("scp", cluster.SSH.KeyName),
		cluster.SSH.User, shared.EncloseSqBraces(ip))
	_, err = shared.RunCommandOnNode(cmd, cluster.BastionConfig.PublicIPv4Addr)
	if err != nil {
		return err
	}

	return nil
}

// copyInstallScripts Copies install scripts on the nodes.
func copyInstallScripts(cluster *shared.Cluster, ip string) (err error) {
	var script string
	cmd := fmt.Sprintf(
		"sudo chmod 400 /tmp/%v.pem && ", cluster.SSH.KeyName)

	if slices.Contains(cluster.ServerIPs, ip) {
		if slices.Index(cluster.ServerIPs, ip) == 0 {
			script = cluster.Config.Product + "_master.sh"
		} else {
			script = "join_" + cluster.Config.Product + "_master.sh"
		}
	}
	if slices.Contains(cluster.AgentIPs, ip) {
		script = "*_agent.sh"
	}
	if strings.Contains(ip, ":") {
		ip = shared.EncloseSqBraces(ip)
	}
	cmd += fmt.Sprintf(
		"sudo %v %v %v@%v:~/",
		ShCmdPrefix("scp", cluster.SSH.KeyName),
		script, cluster.SSH.User, ip)
	_, err = shared.RunCommandOnNode(cmd, cluster.BastionConfig.PublicIPv4Addr)
	if err != nil {
		return err
	}

	return nil
}

// processConfigureFile Runs configure.sh script on the nodes.
func processConfigureFile(cluster *shared.Cluster, ec2 *aws.Client, ip string) (err error) {
	var flags string
	if slices.Contains(cluster.ServerIPs, ip) {
		flags = cluster.Config.ServerFlags
	}
	instanceID, err := ec2.GetInstanceIDByIP(ip)
	if err != nil {
		shared.LogLevel("error", "unable to get instance id for node: %s", ip)
		return err
	}
	shared.LogLevel("info", "Found instance id: %s for node: %s", instanceID, ip)

	cmd := fmt.Sprintf(
		"sudo chmod +x configure.sh && "+
			`sudo ./configure.sh "%v" "%v" "%v"`,
		instanceID, cluster.Config.Product, flags)
	_, err = CmdForPrivateNode(cluster, cmd, ip)
	if err != nil {
		return err
	}

	return nil
}

func buildInstallCmd(cluster *shared.Cluster, nodeType, token, ip string) string {
	var cmdBuilder strings.Builder
	var cmdSlice []string
	var script string
	product := cluster.Config.Product
	if nodeType == "master" && token == "" {
		cmdSlice = []string{
			cluster.NodeOS, cluster.FQDN, "", "", ip,
			cluster.Config.InstallMode, cluster.Config.Version, cluster.Config.Channel,
			cluster.Config.DataStore, cluster.Config.ExternalDbEndpoint, cluster.Config.ServerFlags,
			os.Getenv("username"), os.Getenv("password"), "both",
		}
		if product == "k3s" {
			cmdSlice = slices.Insert(cmdSlice, 8, "")
		} else {
			cmdSlice = slices.Insert(cmdSlice, 8, os.Getenv("install_method"))
		}
		script = "./" + product + "_" + nodeType + ".sh"
	}

	if nodeType == "master" && token != "" {
		cmdSlice = []string{
			cluster.NodeOS, cluster.FQDN, cluster.ServerIPs[0], token, "", "", ip,
			cluster.Config.InstallMode, cluster.Config.Version, cluster.Config.Channel,
			cluster.Config.DataStore, cluster.Config.ExternalDbEndpoint, cluster.Config.ServerFlags,
			os.Getenv("username"), os.Getenv("password"), "both",
		}
		if product == "rke2" {
			cmdSlice = slices.Insert(cmdSlice, 10, cluster.Config.InstallMethod)
		}
		script = "./join_" + product + "_" + nodeType + ".sh"
	}

	if nodeType == "agent" {
		cmdSlice = []string{
			cluster.NodeOS, cluster.ServerIPs[0], token, "", "", ip,
			cluster.Config.InstallMode, cluster.Config.Version, cluster.Config.Channel,
			cluster.Config.WorkerFlags, os.Getenv("username"), os.Getenv("password"), "both",
		}
		if product == "rke2" {
			cmdSlice = slices.Insert(cmdSlice, 9, cluster.Config.InstallMethod)
		}
		script = "./join_" + product + "_" + nodeType + ".sh"
	}

	_, _ = cmdBuilder.WriteString(fmt.Sprintf("sudo chmod +x %v; ", script))
	_, _ = cmdBuilder.WriteString("sudo " + script + " ")
	for _, str := range cmdSlice {
		_, _ = cmdBuilder.WriteString(`"` + str + `" `)
	}

	return cmdBuilder.String()
}
