package support

import (
	"fmt"

	"github.com/rancher/distros-test-framework/shared"
)

// LogAgentNodeIPs Prints out the Agent node IPs.
func LogAgentNodeIPs(agentNum int, agentIPs []string, isWindows bool) {
	info := "Agent Node IPs:"
	if isWindows {
		info = "Windows " + info
	}
	if agentNum > 0 {
		shared.LogLevel("info", info+"  %v", agentIPs)
	}
}

func GetNodesViaBastion(cluster *shared.Cluster) (nodeDetails string, err error) {
	cmd := fmt.Sprintf(
		"KUBECONFIG=/tmp/%v_kubeconf.yaml ",
		cluster.Config.Product)
	cmd += "kubectl get nodes -o wide --no-headers"
	nodeDetails, err = shared.RunCommandOnNode(cmd, cluster.BastionConfig.PublicIPv4Addr)

	return nodeDetails, err
}

func GetPodsViaBastion(cluster *shared.Cluster) (podDetails string) {
	cmd := fmt.Sprintf(
		"KUBECONFIG=/tmp/%v_kubeconf.yaml ",
		cluster.Config.Product)
	cmd += "kubectl get pods -A -o wide --no-headers"
	podDetails, _ = shared.RunCommandOnNode(cmd, cluster.BastionConfig.PublicIPv4Addr)

	return podDetails
}
