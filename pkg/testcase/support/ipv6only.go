package support

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strconv"
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

// configureNodeAssets abstracts the sequential steps for copying and processing scripts on a single node.
func configureNodeAssets(cluster *shared.Cluster, awsClient *aws.Client, nodeIP string, nodeIndex int) error {
	// Helper closure to wrap the shared.LogLevel and error checking for each step
	runStep := func(description string, action func() error) error {
		shared.LogLevel("info", "%s on node: %s", description, nodeIP)
		if err := action(); err != nil {
			return fmt.Errorf("failed during %s: %w", description, err)
		}

		return nil
	}

	// 1. Copy and Process configure.sh
	if err := runStep("Copying configure.sh script", func() error {
		return copyConfigureScript(cluster, nodeIP)
	}); err != nil {
		return err
	}

	if err := runStep("Processing configure.sh", func() error {
		return processConfigureFile(cluster, awsClient, nodeIP)
	}); err != nil {
		return err
	}

	// 2. Copy install script
	if err := runStep("Copying install script", func() error {
		return copyInstallScript(cluster, nodeIP)
	}); err != nil {
		return err
	}

	// 3. Handle split roles configuration (Server nodes only)
	if cluster.Config.SplitRoles.Enabled && slices.Contains(cluster.ServerIPs, nodeIP) {
		if err := runStep("Copying node role script", func() error {
			return copyNodeRoleScript(cluster, nodeIP)
		}); err != nil {
			return err
		}

		if err := runStep("Processing node_role.sh script", func() error {
			// NOTE: nodeIndex is the index in the combined nodeIPs list, used by the original logic.
			return processNodeRole(cluster, nodeIndex, nodeIP)
		}); err != nil {
			return err
		}
	}

	return nil
}

