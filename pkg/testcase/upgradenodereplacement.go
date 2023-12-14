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

	dependencies, err := aws.AddAwsNode()
	if err != nil {
		return err
	}

	cluster := factory.ClusterConfig(GinkgoT())
	product, err := shared.GetProduct()
	if err != nil {
		return shared.ReturnLogError("error getting product: %w\n", err)
	}

	serverLeaderIp := cluster.ServerIPs[0]
	token, nodeErr := shared.RunCommandOnNode("sudo -i cat /tmp/nodetoken", serverLeaderIp)
	if nodeErr != nil {
		return nodeErr
	}

	var serverNames []string
	var serverIds, newServerIps []string
	var cErr error
	resourceName := os.Getenv("resource_name")

	for i := 0; i < len(cluster.ServerIPs); i++ {
		serverNames = append(serverNames, fmt.Sprintf("%s-server-%d", resourceName, i+1))
	}

	newServerIps, serverIds, cErr = dependencies.CreateInstances(serverNames...)
	if cErr != nil {
		return cErr
	}
	shared.LogLevel("info", "created\nserver ips: %s\nserver ids:%s\n", newServerIps, serverIds)

	// scp all new nodes
	if scpErr := scpToNewNodes(product, master, newServerIps); scpErr != nil {
		return scpErr
	}

	// var newServerLeaderIp string
	// newServerLeaderIp = serverIps[0]

	// delete all nodes except the first one so cluster.ServerIPs[0] is not getting deleted for now
	oldServersIps := cluster.ServerIPs
	for i, ip := range newServerIps {
		if len(oldServersIps) > 0 {
			if delErr := dependencies.DeleteInstance(oldServersIps[i+1]); delErr != nil {
				return delErr
			}
		}

		joinCmd, installErr := parseJoinCmd(product, server, serverLeaderIp, token, version, ip)
		if installErr != nil {
			return installErr
		}
		joinErr := joinNode(server, joinCmd, ip)
		if joinErr != nil {
			return joinErr
		}
	}

	//
	// worker/agent part
	workersLenght := len(cluster.AgentIPs)
	if workersLenght > 0 {
		var workerNames []string
		var workerIds, workerIps []string
		for i := 0; i < workersLenght; i++ {
			workerNames = append(workerNames, fmt.Sprintf("%s-agent-%d", os.Getenv("resource_name"), i))
		}

		workerIps, workerIds, err = dependencies.CreateInstances(workerNames...)
		if err != nil {
			return err
		}
		shared.LogLevel("info", "created\nworker ips: %s\nworker ids:%s\n", workerIps, workerIds)

		for i := workersLenght - 1; i >= 0; i++ {
			currentAgent := cluster.AgentIPs[i]
			err = dependencies.DeleteInstance(currentAgent)
			if err != nil {
				return err
			}
		}

		var cmd string
		var installErr error
		for _, ip := range workerIps {
			if scpErr := scpToNewNodes(ip, product, workerIps); scpErr != nil {
				return scpErr
			}

			cmd, installErr = parseJoinCmd(product, agent, serverLeaderIp, token, version, ip)
			if installErr != nil {
				return installErr
			}
			err = joinNode(agent, cmd, serverLeaderIp)
			if err != nil {
				return err
			}
		}
	}

	// delete serverLeaderIp
	err = dependencies.DeleteInstance(serverLeaderIp)
	if err != nil {
		return err
	}

	// get first server node name and patch it
	_, err = shared.RunCommandOnNode("kubectl patch node *****{nodename} -p '{\"metadata\":{\"finalizers\":[]}}' --type=merge", newServerIps[0])
	if err != nil {
		return err
	}

	return nil
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

func parseJoinCmd(product, nodetype, serverIp, token, version, selfIp string) (string, error) {
	if product != "rke2" && product != "k3s" {
		return "", shared.ReturnLogError("unsupported product: %s\n", product)
	}

	// create a separated func fir get env and returns vars
	if err := config.SetEnv(shared.BasePath() + fmt.Sprintf("/config/%s.tfvars", product)); err != nil {
		return "", shared.ReturnLogError("error loading tf vars: %w\n", err)
	}

	// nodeOs := os.Getenv("node_os")
	// serverFlags := os.Getenv("server_flags")
	// workerFlags := os.Getenv("worker_flags")
	// dataStoreType := os.Getenv("datastore_type")
	// installMode := os.Getenv("install_mode")
	// username := os.Getenv("username")
	// password := os.Getenv("password")
	// channel := os.Getenv(fmt.Sprintf("%s_channel", product))

	var flags string
	if nodetype == server {
		flags = fmt.Sprintf("'%s'", os.Getenv("server_flags"))
		nodetype = "master"
	} else {
		flags = fmt.Sprintf("'%s'", os.Getenv("worker_flags"))
		nodetype = "agent"
	}

	templateTest := " "
	// ipv6Ip := " "
	cmd := fmt.Sprintf(
		"sudo -i /tmp/join_%s_%s.sh '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' %s '%s' '%s' '%s'",
		product,
		nodetype,
		os.Getenv("node_os"),
		serverIp,
		os.Getenv("install_mode"),
		version,
		os.Getenv("datastore_type"),
		selfIp,
		serverIp,
		token,
		templateTest,
		flags,
		os.Getenv("username"),
		os.Getenv("password"),
		os.Getenv(fmt.Sprintf("%s_channel", product)),
	)

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
