package testcase

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/rancher/distros-test-framework/pkg/aws"
	"github.com/rancher/distros-test-framework/pkg/customflag"
	"github.com/rancher/distros-test-framework/pkg/k8s"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/gomega"
)

const (
	agent  = "agent"
	master = "master"
)

func TestUpgradeReplaceNode(cluster *shared.Cluster, flags *customflag.FlagConfig) {
	version := flags.InstallMode.String()
	channel := flags.Channel.String()
	if version == "" {
		Expect(version).NotTo(BeEmpty(), "version/commit is empty")
	}

	resourceName := os.Getenv("resource_name")
	awsClient, err := aws.AddClient(cluster)
	Expect(err).NotTo(HaveOccurred(), "error adding aws nodes: %s", err)

	// create server names.
	var serverNames, instanceServerIds, newExternalServerIps, newPrivateServerIps []string
	for i := 0; i < len(cluster.ServerIPs); i++ {
		serverNames = append(serverNames, fmt.Sprintf("%s-server-replace%d", resourceName, i+1))
	}

	var createErr error
	newExternalServerIps, newPrivateServerIps, instanceServerIds, createErr = awsClient.CreateInstances(serverNames...)
	Expect(createErr).NotTo(HaveOccurred(), createErr)

	shared.LogLevel("debug", "Created server public ips: %s and ids: %s\n",
		newExternalServerIps, instanceServerIds)

	scpErr := scpToNewNodes(cluster, master, newExternalServerIps)
	Expect(scpErr).NotTo(HaveOccurred(), scpErr)
	shared.LogLevel("info", "Scp files to new server nodes done\n")

	serverLeaderIP := cluster.ServerIPs[0]
	token, err := shared.FetchToken(cluster.Config.Product, serverLeaderIP)
	Expect(err).NotTo(HaveOccurred(), err)

	// If node os is slemicro prep/update it and reboot the node.
	nodeOS := os.Getenv("node_os")
	shared.LogLevel("debug", "Testing Node OS: %s", nodeOS)
	for _, ip := range newExternalServerIps {
		if nodeOS == "slemicro" {
			cmd := "sudo transactional-update setup-selinux"
			_, updateErr := shared.RunCommandOnNode(cmd, ip)
			Expect(updateErr).NotTo(HaveOccurred())
			rebootInstances(awsClient, ip)
		}
	}
	// bracket sleep to ensure ssh to instance works instead of waiting for every node to get ready
	time.Sleep(20 * time.Second)

	serverErr := nodeReplaceServers(cluster, awsClient, resourceName, serverLeaderIP, token,
		version,
		channel,
		nodeOS,
		newExternalServerIps,
		newPrivateServerIps)
	Expect(serverErr).NotTo(HaveOccurred(), serverErr)
	shared.LogLevel("info", "Server control plane nodes replaced with ips: %s\n", newExternalServerIps)

	// replace agents only if exists.
	if len(cluster.AgentIPs) > 0 {
		nodeReplaceAgents(cluster, awsClient, version, channel, resourceName, serverLeaderIP, token, nodeOS)
	}
	// delete the last remaining server = leader.
	delErr := deleteRemainServer(serverLeaderIP, awsClient)
	Expect(delErr).NotTo(HaveOccurred(), delErr)
	shared.LogLevel("debug", "Last Server deleted ip: %s\n", serverLeaderIP)

	clusterErr := validateClusterHealth()
	if clusterErr != nil {
		shared.LogLevel("error", "error validating cluster health: %w\n", clusterErr)
	}
}

func scpToNewNodes(cluster *shared.Cluster, nodeType string, newNodeIps []string) error {
	if newNodeIps == nil {
		return shared.ReturnLogError("newServerIps should send at least one ip\n")
	}

	if cluster.Config.Product != "k3s" && cluster.Config.Product != "rke2" {
		return shared.ReturnLogError("unsupported product: %s\n", cluster.Config.Product)
	}

	chanErr := make(chan error, len(newNodeIps))
	var wg sync.WaitGroup

	for _, ip := range newNodeIps {
		wg.Add(1)
		go func(ip string) {
			defer wg.Done()
			var err error
			if cluster.Config.Product == "k3s" {
				err = scpK3sFiles(cluster, nodeType, ip)
			} else {
				err = scpRke2Files(cluster, nodeType, ip)
			}
			if err != nil {
				chanErr <- shared.ReturnLogError("error scp files to new nodes: %w\n", err)
				close(chanErr)
			}
		}(ip)
	}
	wg.Wait()
	close(chanErr)

	return nil
}

