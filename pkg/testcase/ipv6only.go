package testcase

import (
	"fmt"
	"slices"
	"sync"

	. "github.com/onsi/gomega"
	"github.com/rancher/distros-test-framework/pkg/aws"
	"github.com/rancher/distros-test-framework/shared"
)

func TestBuildIPv6OnlyCluster(cluster *shared.Cluster) {
	shared.LogLevel("info", "Created nodes for %s cluster...", cluster.Config.Product)
	Expect(cluster.Status).To(Equal("cluster created"))
	Expect(cluster.ServerIPs).ShouldNot(BeEmpty())

	if cluster.BastionConfig.PublicIPv4Addr != "" {
		shared.LogLevel("info", "Bastion Public IP: %v", cluster.BastionConfig.PublicIPv4Addr)
		shared.LogLevel("info", "Bastion Public DNS: %v", cluster.BastionConfig.PublicDNS)
	}
	shared.LogLevel("info", "Server Node IPs: %v", cluster.ServerIPs)

	checkAndPrintAgentNodeIPs(cluster.NumAgents, cluster.AgentIPs, false)

	if cluster.Config.Product == "rke2" {
		checkAndPrintAgentNodeIPs(cluster.NumWinAgents, cluster.WinAgentIPs, true)
	}
}

func TestIPv6Only(cluster *shared.Cluster, awsClient *aws.Client) {
	shared.LogLevel("info", "Setting up %s cluster on ipv6 only nodes...", cluster.Config.Product)
	err := configureNodes(cluster, awsClient)
	Expect(err).NotTo(HaveOccurred(), err)
	// setup ipv6 on all nodes
	// install on servers
	// install on agent
}

func configureNodes(cluster *shared.Cluster, awsClient *aws.Client) (err error) {
	nodeIPs := cluster.ServerIPs
	nodeIPs = append(nodeIPs, cluster.AgentIPs...)
	errChan := make(chan error, len(nodeIPs))
	var wg sync.WaitGroup

	for _, nodeIP := range nodeIPs {
		wg.Add(1)
		go func(nodeIP string) {
			defer wg.Done()
			shared.LogLevel("info", "Copying scripts on node: %s", nodeIP)
			err = copyScripts(cluster, nodeIP)
			if err != nil {
				errChan <- shared.ReturnLogError("error copying scripts on node: %v\n, err: %w", nodeIP, err)
			}
			shared.LogLevel("info", "Processing configure.sh on node: %s", nodeIP)
			err = processConfigureFile(cluster, awsClient, nodeIP)
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

// copyScripts Copies configure.sh and install scripts on the nodes.
func copyScripts(cluster *shared.Cluster, ip string) (err error) {
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
	cmd += fmt.Sprintf(
		"sudo %v configure.sh %v %v@%v:~/",
		shared.ShCmdPrefix("scp", cluster.Aws.KeyName),
		installScript, cluster.Aws.AwsUser, shared.EncloseSqBraces(ip))
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
