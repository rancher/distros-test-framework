package testcase

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/rancher/distros-test-framework/internal/pkg/aws"
	"github.com/rancher/distros-test-framework/internal/pkg/customflag"
	"github.com/rancher/distros-test-framework/internal/pkg/k8s"
	"github.com/rancher/distros-test-framework/internal/provisioning/driver"
	"github.com/rancher/distros-test-framework/internal/provisioning/legacy"
	"github.com/rancher/distros-test-framework/internal/resources"

	. "github.com/onsi/gomega"
)

const (
	agent  = "agent"
	master = "master"
)

func TestUpgradeReplaceNode(cluster *driver.Cluster,
	flags *customflag.FlagConfig,
) {
	version := flags.InstallMode.String()
	channel := flags.Channel.String()
	if version == "" {
		Expect(version).NotTo(BeEmpty(), "version/commit is empty")
	}

	awsClient := getAwsClient(cluster)
	resourceName := os.Getenv("resource_name")

	// create and prepare the servers
	var newExternalServerIps, newPrivateServerIps []string
	newExternalServerIps, newPrivateServerIps = createAndPrepNodes(awsClient, cluster, server, resourceName)

	serverLeaderIP := cluster.ServerIPs[0]
	token, err := resources.FetchToken(cluster.Config.Product, serverLeaderIP)
	Expect(err).NotTo(HaveOccurred(), err)

	serverErr := nodeReplaceServers(cluster, awsClient, serverLeaderIP, token,
		version, channel, resourceName, newExternalServerIps, newPrivateServerIps)
	Expect(serverErr).NotTo(HaveOccurred(), serverErr)
	resources.LogLevel("info", "Server control plane nodes replaced with ips: %s\n", newExternalServerIps)

	// replace agents only if exists.
	if len(cluster.AgentIPs) > 0 {
		nodeReplaceAgents(cluster, awsClient, version, channel, serverLeaderIP, token, resourceName)
	}
	// delete the last remaining server = leader.
	delErr := deleteRemainServer(serverLeaderIP, awsClient)
	Expect(delErr).NotTo(HaveOccurred(), delErr)
	resources.LogLevel("debug", "Last Server deleted ip: %s\n", serverLeaderIP)

	clusterErr := validateClusterHealth()
	if clusterErr != nil {
		resources.LogLevel("error", "error validating cluster health: %w\n", clusterErr)
	}
}

func scpToNewNodes(cluster *driver.Cluster, nodeType string, newNodeIps []string) error {
	if nodeType != master && nodeType != agent {
		return resources.ReturnLogError("unsupported nodetype: %s\n", nodeType)
	}
	if newNodeIps == nil {
		return resources.ReturnLogError("newNodeIps should contain at least one ip\n")
	}

	if cluster.Config.Product != "k3s" && cluster.Config.Product != "rke2" {
		return resources.ReturnLogError("unsupported product: %s\n", cluster.Config.Product)
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
				chanErr <- resources.ReturnLogError("error scp files to new nodes: %w\n", err)
				close(chanErr)
			}
		}(ip)
	}
	wg.Wait()
	close(chanErr)

	return nil
}

func scpRke2Files(cluster *driver.Cluster, nodeType, ip string) error {
	if nodeType != master && nodeType != agent {
		return resources.ReturnLogError("unsupported nodetype: %s\n", nodeType)
	}
	joinLocalPath := resources.BasePath() + fmt.Sprintf("/modules/install/join_rke2_%s.sh", nodeType)
	joinRemotePath := fmt.Sprintf("/var/tmp/join_rke2_%s.sh", nodeType)

	if err := resources.RunScp(cluster, ip, []string{joinLocalPath}, []string{joinRemotePath}); err != nil {
		return resources.ReturnLogError("error running scp: %w with ip: %s", err, ip)
	}

	return nil
}