func scpRke2Files(cluster *shared.Cluster, nodeType, ip string) error {
	joinLocalPath := shared.BasePath() + fmt.Sprintf("/modules/install/join_rke2_%s.sh", nodeType)
	joinRemotePath := fmt.Sprintf("/var/tmp/join_rke2_%s.sh", nodeType)

	if err := shared.RunScp(cluster, ip, []string{joinLocalPath}, []string{joinRemotePath}); err != nil {
		return shared.ReturnLogError("error running scp: %w with ip: %s", err, ip)
	}

	return nil
}

func scpK3sFiles(cluster *shared.Cluster, nodeType, ip string) error {
	if nodeType == agent {
		err := k3sAgentSCP(cluster, ip)
		if err != nil {
			return err
		}
	} else {
		err := k3sServerSCP(cluster, ip)
		if err != nil {
			return err
		}
	}

	return nil
}

func k3sAgentSCP(cluster *shared.Cluster, ip string) error {
	cisWorkerLocalPath := shared.BasePath() + "/modules/k3s/worker/cis_worker_config.yaml"
	cisWorkerRemotePath := "/var/tmp/cis_worker_config.yaml"

	joinLocalPath := shared.BasePath() + fmt.Sprintf("/modules/install/join_k3s_%s.sh", agent)
	joinRemotePath := fmt.Sprintf("/var/tmp/join_k3s_%s.sh", agent)

	return shared.RunScp(
		cluster,
		ip,
		[]string{cisWorkerLocalPath, joinLocalPath},
		[]string{cisWorkerRemotePath, joinRemotePath},
	)
}

func k3sServerSCP(cluster *shared.Cluster, ip string) error {
	cisMasterLocalPath := shared.BasePath() + "/modules/k3s/master/cis_master_config.yaml"
	cisMasterRemotePath := "/var/tmp/cis_master_config.yaml"

	clusterLevelpssLocalPath := shared.BasePath() + "/modules/k3s/master/cluster-level-pss.yaml"
	clusterLevelpssRemotePath := "/var/tmp/cluster-level-pss.yaml"

	auditLocalPath := shared.BasePath() + "/modules/k3s/master/audit.yaml"
	auditRemotePath := "/var/tmp/audit.yaml"

	policyLocalPath := shared.BasePath() + "/modules/k3s/master/policy.yaml"
	policyRemotePath := "/var/tmp/policy.yaml"

	ingressPolicyLocalPath := shared.BasePath() + "/modules/k3s/master/ingresspolicy.yaml"
	ingressPolicyRemotePath := "/var/tmp/ingresspolicy.yaml"

	joinLocalPath := shared.BasePath() + fmt.Sprintf("/modules/install/join_k3s_%s.sh", master)
	joinRemotePath := fmt.Sprintf("/var/tmp/join_k3s_%s.sh", master)

	return shared.RunScp(
		cluster,
		ip,
		[]string{
			cisMasterLocalPath,
			clusterLevelpssLocalPath,
			auditLocalPath,
			policyLocalPath,
			ingressPolicyLocalPath,
			joinLocalPath,
		},
		[]string{
			cisMasterRemotePath,
			clusterLevelpssRemotePath,
			auditRemotePath,
			policyRemotePath,
			ingressPolicyRemotePath,
			joinRemotePath,
		})
}

