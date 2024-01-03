package testcase

import (
	"fmt"
	"os"

	"github.com/rancher/distros-test-framework/config"
	"github.com/rancher/distros-test-framework/factory"
	"github.com/rancher/distros-test-framework/pkg/aws"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/ginkgo/v2"
)

const (
	server = "server"
	agent  = "agent"
	worker = "worker"
	master = "master"
)

func TestUpgradeReplaceNode(version string) error {
	if version == "" {
		return shared.ReturnLogError("version not sent.\n")
	}

	a, err := aws.AddAwsNode()
	if err != nil {
		return err
	}

	c, err := FetchCluster()
	if err != nil {
		return err
	}

	serverLeaderIp := c.ServerIPs[0]
	token, err := fetchToken(serverLeaderIp)
	if err != nil {
		return err
	}

	if err = replaceServers(c, a, serverLeaderIp, token, version); err != nil {
		return err
	}

	if err = replaceWorkers(c, a, serverLeaderIp, token, version); err != nil {
		return err
	}

	if dellErr := a.DeleteInstance(serverLeaderIp); dellErr != nil {
		return dellErr
	}

	return nil
}

//
// 	var (
// 		serverNames,
// 		newServerIds,
// 		newServerEips,
// 		newServerPips []string
// 		createErr error
// 	)
// 	for i := 0; i < len(c.ServerIPs); i++ {
// 		serverNames = append(serverNames, fmt.Sprintf("%s-server-%d", os.Getenv("resource_name"), i+1))
// 	}
//
// 	newServerEips, newServerPips, newServerIds, createErr = a.CreateInstances(serverNames...)
// 	if createErr != nil {
// 		return createErr
// 	}
// 	shared.LogLevel("info", "created server public ips: %s\nserver private ips: %s\n ids:%s\n", newServerEips, newServerPips, newServerIds)
//
// 	if scpErr := scpToNewNodes(c.Config.Product, master, newServerEips); scpErr != nil {
// 		return scpErr
// 	}
//
// 	for i, eip := range newServerEips {
// 		if i >= len(newServerPips) {
// 			return fmt.Errorf("mismatch in the length of external IPs and private Ips")
// 		}
// 		pip := newServerPips[i]
//
// 		joinCmd, installErr := parseJoinCmd(c.Config.Product, server, serverLeaderIp, token, version, eip, pip)
// 		if installErr != nil {
// 			return installErr
// 		}
// 		joinErr := joinNode(server, joinCmd, eip)
// 		if joinErr != nil {
// 			return joinErr
// 		}
//
// 		// will delete server nodes except the first one
// 		oldServersIps := c.ServerIPs[1:]
// 		if delErr := a.DeleteInstance(oldServersIps[i]); delErr != nil {
// 			return delErr
// 		}
// 	}
//
// 	//
// 	workersLenght := len(c.AgentIPs)
// 	if workersLenght > 0 {
// 		var workerNames []string
// 		var workerIds, newWorkerEips, newWorkerPips []string
// 		for i := 0; i < workersLenght; i++ {
// 			workerNames = append(workerNames, fmt.Sprintf("%s-agent-%d", os.Getenv("resource_name"), i))
// 		}
//
// 		newWorkerEips, newWorkerPips, workerIds, err = a.CreateInstances(workerNames...)
// 		if err != nil {
// 			return err
// 		}
// 		shared.LogLevel("info", "created\nworker ips: %s\nworker private ips:%s\nworker ids:%s\n", newWorkerEips, newWorkerPips, workerIds)
//
// 		for i := workersLenght - 1; i >= 0; i++ {
// 			currentAgent := c.AgentIPs[i]
// 			err = a.DeleteInstance(currentAgent)
// 			if err != nil {
// 				return err
// 			}
// 		}
//
// 		var cmd string
// 		var installErr error
// 		for i, eip := range newWorkerEips {
// 			if scpErr := scpToNewNodes(eip, c.Config.Product, newWorkerEips); scpErr != nil {
// 				return scpErr
// 			}
//
// 			if i >= len(newWorkerPips) {
// 				return fmt.Errorf("mismatch in the length of newServerEips and newServerPips")
// 			}
// 			pip := newWorkerPips[i]
//
// 			cmd, installErr = parseJoinCmd(c.Config.Product, agent, serverLeaderIp, token, version, eip, pip)
// 			if installErr != nil {
// 				return installErr
// 			}
// 			err = joinNode(agent, cmd, serverLeaderIp)
// 			if err != nil {
// 				return err
// 			}
// 		}
// 	}
//
// 	// delete serverLeaderIp
// 	err = a.DeleteInstance(serverLeaderIp)
// 	if err != nil {
// 		return err
// 	}
//
// 	// get first server node name and patch it
// 	// _, err = shared.RunCommandOnNode("kubectl patch node *****{nodename} -p '{\"metadata\":{\"finalizers\":[]}}' --type=merge", newServerEips[0])
// 	// if err != nil {
// 	// 	return err
// 	// }
//
// 	return nil
// }

