package testcase

//
// import (
// 	"fmt"
// 	"os"
//
// 	"github.com/rancher/distros-test-framework/config"
// 	"github.com/rancher/distros-test-framework/factory"
// 	"github.com/rancher/distros-test-framework/pkg/aws"
// 	"github.com/rancher/distros-test-framework/shared"
//
// 	. "github.com/onsi/ginkgo/v2"
// )
//
// func TestUpgradeReplaceNode(version string) error {
// 	if version == "" {
// 		return shared.ReturnLogError("version is needed.\n")
// 	}
// 	shared.PrintClusterState()
//
// 	dependencies, err := aws.AddAwsNode()
// 	if err != nil {
// 		return err
// 	}
//
// 	cluster := factory.ClusterConfig(GinkgoT())
// 	product, err := shared.GetProduct()
// 	if err != nil {
// 		return shared.ReturnLogError("error getting product: %w\n", err)
// 	}
//
// 	serversLenght := len(cluster.ServerIPs)
// 	var serverNames []string
// 	var serverIds, serverIps []string
//
// 	for i := 0; i < serversLenght; i++ {
// 		serverNames = append(serverNames, fmt.Sprintf("%s-server-%d", os.Getenv("resource_name"), i))
// 	}
//
// 	serverIds, serverIps, err = dependencies.CreateInstances(serverNames...)
// 	if err != nil {
// 		return err
// 	}
// 	fmt.Println(serverIds, serverIps)
//
// 	for i := serversLenght - 1; i >= 0; i-- {
// 		err = dependencies.DeleteInstance(cluster.ServerIPs[i])
// 		if err != nil {
// 			return err
// 		}
// 		for _, ip := range serverIps {
// 			err = joinNode("server", version, product, ip)
// 			if err != nil {
// 				return err
// 			}
// 		}
// 	}
//
// 	workersLenght := len(cluster.AgentIPs)
// 	if workersLenght > 0 {
// 		var workerNames []string
// 		var workerIds, workerIps []string
//
// 		for i := 0; i < workersLenght; i++ {
// 			workerNames = append(workerNames, fmt.Sprintf("%s-agent-%d", os.Getenv("resource_name"), i))
// 		}
//
// 		workerIds, workerIps, err = dependencies.CreateInstances(workerNames...)
// 		if err != nil {
// 			return err
// 		}
// 		fmt.Println(workerIds, workerIps)
//
// 		for i := workersLenght - 1; i >= 0; i-- {
// 			err = dependencies.DeleteInstance(cluster.AgentIPs[i])
// 			if err != nil {
// 				return err
// 			}
// 			for _, ip := range workerIps {
// 				err = joinNode("agent", version, product, ip)
// 				if err != nil {
// 					return err
// 				}
// 			}
// 		}
// 	}
//
// 	return nil
// }
//
// func serverData() (string, string, error) {
// 	leader := shared.FetchNodeExternalIP()
// 	token, err := shared.RunCommandOnNode("sudo -i cat /tmp/nodetoken", leader[0])
// 	if err != nil {
// 		return "", "", err
// 	}
//
// 	return leader[0], token, nil
// }
//
// func joinNode(nodetype, version, product, selfIp string) error {
// 	serverIp, token, err := serverData()
// 	if err != nil {
// 		return err
// 	}
//
// 	cmd, err := addCmd(product, nodetype, serverIp, token, version, selfIp)
//
// 	switch nodetype {
// 	case "server":
// 		err = joinServer(cmd, selfIp)
// 		if err != nil {
// 			return err
// 		}
// 	case "agent":
// 		err = joinAgent(cmd, selfIp)
// 		if err != nil {
// 			return err
// 		}
// 	case "":
// 		return shared.ReturnLogError("invalid node type: %s\n", nodetype)
// 	default:
// 		return shared.ReturnLogError("invalid node type: %s\n", nodetype)
// 	}
//
// 	return nil
// }
//
// func joinServer(cmd, ip string) error {
// 	_, err := shared.RunCommandOnNode(cmd, ip)
// 	if err != nil {
// 		return err
// 	}
//
// 	return nil
// }
//
// func joinAgent(cmd, ip string) error {
// 	_, err := shared.RunCommandOnNode(cmd, ip)
// 	if err != nil {
// 		return err
// 	}
//
// 	return nil
// }
//
// func addCmd(product, nodetype, serverIp, token, version, selfIp string) (string, error) {
// 	if product != "rke2" && product != "k3s" {
// 		return "", shared.ReturnLogError("unsupported product: %s\n", product)
// 	}
//
// 	if err := config.SetEnv(shared.BasePath() + fmt.Sprintf("/config/%s.tfvars", product)); err != nil {
// 		return "", shared.ReturnLogError("error loading tf vars: %w\n", err)
// 	}
//
// 	nodeOs := os.Getenv("node_os")
// 	accessKey := os.Getenv("access_key")
// 	awsUser := os.Getenv("aws_user")
// 	serverFlags := os.Getenv("server_flags")
// 	workerFlags := os.Getenv("worker_flags")
// 	dataStoreType := os.Getenv("datastore_type")
// 	installMode := os.Getenv("install_mode")
// 	username := os.Getenv("username")
// 	password := os.Getenv("password")
// 	channel := os.Getenv(fmt.Sprintf("%s_channel", product))
//
// 	var flags string
// 	if nodetype == "server" {
// 		flags = serverFlags
// 	} else {
// 		flags = workerFlags
// 	}
//
// 	script := fmt.Sprintf(shared.BasePath()+"/modules/install/join_%s_%s.sh", product, nodetype)
// 	remote := fmt.Sprintf("/tmp/join_%s_%s.sh", product, nodetype)
//
// 	scp := fmt.Sprintf(
// 		"scp -r -i %s -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null "+script+" %s@"+selfIp+":%s",
// 		accessKey,
// 		awsUser,
// 		remote,
// 	)
// 	ssh := fmt.Sprintf("ssh -i %s -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null %s@"+selfIp+" 'bash %s %s %s %s %s %s %s %s %s %s %s'",
// 		accessKey,
// 		awsUser,
// 		remote,
// 		nodeOs,
// 		serverIp,
// 		installMode,
// 		version,
// 		dataStoreType,
// 		selfIp,
// 		token,
// 		flags,
// 		username,
// 		password,
// 		channel,
// 	)
//
// 	cmd := scp + " && " + ssh
//
// 	return cmd, nil
// }
