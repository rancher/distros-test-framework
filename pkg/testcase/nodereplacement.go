package testcase

import (
	"fmt"
	"os"

	"github.com/rancher/distros-test-framework/config"
	"github.com/rancher/distros-test-framework/shared"
)

type Awsec2 interface {
	CreateInstances(names ...string) (ids, ips []string, err error)
	DeleteInstance(ip string) error
	WaitForInstanceRunning(instanceId string) error
}

func TestReplaceNode(version string, e Awsec2) error {
	// product, err := shared.GetProduct()

	// if err != nil {
	// 	return err
	// }
	// product, err := shared.GetProduct()
	// if err != nil {
	// 	return shared.ReturnLogError("error getting product: %w\n", err)
	// }
	//
	// varDir, err := filepath.Abs(shared.BasePath() +
	// 	fmt.Sprintf("/distros-test-framework/config/%s.tfvars", product))
	// if err != nil {
	// 	CreateInstances
	// 	return shared.ReturnLogError("invalid product: %s\n", product)
	// }
	//
	// resourceName := tf.GetVariableAsStringFromVarFile(GinkgoT(), varDir, "resource_name")

	// TODO: DELETE first server , create a node and join
	// TODO: Delete next servers if exists and create new servers and join
	// TODO: DELETE agent if exists and create new agent and join

	// TODO: 4 servers 2 agents
	// TODO: need to have 1 server to get the token and ip to join the new server

	// get 1 server to be a leader
	// get token from leader
	// get ip from leader

	// serverIps := factory.AddCluster(GinkgoT()).ServerIPs[0:]
	// err := deleteNode("server")
	// if err != nil {
	// 	return err
	// }
	//
	// for _, ip := range ips[0:] {
	// 	err := deleteNode("server")
	// 	if err != nil {
	// 		return err
	// 	}
	// 	joinNode("server", ip, t)
	// }
	//
	// for _, ip := range factory.AddCluster(GinkgoT()).ServerIPs {
	// 	if err := e.DeleteInstance(ip); err != nil {
	// 		return err
	// 	}
	//
	// 	name := fmt.Sprintf("%s-server", resourceName)
	//
	// 	joinNode("server", version, name, product, e)
	// }
	//
	// if len(factory.AddCluster(GinkgoT()).AgentIPs) > 0 {
	// 	for _, ip := range factory.AddCluster(GinkgoT()).AgentIPs {
	// 		err := deleteNode("agent", a)
	// 		if err != nil {
	// 			return err
	// 		}
	// 		name := fmt.Sprintf("%s-agent", resourceName)
	// 		joinNode("agent", version, name, product, e)
	// 	}
	// }

	return nil
}

func serverData() (string, string, error) {
	leader := shared.FetchNodeExternalIP()
	token, err := shared.RunCommandOnNode("sudo -i cat /tmp/nodetoken", leader[0])
	if err != nil {
		return "", "", err
	}

	return leader[0], token, nil
}

//	func deleteNode(ip string, a ec2) error {
//		if err := a.DeleteInstance(ip); err != nil {
//			return err
//		}
//		cc := factory.AddCluster(GinkgoT())
//		serverIpsLen := len(cc.ServerIPs)
//		agentIpsLen := len(cc.AgentIPs)
//
//		switch nodetype {
//		case "server":
//			ip := cc.ServerIPs[serverIpsLen-1]
//			_ = a.DeleteInstance(ip)
//			cc.ServerIPs = cc.ServerIPs[:len(cc.ServerIPs)-1]
//			fmt.Println(cc.ServerIPs)
//		case "agent":
//			ip := cc.AgentIPs[agentIpsLen-1]
//			_ = a.DeleteInstance(ip)
//			cc.AgentIPs = cc.AgentIPs[:len(cc.AgentIPs)-1]
//			fmt.Println(cc.AgentIPs)
//		}
//
//		errDel := shared.DeleteInstance(cluster.ServerIPs[0])
//		if errDel != nil {
//			return errDel
//		}
//
//		return nil
//	}
// func joinNode(nodetype, version, name, product string, a Awsec2) error {
// 	_, selfIps, err := a.CreateInstances(name)
// 	if err != nil {
// 		return err
// 	}
//
// 	if (nodetype != "server" && nodetype != "agent") || nodetype == "" {
// 		return shared.ReturnLogError("invalid node type: %s\n", nodetype)
// 	}
//
// 	serverIp, token, err := serverData()
// 	if err != nil {
// 		return err
// 	}
//
// 	cmd, err := AddCmd(product, nodetype, serverIp, token, version, selfIps[0])
//
// 	// err = joinServer(cmd, ips[0])
// 	// if err != nil {
// 	// 	return err
// 	// } else {
// 	// 	err = joinAgent(cmd, ips[0])
// 	// 	if err != nil {
// 	// 		return err
// 	// 	}
// 	// }
//
// 	return nil
// }

func joinServer(cmd, ip string) error {
	_, _ = shared.RunCommandOnNode(cmd, ip)

	return nil
}

func joinAgent(cmd, ip string) error {
	_, _ = shared.RunCommandOnNode(cmd, ip)

	return nil
}

// vars needed nodeos,initial_node_ip,token,version,channnel

func AddCmd(product, nodetype, serverIp, token, version, selfIp string) (string, error) {
	if product != "rke2" && product != "k3s" {
		return "", shared.ReturnLogError("unsupported product: %s\n", product)
	}

	if err := config.SetEnv(shared.BasePath() + fmt.Sprintf("/config/%s.tfvars", product)); err != nil {
		return "", shared.ReturnLogError("error loading tf vars: %w\n", err)
	}
	nodeOs, _ := os.LookupEnv("node_os")
	accessKey, _ := os.LookupEnv("access_key")
	awsUser, _ := os.LookupEnv("aws_user")
	serverFlags := os.Getenv("server_flags")
	workerFlags := os.Getenv("worker_flags")
	dataStoreType := os.Getenv("datastore_type")
	installMode := os.Getenv("install_mode")
	username := os.Getenv("username")
	password := os.Getenv("password")
	channel := os.Getenv(fmt.Sprintf("%s_channel", product))

	var flags string
	if nodetype == "server" {
		flags = serverFlags
	} else {
		flags = workerFlags
	}

	script := fmt.Sprintf(shared.BasePath()+"/modules/install/join_%s_%s.sh", product, nodetype)
	remote := fmt.Sprintf("/tmp/join_%s_%s.sh", product, nodetype)

	scp := fmt.Sprintf(
		"scp -r -i %s -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null "+script+" %s@"+selfIp+":%s",
		accessKey,
		awsUser,
		remote,
	)
	ssh := fmt.Sprintf("ssh -i %s -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null %s@"+selfIp+" 'bash %s %s %s %s %s %s %s %s %s %s %s'",
		accessKey,
		awsUser,
		remote,
		nodeOs,
		serverIp,
		installMode,
		version,
		dataStoreType,
		selfIp,
		token,
		flags,
		username,
		password,
		channel,
	)

	cmd := scp + " && " + ssh

	return cmd, nil
}

// func joinInstance() {
// 	s := session.Must(session.NewSession())
// 	svc := ec2.New(s, aws.NewConfig().WithRegion("us-west-2"))
// }

// delete 1 server +  create 1 server and join

// delete agent if exists and create new agent and join

// check cluster is ok