func nodeReplaceServers(
	cluster *shared.Cluster,
	a *aws.Client,
	resourceName, serverLeaderIp, token, version, channel, nodeOS string,
	newExternalServerIps, newPrivateServerIps []string,
) error {
	if token == "" {
		return shared.ReturnLogError("token not sent\n")
	}

	if len(newExternalServerIps) == 0 || len(newPrivateServerIps) == 0 {
		return shared.ReturnLogError("externalIps or privateIps empty\n")
	}

	// join the first new server.
	newFirstServerIP := newExternalServerIps[0]
	err := serverJoin(cluster, a, serverLeaderIp, token, version, channel, newFirstServerIP, newPrivateServerIps[0], nodeOS)
	if err != nil {
		shared.LogLevel("error", "error joining first server: %w\n", err)

		return err
	}

	// delete first the server that is not the leader neither the server ip in the kubeconfig.
	oldServerIPs := cluster.ServerIPs
	if delErr := deleteRemainServer(oldServerIPs[len(oldServerIPs)-2], a); delErr != nil {
		shared.LogLevel("error", "error deleting server: %w\n", delErr)

		return delErr
	}

	shared.LogLevel("info", "Proceeding to update kubeconfig file to point to new first server join %s\n", newFirstServerIP)
	kubeConfigUpdated, kbCfgErr := shared.UpdateKubeConfig(newFirstServerIP, resourceName, cluster.Config.Product)
	if kbCfgErr != nil {
		return shared.ReturnLogError("error updating kubeconfig: %w with ip: %s", kbCfgErr, newFirstServerIP)
	}
	shared.LogLevel("debug", "Updated local kubeconfig with ip: %s", newFirstServerIP)

	nodeErr := validateNodeJoin(newFirstServerIP)
	if nodeErr != nil {
		shared.LogLevel("error", "error validating node join: %w with ip: %s", nodeErr, newFirstServerIP)

		return nodeErr
	}

	// join the rest of the servers and delete all except the leader.
	err = joinRemainServers(cluster, a, newExternalServerIps, newPrivateServerIps,
		oldServerIPs, serverLeaderIp, token, version, channel, nodeOS)
	if err != nil {
		return err
	}

	shared.LogLevel("info", "Updated kubeconfig base64 string:\n%s\n", kubeConfigUpdated)

	return nil
}

func buildJoinCmd(
	cluster *shared.Cluster,
	nodetype, serverIp, token, version, channel, selfExternalIP, selfPrivateIP, installEnableOrBoth string,
) (string, error) {
	if nodetype != master && nodetype != agent {
		return "", shared.ReturnLogError("unsupported nodetype: %s\n", nodetype)
	}

	var flags string
	var installMode string
	if nodetype == master {
		flags = fmt.Sprintf("'%s'", os.Getenv("server_flags"))
	} else {
		flags = fmt.Sprintf("'%s'", os.Getenv("worker_flags"))
	}

	if strings.HasPrefix(version, "v") {
		installMode = fmt.Sprintf("INSTALL_%s_VERSION", strings.ToUpper(cluster.Config.Product))
	} else {
		installMode = fmt.Sprintf("INSTALL_%s_COMMIT", strings.ToUpper(cluster.Config.Product))
	}

	switch cluster.Config.Product {
	case "k3s":
		return buildK3sCmd(
			cluster, nodetype, serverIp, token, version, channel, selfExternalIP,
			selfPrivateIP, installMode, flags, installEnableOrBoth)
	case "rke2":
		return buildRke2Cmd(
			cluster, nodetype, serverIp, token, version, channel, selfExternalIP,
			selfPrivateIP, installMode, flags, installEnableOrBoth)
	default:
		return "", shared.ReturnLogError("unsupported product: %s\n", cluster.Config.Product)
	}
}

func buildK3sCmd(
	cluster *shared.Cluster,
	nodetype, serverIP, token, version, channel, selfExternalIP, selfPrivateIP, installMode, flags, installEnableOrBoth string,
) (string, error) {
	var cmd string
	ipv6 := ""
	if nodetype == agent {
		cmd = fmt.Sprintf(
			"sudo /var/tmp/join_k3s_%s.sh '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' %s '%s' '%s' '%s'",
			nodetype,
			os.Getenv("node_os"),
			serverIP,
			token,
			selfExternalIP,
			selfPrivateIP,
			ipv6,
			installMode,
			version,
			channel,
			flags,
			os.Getenv("username"),
			os.Getenv("password"),
			installEnableOrBoth,
		)
	} else {
		datastoreEndpoint := cluster.Config.RenderedTemplate
		cmd = fmt.Sprintf(
			"sudo /var/tmp/join_k3s_%s.sh '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' %s '%s' '%s' '%s'",
			nodetype,
			os.Getenv("node_os"),
			serverIP,
			serverIP,
			token,
			selfExternalIP,
			selfPrivateIP,
			ipv6,
			installMode,
			version,
			channel,
			os.Getenv("datastore_type"),
			datastoreEndpoint,
			flags,
			os.Getenv("username"),
			os.Getenv("password"),
			installEnableOrBoth,
		)
	}

	return cmd, nil
}