func scpK3sFiles(cluster *driver.Cluster, nodeType, ip string) error {
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

func k3sAgentSCP(cluster *driver.Cluster, ip string) error {
	cisWorkerLocalPath := resources.BasePath() + "/modules/k3s/worker/cis_worker_config.yaml"
	cisWorkerRemotePath := "/tmp/cis_worker_config.yaml"

	joinLocalPath := resources.BasePath() + fmt.Sprintf("/modules/install/join_k3s_%s.sh", agent)
	joinRemotePath := fmt.Sprintf("/var/tmp/join_k3s_%s.sh", agent)

	return resources.RunScp(
		cluster,
		ip,
		[]string{cisWorkerLocalPath, joinLocalPath},
		[]string{cisWorkerRemotePath, joinRemotePath},
	)
}

func k3sServerSCP(cluster *driver.Cluster, ip string) error {
	cisMasterLocalPath := resources.BasePath() + "/modules/k3s/master/cis_master_config.yaml"
	cisMasterRemotePath := "/tmp/cis_master_config.yaml"

	clusterLevelpssLocalPath := resources.BasePath() + "/modules/k3s/master/cluster-level-pss.yaml"
	clusterLevelpssRemotePath := "/tmp/cluster-level-pss.yaml"

	auditLocalPath := resources.BasePath() + "/modules/k3s/master/audit.yaml"
	auditRemotePath := "/tmp/audit.yaml"

	policyLocalPath := resources.BasePath() + "/modules/k3s/master/policy.yaml"
	policyRemotePath := "/tmp/policy.yaml"

	ingressPolicyLocalPath := resources.BasePath() + "/modules/k3s/master/ingresspolicy.yaml"
	ingressPolicyRemotePath := "/tmp/ingresspolicy.yaml"

	joinLocalPath := resources.BasePath() + fmt.Sprintf("/modules/install/join_k3s_%s.sh", master)
	joinRemotePath := fmt.Sprintf("/var/tmp/join_k3s_%s.sh", master)

	return resources.RunScp(
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
	cluster *driver.Cluster,
	a *aws.Client,
	serverLeaderIp, token, version, channel, resourceName string,
	newExternalServerIps, newPrivateServerIps []string,
) error {
	if token == "" {
		return resources.ReturnLogError("token not sent\n")
	}

	if len(newExternalServerIps) == 0 || len(newPrivateServerIps) == 0 {
		return resources.ReturnLogError("externalIps or privateIps empty\n")
	}

	// join the first new server.
	newFirstServerIP := newExternalServerIps[0]
	err := serverJoin(cluster, a, serverLeaderIp, token, version, channel, newFirstServerIP, newPrivateServerIps[0])
	if err != nil {
		resources.LogLevel("error", "error joining first server: %w\n", err)

		return err
	}

	// delete first the server that is not the leader neither the server ip in the kubeconfig.
	oldServerIPs := cluster.ServerIPs
	if delErr := deleteRemainServer(oldServerIPs[len(oldServerIPs)-2], a); delErr != nil {
		resources.LogLevel("error", "error deleting server: %w\n", delErr)

		return delErr
	}

	resources.LogLevel("info", "Proceeding to update kubeconfig file to point to new first server join %s\n",
		newFirstServerIP)
	kubeConfigUpdated, kbCfgErr := legacy.UpdateKubeConfig(newFirstServerIP, resourceName, cluster.Config.Product)
	if kbCfgErr != nil {
		return resources.ReturnLogError("error updating kubeconfig: %w with ip: %s", kbCfgErr, newFirstServerIP)
	}
	resources.LogLevel("debug", "Updated local kubeconfig with ip: %s", newFirstServerIP)

	nodeErr := validateNodeJoin(newFirstServerIP)
	if nodeErr != nil {
		resources.LogLevel("error", "error validating node join: %w with ip: %s", nodeErr, newFirstServerIP)

		return nodeErr
	}

	// join the rest of the servers and delete all except the leader.
	err = joinRemainServers(cluster, a, newExternalServerIps, newPrivateServerIps,
		oldServerIPs, serverLeaderIp, token, version, channel)
	if err != nil {
		return err
	}

	resources.LogLevel("info", "Updated kubeconfig base64 string:\n%s\n", kubeConfigUpdated)

	return nil
}

func buildJoinCmd(
	cluster *driver.Cluster,
	nodetype, serverIp, token, version, channel, selfExternalIP, selfPrivateIP, installEnableOrBoth string,
) (string, error) {
	if nodetype != master && nodetype != agent {
		return "", resources.ReturnLogError("unsupported nodetype: %s\n", nodetype)
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
		return "", resources.ReturnLogError("unsupported product: %s\n", cluster.Config.Product)
	}
}

func buildK3sCmd(
	cluster *driver.Cluster,
	nodetype, serverIP, token, version, channel, selfExternalIP string,
	selfPrivateIP, installMode, flags, installEnableOrBoth string,
) (string, error) {
	var cmd string
	ipv6 := ""
	if nodetype == agent {
		cmd = fmt.Sprintf(
			"sudo /var/tmp/join_k3s_%s.sh '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' %s '%s' '%s' '%s'",
			nodetype,
			cluster.NodeOS,
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
		datastoreEndpoint := cluster.Config.ExternalDb
		cmd = fmt.Sprintf(
			"sudo /var/tmp/join_k3s_%s.sh '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' %s '%s' '%s' '%s'",
			nodetype,
			cluster.NodeOS,
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
	cluster *driver.Cluster,
	nodetype, serverIp, token, version, channel string,
	selfExternalIp, selfPrivateIp, installMode, flags, installEnableOrBoth string,
) (string, error) {
	installMethod := os.Getenv("install_method")
	var cmd string
	ipv6 := ""
	if nodetype == agent {
		cmd = fmt.Sprintf(
			"sudo /var/tmp/join_rke2_%s.sh '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' %s '%s' '%s' '%s'",
			nodetype,
			cluster.NodeOS,
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
		datastoreEndpoint := cluster.Config.ExternalDb
		arguments := fmt.Sprintf(
			"'%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' %s '%s' '%s' '%s'",
			cluster.NodeOS,
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
		cmd = fmt.Sprintf(
			"sudo /var/tmp/join_rke2_%s.sh %s", nodetype, arguments)
	}

	return cmd, nil
}

func joinRemainServers(
	cluster *driver.Cluster,
	a *aws.Client,
	newExternalServerIps,
	newPrivateServerIps,
	oldServerIPs []string,
	serverLeaderIp,
	token,
	version,
	channel string,
) error {
	for i := 1; i <= len(newExternalServerIps[1:]); i++ {
		privateIp := newPrivateServerIps[i]
		externalIp := newExternalServerIps[i]

		if i < len(oldServerIPs[1:]) {
			if delErr := deleteRemainServer(oldServerIPs[len(oldServerIPs)-1], a); delErr != nil {
				resources.LogLevel("error", "error deleting server: %w\n for ip: %s", delErr, oldServerIPs[i])

				return delErr
			}
		}

		joinErr := serverJoin(cluster, a, serverLeaderIp, token, version, channel, externalIp, privateIp)
		if joinErr != nil {
			resources.LogLevel("error", "error joining server: %w with ip: %s\n", joinErr, externalIp)

			return joinErr
		}

		validateJoinErr := validateNodeJoin(externalIp)
		if validateJoinErr != nil {
			resources.LogLevel("error", "error validating node join: %w with ip: %s", validateJoinErr, externalIp)

			return validateJoinErr
		}
	}

	return nil
}

func validateNodeJoin(ip string) error {
	node, err := resources.GetNodeNameByIP(ip)
	if err != nil {
		return resources.ReturnLogError("error getting node name by ip:%s %w\n", ip, err)
	}
	if node == "" {
		return resources.ReturnLogError("node not found\n")
	}
	node = strings.TrimSpace(node)

	resources.LogLevel("info", "Node joined: %s with ip: %s", node, ip)

	return nil
}

func serverJoinSlemicro(cluster *driver.Cluster,
	awsClient *aws.Client,
	serverLeaderIP, token, version, channel, newExternalIP, newPrivateIP string,
) error {
	// For slemicro nodes, we perform only 'install' step at this stage.
	resources.LogLevel("debug", "Running Install on: %s", newExternalIP)
	joinStepsErr := joinSteps(cluster, serverLeaderIP, token, version, channel,
		newExternalIP, newPrivateIP, "install")
	if joinStepsErr != nil {
		return resources.ReturnLogError("error installing product (k3s | rke2) %w\n", joinStepsErr)
	}

	// reboot nodes.
	rebootNodeAndWait(awsClient, newExternalIP)

	// enable service post reboot.
	resources.LogLevel("debug", "Enable Services on: %s", newExternalIP)
	enableErr := joinSteps(cluster, serverLeaderIP, token, version, channel,
		newExternalIP, newPrivateIP, "enable")
	if enableErr != nil {
		return resources.ReturnLogError("error enabling service for product (k3s | rke2) %w\n", enableErr)
	}

	return nil
}

func serverJoin(cluster *driver.Cluster,
	awsClient *aws.Client,
	serverLeaderIP, token, version, channel, newExternalIP, newPrivateIP string,
) error {
	if cluster.NodeOS == "slemicro" {
		return serverJoinSlemicro(cluster, awsClient, serverLeaderIP, token, version, channel, newExternalIP, newPrivateIP)
	}

	joinStepsErr := joinSteps(cluster, serverLeaderIP, token, version, channel,
		newExternalIP, newPrivateIP, "both")
	if joinStepsErr != nil {
		return resources.ReturnLogError("error joining node %w\n", joinStepsErr)
	}

	return nil
}

func joinSteps(cluster *driver.Cluster,
	serverLeaderIP, token, version, channel string,
	newExternalIP, newPrivateIP, installEnableOrBoth string,
) error {
	joinCmd, parseErr := buildJoinCmd(cluster, master, serverLeaderIP, token,
		version, channel, newExternalIP, newPrivateIP, installEnableOrBoth)
	if parseErr != nil {
		return resources.ReturnLogError("error parsing join command for join step: %s %w\n", installEnableOrBoth, parseErr)
	}
	var delayTime bool
	if installEnableOrBoth == "both" || installEnableOrBoth == "enable" {
		delayTime = true
	} else {
		delayTime = false
	}
	if executeErr := executeJoinCmd(joinCmd, newExternalIP, delayTime); executeErr != nil {
		return resources.ReturnLogError("error performing install or enable action on node: %s %w\n",
			installEnableOrBoth, executeErr)
	}

	return nil
}

func deleteRemainServer(ip string, a *aws.Client) error {
	if ip == "" {
		return resources.ReturnLogError("ip not sent\n")
	}

	if delNodeErr := resources.DeleteNode(ip); delNodeErr != nil {
		resources.LogLevel("error", "error deleting server: %w\n", delNodeErr)

		return delNodeErr
	}
	resources.LogLevel("debug", "Node IP deleted from the cluster: %s\n", ip)

	err := a.DeleteInstance(ip)
	if err != nil {
		return err
	}

	return nil
}

func nodeReplaceAgents(
	cluster *driver.Cluster,
	awsClient *aws.Client,
	version,
	channel,
	serverLeaderIp,
	token,
	resourceName string,
) {
	newExternalAgentIps, newPrivateAgentIps := createAndPrepNodes(awsClient, cluster, agent, resourceName)

	agentErr := replaceAgents(cluster, awsClient, serverLeaderIp, token, version, channel,
		newExternalAgentIps, newPrivateAgentIps)
	Expect(agentErr).NotTo(HaveOccurred(), "error replacing agents: %s", agentErr)

	resources.LogLevel("info", "Agent nodes replaced with ips: %s\n", newExternalAgentIps)
}

func replaceAgents(
	cluster *driver.Cluster,
	a *aws.Client,
	serverLeaderIp, token, version, channel string,
	newExternalAgentIps, newPrivateAgentIps []string,
) error {
	if token == "" {
		return resources.ReturnLogError("token not sent\n")
	}

	if len(newExternalAgentIps) == 0 || len(newPrivateAgentIps) == 0 {
		return resources.ReturnLogError("externalIps or privateIps empty\n")
	}

	if err := deleteAgents(a, cluster); err != nil {
		resources.LogLevel("error", "error deleting agent: %w\n", err)

		return err
	}

	for i, externalIp := range newExternalAgentIps {
		privateIp := newPrivateAgentIps[i]

		joinErr := joinAgent(cluster, a, serverLeaderIp, token, version, channel, externalIp, privateIp)
		if joinErr != nil {
			resources.LogLevel("error", "error joining agent: %w\n", joinErr)

			return joinErr
		}
	}

	return nil
}

func deleteAgents(a *aws.Client, c *driver.Cluster) error {
	for _, i := range c.AgentIPs {
		if deleteNodeErr := resources.DeleteNode(i); deleteNodeErr != nil {
			resources.LogLevel("error", "error deleting agent: %w\n", deleteNodeErr)

			return deleteNodeErr
		}

		err := a.DeleteInstance(i)
		if err != nil {
			return err
		}
		resources.LogLevel("debug", "Instance IP deleted from cloud provider: %s\n", i)
	}

	return nil
}

func joinAgentSlemicro(cluster *driver.Cluster, awsClient *aws.Client,
	serverIp, token, version, channel, selfExternalIp, selfPrivateIp string,
) error {
	// For slemicro nodes, we perform only 'install' step at this stage.
	resources.LogLevel("debug", "Running Install step for ip: %s", selfExternalIp)
	cmd, parseErr := buildJoinCmd(cluster, agent, serverIp, token, version,
		channel, selfExternalIp, selfPrivateIp, "install")
	if parseErr != nil {
		return resources.ReturnLogError("error parsing install command: %w\n", parseErr)
	}

	if installErr := executeJoinCmd(cmd, selfExternalIp, false); installErr != nil {
		return resources.ReturnLogError("error on install step on agent node: %w\n", installErr)
	}

	// reboot nodes.
	rebootNodeAndWait(awsClient, selfExternalIp)

	// enable services post reboot.
	cmd, parseErr = buildJoinCmd(cluster, agent, serverIp, token, version,
		channel, selfExternalIp, selfPrivateIp, "enable")
	if parseErr != nil {
		return resources.ReturnLogError("error parsing enable commands: %w\n", parseErr)
	}
	if enableErr := executeJoinCmd(cmd, selfExternalIp, true); enableErr != nil {
		return resources.ReturnLogError("error enabling services during join of agent node: %w\n", enableErr)
	}

	return nil
}

func joinAgent(cluster *driver.Cluster, awsClient *aws.Client,
	serverIp, token, version, channel, selfExternalIp, selfPrivateIp string,
) error {
	if cluster.NodeOS == "slemicro" {
		return joinAgentSlemicro(cluster, awsClient, serverIp, token, version, channel, selfExternalIp, selfPrivateIp)
	}

	cmd, parseErr := buildJoinCmd(cluster, agent, serverIp, token, version,
		channel, selfExternalIp, selfPrivateIp, "both")
	if parseErr != nil {
		return resources.ReturnLogError("error parsing join(both) commands: %w\n", parseErr)
	}

	if joinErr := executeJoinCmd(cmd, selfExternalIp, true); joinErr != nil {
		return resources.ReturnLogError("error on step join(both) on agent node: %w\n", joinErr)
	}

	return nil
}

func executeJoinCmd(cmd, ip string, delayTime bool) error {
	if cmd == "" {
		return resources.ReturnLogError("cmd not sent\n")
	}
	if ip == "" {
		return resources.ReturnLogError("server IP not sent\n")
	}

	resources.LogLevel("debug", "Executing: %s on ip: %s", cmd, ip)
	res, err := resources.RunCommandOnNode(cmd, ip)
	if err != nil {
		return resources.ReturnLogError("error running cmd on node: %w\n", err)
	}

	res = strings.TrimSpace(res)
	if strings.Contains(res, "service failed") {
		resources.LogLevel("error", "join node response: %s\n", res)

		return resources.ReturnLogError("error joining node: %s\n", res)
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

func rebootEc2Instance(awsClient *aws.Client, ip string) {
	serverInstanceID, getErr := awsClient.GetInstanceIDByIP(ip)
	Expect(getErr).NotTo(HaveOccurred())

	resources.LogLevel("debug", "Rebooting instance id: %s", serverInstanceID)
	rebootError := awsClient.RebootInstance(serverInstanceID)
	Expect(rebootError).NotTo(HaveOccurred())
}

func rebootNodeAndWait(awsClient *aws.Client, ip string) {
	rebootEc2Instance(awsClient, ip)

	sshErr := resources.WaitForSSHReady(ip)
	Expect(sshErr).NotTo(HaveOccurred())
}

func getAwsClient(cluster *driver.Cluster) *aws.Client {
	awsClient, err := aws.AddClient(cluster)
	Expect(err).NotTo(HaveOccurred(), "error adding aws nodes: %s", err)

	return awsClient
}

func prepSlemicro(awsClient *aws.Client, ip, nodeOS string) {
	resources.LogLevel("debug", "Pre-install Setup for nodeOS: %s on ip: %s for selinux", nodeOS, ip)

	cmd := "sudo transactional-update setup-selinux"
	resources.LogLevel("debug", "Running cmd: %s on ip: %s", cmd, ip)
	_, updateErr := resources.RunCommandOnNode(cmd, ip)
	Expect(updateErr).NotTo(HaveOccurred())

	rebootNodeAndWait(awsClient, ip)
}

func prepSlemicroNodes(ips []string, nodeOS string, awsClient *aws.Client) {
	if nodeOS == "slemicro" {
		for _, ip := range ips {
			prepSlemicro(awsClient, ip, nodeOS)
		}
	}
}

func getNodeNames(cluster *driver.Cluster, resourceName, nodeType string) []string {
	var nodeNames []string
	nodeCount := len(cluster.ServerIPs)
	if nodeType == "agent" {
		nodeCount = len(cluster.AgentIPs)
	}
	for i := 0; i < nodeCount; i++ {
		nodeNames = append(nodeNames, fmt.Sprintf("%s-%s-replace%d", resourceName, nodeType, i+1))
	}

	return nodeNames
}

func createAndPrepNodes(awsClient *aws.Client, cluster *driver.Cluster, nodeType, resourceName string) (
	newExternalIps []string, newPrivateIps []string,
) {
	// create aws ec2 instances
	names := getNodeNames(cluster, resourceName, nodeType)
	newExternalIps, newPrivateIps, instanceIds, createErr := awsClient.CreateInstances(names...)
	Expect(createErr).NotTo(HaveOccurred(), createErr)
	resources.LogLevel("debug", "Created %s nodes with public ips: %s and ids: %s\n",
		nodeType, newExternalIps, instanceIds)

	// If node os is slemicro prep/update it and reboot the node
	prepSlemicroNodes(newExternalIps, cluster.NodeOS, awsClient)

	// scp needed files to the new nodes
	var scpErr error
	if nodeType == agent {
		scpErr = scpToNewNodes(cluster, nodeType, newExternalIps)
	} else {
		scpErr = scpToNewNodes(cluster, master, newExternalIps)
	}
	Expect(scpErr).NotTo(HaveOccurred(), scpErr)
	resources.LogLevel("info", "Scp files to new %s nodes done\n", nodeType)

	return newExternalIps, newPrivateIps
}