func fetchToken(ip string) (string, error) {
	token, err := shared.RunCommandOnNode("sudo -i cat /tmp/nodetoken", ip)
	if err != nil {
		return "", err
	}

	return token, nil
}

func joinNode(nodetype, cmd, serverIp string) error {
	var err error

	switch nodetype {
	case server:
		err = joinServer(cmd, serverIp)
		if err != nil {
			return err
		}
	case agent:
		err = joinAgent(cmd, serverIp)
		if err != nil {
			return err
		}
	default:
		return shared.ReturnLogError("invalid node type: %s\n", nodetype)
	}

	return nil
}

func joinServer(cmd, ip string) error {
	_, err := shared.RunCommandOnNode(cmd, ip)
	if err != nil {
		return err
	}

	return nil
}

func joinAgent(cmd, ip string) error {
	_, err := shared.RunCommandOnNode(cmd, ip)
	if err != nil {
		return err
	}

	return nil
}

func parseJoinCmd(product, nodetype, serverIp, token, version, selfExternalIp, selfPrivateIp string) (string, error) {
	if product != "rke2" && product != "k3s" {
		return "", shared.ReturnLogError("unsupported product: %s\n", product)
	}

	if err := config.SetEnv(shared.BasePath() + fmt.Sprintf("/config/%s.tfvars", product)); err != nil {
		return "", shared.ReturnLogError("error loading tf vars: %w\n", err)
	}

	var flags string
	if nodetype == server {
		flags = fmt.Sprintf("'%s'", os.Getenv("server_flags"))
		nodetype = "master"
	} else {
		flags = fmt.Sprintf("'%s'", os.Getenv("worker_flags"))
		nodetype = "agent"
	}

	datastoreEndpoint := ""
	ipv6 := ""

	var cmd string
	if nodetype == agent {
		cmd = fmt.Sprintf(
			"sudo /tmp/join_%s_%s.sh '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' %s '%s'",
			product,
			nodetype,
			os.Getenv("node_os"),
			serverIp,
			token,
			selfExternalIp,
			selfPrivateIp,
			ipv6,
			os.Getenv("install_mode"),
			version,
			os.Getenv(fmt.Sprintf("%s_channel", product)),
			flags,
			os.Getenv("username"),
			os.Getenv("password"),
		)
	} else {
		cmd = fmt.Sprintf(
			"sudo /tmp/join_%s_%s.sh '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' %s '%s' '%s'",
			product,
			nodetype,
			os.Getenv("node_os"),
			serverIp,
			serverIp,
			token,
			selfExternalIp,
			selfPrivateIp,
			ipv6,
			os.Getenv("install_mode"),
			version,
			os.Getenv(fmt.Sprintf("%s_channel", product)),
			os.Getenv("datastore_type"),
			datastoreEndpoint,
			flags,
			os.Getenv("username"),
			os.Getenv("password"),
		)
	}

	return cmd, nil
}

func scpToNewNodes(product, nodeType string, newNodeIps []string) error {
	if newNodeIps == nil {
		return shared.ReturnLogError("newServerIps should send at least one ip\n")
	}

	for _, ip := range newNodeIps {
		switch product {
		case "k3s":
			if nodeType == worker {
				cisWorkerLocalPath := shared.BasePath() + "/modules/k3s/worker/cis_worker_config.yaml"
				cisWorkerRemotePath := "/tmp/cis_worker_config.yaml"

				joinLocalPath := shared.BasePath() + fmt.Sprintf("/modules/install/join_%s_%s.sh", product, agent)
				joinRemotePath := fmt.Sprintf("/tmp/join_%s_%s.sh", product, agent)

				if err := shared.RunScp(
					ip,
					product,
					[]string{cisWorkerLocalPath, joinLocalPath},
					[]string{cisWorkerRemotePath, joinRemotePath},
				); err != nil {
					return err
				}
			} else {
				cisMasterLocalPath := shared.BasePath() + "/modules/k3s/master/cis_master_config.yaml"
				cisMasterRemotePath := "/tmp/cis_master_config.yaml"

				clusterLevelpssLocalPath := shared.BasePath() + "/modules/k3s/master/cluster-level-pss.yaml"
				clusterLevelpssRemotePath := "/tmp/cluster-level-pss.yaml"

				auditLocalPath := shared.BasePath() + "/modules/k3s/master/audit.yaml"
				auditRemotePath := "/tmp/audit.yaml"

				policyLocalPath := shared.BasePath() + "/modules/k3s/master/policy.yaml"
				policyRemotePath := "/tmp/policy.yaml"

				ingressPolicyLocalPath := shared.BasePath() + "/modules/k3s/master/ingresspolicy.yaml"
				ingressPolicyRemotePath := "/tmp/ingresspolicy.yaml"

				joinLocalPath := shared.BasePath() + fmt.Sprintf("/modules/install/join_%s_%s.sh", product, master)
				joinRemotePath := fmt.Sprintf("/tmp/join_%s_%s.sh", product, master)

				if err := shared.RunScp(ip, product, []string{cisMasterLocalPath, clusterLevelpssLocalPath, auditLocalPath, policyLocalPath, ingressPolicyLocalPath, joinLocalPath},
					[]string{cisMasterRemotePath, clusterLevelpssRemotePath, auditRemotePath, policyRemotePath, ingressPolicyRemotePath, joinRemotePath}); err != nil {
					return err
				}
			}
		case "rke2":
			joinLocalPath := shared.BasePath() + fmt.Sprintf("/modules/install/join_%s_%s.sh", product, master)
			joinRemotePath := fmt.Sprintf("/tmp/join_%s_%s.sh", product, master)
			if err := shared.RunScp(ip, product, []string{joinLocalPath}, []string{joinRemotePath}); err != nil {
				return err
			}
		default:
			return shared.ReturnLogError("unsupported product: %s\n", product)
		}
	}

	return nil
}