func buildRke2Cmd(
	cluster *shared.Cluster,
	nodetype, serverIp, token, version, channel, selfExternalIp, selfPrivateIp, installMode, flags, installEnableOrBoth string,
) (string, error) {
	installMethod := os.Getenv("install_method")
	var cmd string
	ipv6 := ""
	if nodetype == agent {
		cmd = fmt.Sprintf(
			"sudo /var/tmp/join_rke2_%s.sh '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' %s '%s' '%s' '%s'",
			nodetype,
			os.Getenv("node_os"),
			serverIp,
			token,
			selfExternalIp,
			selfPrivateIp,
			ipv6,
			installMode,
			version,
			channel,
			installMethod,
			flags,
			os.Getenv("username"),
			os.Getenv("password"),
			installEnableOrBoth,
		)
	} else {
		datastoreEndpoint := cluster.Config.RenderedTemplate
		cmd = fmt.Sprintf(
			"sudo /var/tmp/join_rke2_%s.sh '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' %s '%s' '%s' '%s'",
			nodetype,
			os.Getenv("node_os"),
			serverIp,
			serverIp,
			token,
			selfExternalIp,
			selfPrivateIp,
			ipv6,
			installMode,
			version,
			channel,
			installMethod,
			os.Getenv("datastore_type"),
			datastoreEndpoint,
			flags,
			os.Getenv("username"),
			os.Getenv("password"),
			installEnableOrBoth,
		)
	}

	return cmd, nil
}

func joinRemainServers(
	cluster *shared.Cluster,
	a *aws.Client,
	newExternalServerIps,
	newPrivateServerIps,
	oldServerIPs []string,
	serverLeaderIp,
	token,
	version,
	channel,
	nodeOS string,
) error {
	for i := 1; i <= len(newExternalServerIps[1:]); i++ {
		privateIp := newPrivateServerIps[i]
		externalIp := newExternalServerIps[i]

		if i < len(oldServerIPs[1:]) {
			if delErr := deleteRemainServer(oldServerIPs[len(oldServerIPs)-1], a); delErr != nil {
				shared.LogLevel("error", "error deleting server: %w\n for ip: %s", delErr, oldServerIPs[i])

				return delErr
			}
		}

		if joinErr := serverJoin(cluster, a, serverLeaderIp, token, version, channel, externalIp, privateIp, nodeOS); joinErr != nil {
			shared.LogLevel("error", "error joining server: %w with ip: %s\n", joinErr, externalIp)

			return joinErr
		}

		joinErr := validateNodeJoin(externalIp)
		if joinErr != nil {
			shared.LogLevel("error", "error validating node join: %w with ip: %s", joinErr, externalIp)

			return joinErr
		}
	}

	return nil
}

func validateNodeJoin(ip string) error {
	node, err := shared.GetNodeNameByIP(ip)
	if err != nil {
		return shared.ReturnLogError("error getting node name by ip:%s %w\n", ip, err)
	}
	if node == "" {
		return shared.ReturnLogError("node not found\n")
	}
	node = strings.TrimSpace(node)

	shared.LogLevel("info", "Node joined: %s with ip: %s", node, ip)

	return nil
}

func serverJoin(cluster *shared.Cluster,
	awsClient *aws.Client,
	serverLeaderIP, token, version, channel, newExternalIP, newPrivateIP, nodeOS string) error {
	installOrBoth := "both"
	if nodeOS == "slemicro" {
		installOrBoth = "install"
		shared.LogLevel("debug", "Running Install on: %s", newExternalIP)
	}

	joinStepsErr := joinSteps(cluster, serverLeaderIP, token, version, channel,
		newExternalIP, newPrivateIP, installOrBoth)
	if joinStepsErr != nil {
		if nodeOS == "slemicro" {
			return shared.ReturnLogError("error installing product (k3s | rke2) %w\n", joinStepsErr)
		} else {
			return shared.ReturnLogError("error joining node %w\n", joinStepsErr)
		}
	}

	// reboot nodes and enable services for slemicro OS
	if nodeOS == "slemicro" {
		rebootInstances(awsClient, newExternalIP)
		sshErr := waitForSSHReady(newExternalIP)
		if sshErr != nil {
			return shared.ReturnLogError("error connecting via SSH to %s to run commands after reboot of node: %w\n", newExternalIP, sshErr)
		}

		// enable service post reboot
		shared.LogLevel("debug", "Enable Services on: %s", newExternalIP)
		enableErr := joinSteps(cluster, serverLeaderIP, token, version, channel,
			newExternalIP, newPrivateIP, "enable")
		if enableErr != nil {
			return shared.ReturnLogError("error enabling service for product (k3s | rke2) %w\n", enableErr)
		}
	}

	return nil
}