// ConfigureIPv6OnlyNodes orchestrates the configuration of all nodes concurrently.
func ConfigureIPv6OnlyNodes(cluster *shared.Cluster, awsClient *aws.Client) error {
	nodeIPs := make([]string, 0, len(cluster.ServerIPs)+len(cluster.AgentIPs))
	nodeIPs = append(nodeIPs, cluster.ServerIPs...)
	nodeIPs = append(nodeIPs, cluster.AgentIPs...)

	errChan := make(chan error, len(nodeIPs))
	var wg sync.WaitGroup

	for idx, nodeIP := range nodeIPs {
		wg.Add(1)
		go func(nodeIndex int, ip string) {
			defer wg.Done()

			if err := configureNodeAssets(cluster, awsClient, ip, nodeIndex); err != nil {
				shared.LogLevel("error", "Configuration failed for node %s: %v", ip, err)
				errChan <- err
			}
		}(idx, nodeIP)
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

func InstallOnIPv6Servers(cluster *shared.Cluster) error {
	var currentToken string
	installType := "master"
	for idx, serverIP := range cluster.ServerIPs {
		if idx == 0 {
			shared.LogLevel("info", "Installing %v on server-1 (Primary)...", cluster.Config.Product)
		} else {
			currentToken = token // Uses the global token set by the primary server
			shared.LogLevel("info", "Installing %v on server-%v (Join)...", cluster.Config.Product, idx+1)

			if currentToken == "" {
				return fmt.Errorf("server token is empty; cannot join server-%d", idx+1)
			}
		}

		cmd := buildInstallCmd(cluster, installType, currentToken, serverIP)
		shared.LogLevel("debug", "Install cmd: %v", cmd)

		_, err := CmdForPrivateNode(cluster, cmd, serverIP)
		if err != nil {
			return fmt.Errorf("failed to install %s on server-%d (%s): %w", cluster.Config.Product, idx+1, serverIP, err)
		}

		if idx == 0 {
			cmd := fmt.Sprintf("sudo cat /var/lib/rancher/%v/server/token", cluster.Config.Product)
			output, err := CmdForPrivateNode(cluster, cmd, serverIP)
			if err != nil {
				return fmt.Errorf("failed to retrieve token from primary server (%s): %w", serverIP, err)
			}

			token = strings.TrimSpace(output)
			if token == "" {
				return errors.New("retrieved token from primary server is empty")
			}
			shared.LogLevel("debug", "Extracted token: %v", token)
		}
	}

	kubeconfigIP, err := getKubeconfigSourceIP(cluster)
	if err != nil {
		return err // Returns error if no suitable IP is found
	}

	shared.LogLevel("info", "Process kubeconfig from server node IP: %v", kubeconfigIP)
	if err := processKubeconfigOnBastion(cluster, kubeconfigIP); err != nil {
		return fmt.Errorf("unable to process kubeconfig on bastion from IP %s: %w", kubeconfigIP, err)
	}
	shared.LogLevel("info", "Process kubeconfig: Complete!")

	return nil
}

// getKubeconfigSourceIP determines which server to pull the kubeconfig from.
func getKubeconfigSourceIP(cluster *shared.Cluster) (string, error) {
	if !cluster.Config.SplitRoles.Enabled {
		if len(cluster.ServerIPs) == 0 {
			return "", errors.New("cannot get kubeconfig source: ServerIPs list is empty")
		}
		return cluster.ServerIPs[0], nil
	}

	cmdTpl := "cat /etc/rancher/%v/config.yaml.d/role_config.yaml 2>/dev/null | grep %v"

	for _, serverIP := range cluster.ServerIPs {
		cmd := fmt.Sprintf(cmdTpl, cluster.Config.Product, "control-plane")
		res, _ := CmdForPrivateNode(cluster, cmd, serverIP)

		if strings.Contains(res, "control-plane") {
			return serverIP, nil
		}
	}

	shared.LogLevel("warn", "No server found with 'control-plane' role; falling back to server-1.")

	return cluster.ServerIPs[0], nil
}

func InstallOnIPv6Agents(cluster *shared.Cluster) error {
	if token == "" {
		return errors.New("cannot install agents: the required server token is empty or was not retrieved")
	}

	for idx, agentIP := range cluster.AgentIPs {
		shared.LogLevel("info", "Installing %v on agent-%v...", cluster.Config.Product, idx+1)
		cmd := buildInstallCmd(cluster, "agent", token, agentIP)
		shared.LogLevel("debug", "Install cmd: %v", cmd)

		_, err := CmdForPrivateNode(cluster, cmd, agentIP)
		if err != nil {
			return fmt.Errorf("failed to install %s on agent-%d (%s): %w", cluster.Config.Product, idx+1, agentIP, err)
		}
	}

	return nil
}

// copyConfigureScript Copies configure.sh script on the nodes.
func copyConfigureScript(cluster *shared.Cluster, ip string) (err error) {
	cmd := fmt.Sprintf(
		"sudo chmod 400 /tmp/%v.pem && ", cluster.Aws.KeyName)

	cmd += fmt.Sprintf(
		"sudo %v configure.sh %v@%v:~/",
		ShCmdPrefix("scp", cluster.Aws.KeyName),
		cluster.Aws.AwsUser, shared.EncloseSqBraces(ip))
	_, err = shared.RunCommandOnNode(cmd, cluster.BastionConfig.PublicIPv4Addr)
	if err != nil {
		return err
	}

	return nil
}

// copyInstallScript Copies install script on the nodes.
func copyInstallScript(cluster *shared.Cluster, ip string) (err error) {
	var script string
	cmd := fmt.Sprintf(
		"sudo chmod 400 /tmp/%v.pem && ", cluster.Aws.KeyName)

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
		ShCmdPrefix("scp", cluster.Aws.KeyName),
		script, cluster.Aws.AwsUser, ip)
	_, err = shared.RunCommandOnNode(cmd, cluster.BastionConfig.PublicIPv4Addr)
	if err != nil {
		return err
	}

	return nil
}

// copyNodeRoleScript Copies install script on the nodes.
func copyNodeRoleScript(cluster *shared.Cluster, ip string) (err error) {
	cmd := fmt.Sprintf(
		"sudo chmod 400 /tmp/%v.pem && ", cluster.Aws.KeyName)

	if strings.Contains(ip, ":") {
		ip = shared.EncloseSqBraces(ip)
	}
	cmd += fmt.Sprintf(
		"sudo %v %v %v@%v:~/",
		ShCmdPrefix("scp", cluster.Aws.KeyName),
		"node_role.sh", cluster.Aws.AwsUser, ip)
	_, err = shared.RunCommandOnNode(cmd, cluster.BastionConfig.PublicIPv4Addr)
	if err != nil {
		return err
	}

	return nil
}

// processNodeRole Runs node_role.sh script on the server nodes.
func processNodeRole(cluster *shared.Cluster, idx int, ip string) (err error) {
	splitRoles := cluster.Config.SplitRoles
	cmd := fmt.Sprintf(
		"sudo chmod +x node_role.sh && "+
			`sudo ./node_role.sh "%v" "%v" "%v" "%v" "%v" "%v" "%v" "%v" "%v"`,
		(idx - 1), splitRoles.RoleOrder, cluster.NumServers, splitRoles.EtcdOnly,
		splitRoles.EtcdCP, splitRoles.EtcdWorker, splitRoles.ControlPlaneOnly,
		splitRoles.ControlPlaneWorker, cluster.Config.Product,
	)
	_, err = CmdForPrivateNode(cluster, cmd, ip)
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
		`sudo chmod +x configure.sh && sudo ./configure.sh "%v"`, instanceID)
	_, err = CmdForPrivateNode(cluster, cmd, ip)
	if err != nil {
		return err
	}

	return nil
}

//nolint:funlen // Better readability.
func buildInstallCmd(cluster *shared.Cluster, nodeType, token, ip string) string {
	var cmdSlice []string
	var script string
	var cmdBuilder strings.Builder

	product := cluster.Config.Product
	if nodeType == "master" && token == "" {
		cmdSlice = []string{
			cluster.NodeOS, cluster.FQDN, "", "", ip,
			cluster.Config.InstallMode, cluster.Config.Version, cluster.Config.Channel,
			cluster.Config.DataStore, cluster.Config.ExternalDbEndpoint, cluster.Config.ServerFlags,
			os.Getenv("username"), os.Getenv("password"), "both",
		}
		if product == "k3s" {
			if cluster.Config.SplitRoles.Enabled {
				cmdSlice = slices.Insert(
					cmdSlice, 8, strconv.Itoa(cluster.Config.SplitRoles.EtcdOnly))
			} else {
				cmdSlice = slices.Insert(cmdSlice, 8, "")
			}
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
		cmdBuilder.WriteString(fmt.Sprintf(`%q `, str))
	}

	return cmdBuilder.String()
}