func replaceServers(c *factory.Cluster, a *aws.Client, serverLeaderIp, token, version string) error {
	var (
		serverNames,
		newServerIds,
		newServerEips,
		newServerPips []string
		createErr error
	)

	for i := 0; i < len(c.ServerIPs); i++ {
		serverNames = append(serverNames, fmt.Sprintf("%s-server-%d", os.Getenv("resource_name"), i+1))
	}

	newServerEips, newServerPips, newServerIds, createErr = a.CreateInstances(serverNames...)
	if createErr != nil {
		return createErr
	}
	shared.LogLevel("info", "created server public ips: %s\nserver private ips: %s\n ids:%s\n", newServerEips, newServerPips, newServerIds)

	if scpErr := scpToNewNodes(c.Config.Product, master, newServerEips); scpErr != nil {
		return scpErr
	}

	for i, eip := range newServerEips {
		if i >= len(newServerPips) {
			return fmt.Errorf("mismatch in the length of external IPs and private Ips")
		}
		pip := newServerPips[i]

		joinCmd, installErr := parseJoinCmd(c.Config.Product, server, serverLeaderIp, token, version, eip, pip)
		if installErr != nil {
			return installErr
		}
		joinErr := joinNode(server, joinCmd, eip)
		if joinErr != nil {
			return joinErr
		}

		// will delete server nodes except the first one
		oldServersIps := c.ServerIPs[1:]
		if delErr := a.DeleteInstance(oldServersIps[i]); delErr != nil {
			return delErr
		}
	}

	return nil
}

func replaceWorkers(c *factory.Cluster, a *aws.Client, serverLeaderIp, token, version string) error {
	workersLength := len(c.AgentIPs)
	if workersLength > 0 {
		var workerNames []string
		var workerIds, newWorkerEips, newWorkerPips []string
		for i := 0; i < workersLength; i++ {
			workerNames = append(workerNames, fmt.Sprintf("%s-agent-%d", os.Getenv("resource_name"), i))
		}

		newWorkerEips, newWorkerPips, workerIds, err := a.CreateInstances(workerNames...)
		if err != nil {
			return err
		}
		shared.LogLevel("info", "created\nworker ips: %s\nworker private ips:%s\nworker ids:%s\n", newWorkerEips, newWorkerPips, workerIds)

		for i := workersLength - 1; i >= 0; i-- {
			currentAgent := c.AgentIPs[i]
			err = a.DeleteInstance(currentAgent)
			if err != nil {
				return err
			}
		}

		var cmd string
		var installErr error
		for i, eip := range newWorkerEips {
			if scpErr := scpToNewNodes(eip, c.Config.Product, newWorkerEips); scpErr != nil {
				return scpErr
			}

			if i >= len(newWorkerPips) {
				return fmt.Errorf("mismatch in the length of newServerEips and newServerPips")
			}
			pip := newWorkerPips[i]

			cmd, installErr = parseJoinCmd(c.Config.Product, agent, serverLeaderIp, token, version, eip, pip)
			if installErr != nil {
				return installErr
			}
			err = joinNode(agent, cmd, serverLeaderIp)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func FetchCluster() (*factory.Cluster, error) {
	cluster := factory.ClusterConfig(GinkgoT())
	return cluster, nil
}