func joinSteps(cluster *shared.Cluster,
	serverLeaderIP, token, version, channel, newExternalIP, newPrivateIP, installEnableOrBoth string) error {
	joinCmd, parseErr := buildJoinCmd(cluster, master, serverLeaderIP, token,
		version, channel, newExternalIP, newPrivateIP, installEnableOrBoth)
	if parseErr != nil {
		return shared.ReturnLogError("error parsing join command for join step: %s %w\n", installEnableOrBoth, parseErr)
	}
	var delayTime bool
	if installEnableOrBoth == "both" || installEnableOrBoth == "enable" {
		delayTime = true
	} else {
		delayTime = false
	}
	if executeErr := execute(joinCmd, newExternalIP, delayTime); executeErr != nil {
		return shared.ReturnLogError("error performing install or enable action on node: %s %w\n", installEnableOrBoth, executeErr)
	}

	return nil
}

func deleteRemainServer(ip string, a *aws.Client) error {
	if ip == "" {
		return shared.ReturnLogError("ip not sent\n")
	}

	if delNodeErr := shared.DeleteNode(ip); delNodeErr != nil {
		shared.LogLevel("error", "error deleting server: %w\n", delNodeErr)

		return delNodeErr
	}
	shared.LogLevel("debug", "Node IP deleted from the cluster: %s\n", ip)

	err := a.DeleteInstance(ip)
	if err != nil {
		return err
	}

	return nil
}

func nodeReplaceAgents(
	cluster *shared.Cluster,
	awsClient *aws.Client,
	version,
	channel,
	resourceName,
	serverLeaderIp,
	token,
	nodeOS string,
) {
	// create agent names.
	var agentNames []string
	for i := 0; i < len(cluster.AgentIPs); i++ {
		agentNames = append(agentNames, fmt.Sprintf("%s-worker-replace%d", resourceName, i+1))
	}

	newExternalAgentIps,
		newPrivateAgentIps,
		instanceAgentIds,
		createAgentErr := awsClient.CreateInstances(agentNames...)
	Expect(createAgentErr).NotTo(HaveOccurred(), createAgentErr)

	shared.LogLevel("debug", "created worker ips: %s and worker ids: %s\n",
		newExternalAgentIps, instanceAgentIds)

	scpErr := scpToNewNodes(cluster, agent, newExternalAgentIps)
	Expect(scpErr).NotTo(HaveOccurred(), scpErr)
	shared.LogLevel("info", "Scp files to new worker nodes done\n")

	// If node os is slemicro prep/update it and reboot the node.
	for _, ip := range newExternalAgentIps {
		if nodeOS == "slemicro" {
			// method to prep slemicro call here
			cmd := "sudo transactional-update setup-selinux"
			_, updateErr := shared.RunCommandOnNode(cmd, ip)
			Expect(updateErr).NotTo(HaveOccurred())
			rebootInstances(awsClient, ip)
		}
	}
	// bracket sleep instead of waiting for every ip to get ssh ready
	shared.LogLevel("debug", "sleep 20 to ensure ssh works before next cmd")
	time.Sleep(20 * time.Second)

	agentErr := replaceAgents(cluster, awsClient, serverLeaderIp, token, version, channel, nodeOS,
		newExternalAgentIps, newPrivateAgentIps)
	Expect(agentErr).NotTo(HaveOccurred(), "error replacing agents: %s", agentErr)

	shared.LogLevel("info", "Agent nodes replaced with ips: %s\n", newExternalAgentIps)
}

func replaceAgents(
	cluster *shared.Cluster,
	a *aws.Client,
	serverLeaderIp, token, version, channel, nodeOS string,
	newExternalAgentIps, newPrivateAgentIps []string,
) error {
	if token == "" {
		return shared.ReturnLogError("token not sent\n")
	}

	if len(newExternalAgentIps) == 0 || len(newPrivateAgentIps) == 0 {
		return shared.ReturnLogError("externalIps or privateIps empty\n")
	}

	if err := deleteAgents(a, cluster); err != nil {
		shared.LogLevel("error", "error deleting agent: %w\n", err)

		return err
	}

	for i, externalIp := range newExternalAgentIps {
		privateIp := newPrivateAgentIps[i]

		joinErr := joinAgent(cluster, a, serverLeaderIp, token, version, channel, externalIp, privateIp, nodeOS)
		if joinErr != nil {
			shared.LogLevel("error", "error joining agent: %w\n", joinErr)

			return joinErr
		}
	}

	return nil
}

