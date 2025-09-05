package support

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"

	. "github.com/onsi/gomega"

	"github.com/rancher/distros-test-framework/internal/pkg/aws"
	"github.com/rancher/distros-test-framework/internal/resources"
)

var token string

func BuildIPv6OnlyCluster(cluster *resources.Cluster) {
	resources.LogLevel("info", "Created nodes for %s cluster...", cluster.Config.Product)
	Expect(cluster.Status).To(Equal("cluster created"))
	Expect(cluster.ServerIPs).ShouldNot(BeEmpty())

	if cluster.Bastion.PublicIPv4Addr != "" {
		resources.LogLevel("info", "Bastion Public IP: %v", cluster.Bastion.PublicIPv4Addr)
		resources.LogLevel("info", "Bastion Public DNS: %v", cluster.Bastion.PublicDNS)
	}
	resources.LogLevel("info", "Server Node IPs: %v", cluster.ServerIPs)

	LogAgentNodeIPs(cluster.NumAgents, cluster.AgentIPs, false)

	if cluster.Config.Product == "rke2" {
		LogAgentNodeIPs(cluster.NumWinAgents, cluster.WinAgentIPs, true)
	}
}

func ConfigureIPv6OnlyNodes(cluster *resources.Cluster, awsClient *aws.Client) (err error) {
	nodeIPs := cluster.ServerIPs
	nodeIPs = append(nodeIPs, cluster.AgentIPs...)
	errChan := make(chan error, len(nodeIPs))
	var wg sync.WaitGroup

	for _, nodeIP := range nodeIPs {
		wg.Add(1)
		go func(nodeIP string) {
			defer wg.Done()
			resources.LogLevel("info", "Copying configure.sh script on node: %s", nodeIP)
			err = copyConfigureScript(cluster, nodeIP)
			if err != nil {
				errChan <- resources.ReturnLogError("error copying configure.sh script on node: %v\n, err: %w", nodeIP, err)
			}
			resources.LogLevel("info", "Processing configure.sh on node: %s", nodeIP)
			err = processConfigureFile(cluster, awsClient, nodeIP)
			if err != nil {
				errChan <- resources.ReturnLogError("error configuring node: %v\n, err: %w", nodeIP, err)
			}
			resources.LogLevel("info", "Copying install script on node: %s", nodeIP)
			err = copyInstallScripts(cluster, nodeIP)
			if err != nil {
				errChan <- resources.ReturnLogError("error copying install script on node: %v\n, err: %w", nodeIP, err)
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

func InstallOnIPv6Servers(cluster *resources.Cluster) {
	for idx, serverIP := range cluster.ServerIPs {
		// Installing product on primary server aka server-1, saving the token.
		if idx == 0 {
			resources.LogLevel("info", "Installing %v on server-1...", cluster.Config.Product)
			cmd := buildInstallCmd(cluster, "master", "", serverIP)
			resources.LogLevel("debug", "Install cmd: %v", cmd)
			_, err := CmdForPrivateNode(cluster, cmd, serverIP)
			Expect(err).To(BeNil(), err)

			cmd = fmt.Sprintf("sudo cat /var/lib/rancher/%v/server/token", cluster.Config.Product)
			token, err = CmdForPrivateNode(cluster, cmd, serverIP)
			Expect(err).To(BeNil(), err)
			Expect(token).NotTo(BeEmpty())
			resources.LogLevel("debug", "token: %v", token)
		}
		// Installing product on additional server nodes.
		if idx > 0 {
			resources.LogLevel("info", "Installing %v on server-%v...", cluster.Config.Product, idx+1)
			cmd := buildInstallCmd(cluster, "master", token, serverIP)
			resources.LogLevel("debug", "Install cmd: %v", cmd)
			_, err := CmdForPrivateNode(cluster, cmd, serverIP)
			Expect(err).To(BeNil(), err)
		}
	}

	resources.LogLevel("info", "Process kubeconfig from primary server node: %v", cluster.ServerIPs[0])
	err := processKubeconfigOnBastion(cluster)
	if err != nil {
		resources.LogLevel("error", "unable to get kubeconfig\n%w", err)
	}
	resources.LogLevel("info", "Process kubeconfig: Complete!")
}

func InstallOnIPv6Agents(cluster *resources.Cluster) {
	// Installing product on agent nodes.
	for idx, agentIP := range cluster.AgentIPs {
		resources.LogLevel("info", "Installing %v on agent-%v...", cluster.Config.Product, idx+1)
		cmd := buildInstallCmd(cluster, "agent", token, agentIP)
		resources.LogLevel("debug", "Install cmd: %v", cmd)
		_, err := CmdForPrivateNode(cluster, cmd, agentIP)
		Expect(err).To(BeNil(), err)
	}
}

// copyConfigureScript Copies configure.sh script on the nodes.
func copyConfigureScript(cluster *resources.Cluster, ip string) (err error) {
	cmd := fmt.Sprintf(
		"sudo chmod 400 /tmp/%v.pem && ", cluster.SSH.KeyName)

	cmd += fmt.Sprintf(
		"sudo %v configure.sh %v@%v:~/",
		ShCmdPrefix("scp", cluster.SSH.KeyName),
		cluster.SSH.User, resources.EncloseSqBraces(ip))
	_, err = resources.RunCommandOnNode(cmd, cluster.Bastion.PublicIPv4Addr)
	if err != nil {
		return err
	}

	return nil
}

// copyInstallScripts Copies install scripts on the nodes.
func copyInstallScripts(cluster *resources.Cluster, ip string) (err error) {
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
		ip = resources.EncloseSqBraces(ip)
	}
	cmd += fmt.Sprintf(
		"sudo %v %v %v@%v:~/",
		ShCmdPrefix("scp", cluster.SSH.KeyName),
		script, cluster.SSH.User, ip)
	_, err = resources.RunCommandOnNode(cmd, cluster.Bastion.PublicIPv4Addr)
	if err != nil {
		return err
	}

	return nil
}

// processConfigureFile Runs configure.sh script on the nodes.
func processConfigureFile(cluster *resources.Cluster, ec2 *aws.Client, ip string) (err error) {
	var flags string
	if slices.Contains(cluster.ServerIPs, ip) {
		flags = cluster.Config.ServerFlags
	}
	instanceID, err := ec2.GetInstanceIDByIP(ip)
	if err != nil {
		resources.LogLevel("error", "unable to get instance id for node: %s", ip)
		return err
	}
	resources.LogLevel("info", "Found instance id: %s for node: %s", instanceID, ip)

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

func buildInstallCmd(cluster *resources.Cluster, nodeType, token, ip string) string {
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