func deleteAgents(a *aws.Client, c *shared.Cluster) error {
	for _, i := range c.AgentIPs {
		if deleteNodeErr := shared.DeleteNode(i); deleteNodeErr != nil {
			shared.LogLevel("error", "error deleting agent: %w\n", deleteNodeErr)

			return deleteNodeErr
		}

		err := a.DeleteInstance(i)
		if err != nil {
			return err
		}
		shared.LogLevel("debug", "Instance IP deleted from cloud provider: %s\n", i)
	}

	return nil
}

func joinAgent(cluster *shared.Cluster, awsClient *aws.Client, serverIp, token, version, channel, selfExternalIp, selfPrivateIp, nodeOS string) error {
	installOrBoth := "both"
	if nodeOS == "slemicro" {
		installOrBoth = "install"
		shared.LogLevel("debug", "Running Install step for ip: %s", selfExternalIp)
	}

	cmd, parseErr := buildJoinCmd(cluster, agent, serverIp, token, version,
		channel, selfExternalIp, selfPrivateIp, installOrBoth)
	if parseErr != nil {
		return shared.ReturnLogError("error parsing join(both)|install: %s commands: %w\n", installOrBoth, parseErr)
	}
	var joinErr error
	if installOrBoth == "both" {
		joinErr = execute(cmd, selfExternalIp, true)
	} else {
		joinErr = execute(cmd, selfExternalIp, false)
	}

	if joinErr != nil {
		return shared.ReturnLogError("error on step join(both)|install: %s on agent node: %w\n", installOrBoth, joinErr)
	}

	// reboot nodes and enable services for slemicro OS
	if nodeOS == "slemicro" {
		rebootInstances(awsClient, selfExternalIp)
		sshErr := waitForSSHReady(selfExternalIp)
		if sshErr != nil {
			return shared.ReturnLogError("error connecting via SSH to %s to run commands after reboot of node: %w\n", selfExternalIp, sshErr)
		}
		// enable service post reboot
		cmd, parseErr = buildJoinCmd(cluster, agent, serverIp, token, version,
			channel, selfExternalIp, selfPrivateIp, "enable")
		if parseErr != nil {
			return shared.ReturnLogError("error parsing enable commands: %w\n", parseErr)
		}

		if joinErr := execute(cmd, selfExternalIp, true); joinErr != nil {
			return shared.ReturnLogError("error enabling services during join of agent node: %w\n", joinErr)
		}
	}

	return nil
}

func execute(cmd, ip string, delayTime bool) error {
	if cmd == "" {
		return shared.ReturnLogError("cmd not sent\n")
	}
	if ip == "" {
		return shared.ReturnLogError("server IP not sent\n")
	}

	shared.LogLevel("debug", "Executing: %s on ip: %s", cmd, ip)
	res, err := shared.RunCommandOnNode(cmd, ip)
	if err != nil {
		return shared.ReturnLogError("error running cmd %s on node: %w\n", cmd, err)
	}

	res = strings.TrimSpace(res)
	if strings.Contains(res, "service failed") {
		shared.LogLevel("error", "join node response: %s\n", res)

		return shared.ReturnLogError("error joining node: %s\n", res)
	}

	if delayTime {
		delay := time.After(40 * time.Second)
		// delay not meant to wait if node is joined, but rather to give time for all join process to complete under the hood
		<-delay
	}

	return nil
}

func validateClusterHealth() error {
	k8sC, err := k8s.AddClient()
	if err != nil {
		return fmt.Errorf("error adding k8s client: %w", err)
	}

	ok, err := k8sC.CheckClusterHealth(0)
	if err != nil {
		return fmt.Errorf("error checking cluster health: %w", err)
	}
	if !ok {
		return errors.New("cluster is not healthy")
	}

	return nil
}

func waitForSSHReady(ip string) error {
	ticker := time.NewTicker(10 * time.Second)
	timeout := time.After(2 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-timeout:
			return errors.New("timed out waiting for SSH Ready")
		case <-ticker.C:
			cmdOutput, sshErr := shared.RunCommandOnNode("ls -lrt", ip)
			if sshErr != nil {
				continue
			}
			if cmdOutput != "" {
				return nil
			}
		}
	}
}
