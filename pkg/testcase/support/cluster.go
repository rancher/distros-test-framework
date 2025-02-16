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

func GetPrivateNodes(cluster *shared.Cluster) (nodeDetails string, err error) {
	cmd := fmt.Sprintf(
		"PATH=$PATH:/var/lib/rancher/%[1]v/bin:/opt/%[1]v/bin; "+
			"KUBECONFIG=/etc/rancher/%[1]v/%[1]v.yaml ",
		cluster.Config.Product)
	cmd += "kubectl get nodes -o wide --no-headers"
	nodeDetails, err = CmdForPrivateNode(cluster, cmd, cluster.ServerIPs[0])

	return nodeDetails, err
}

func GetPrivatePods(cluster *shared.Cluster) (podDetails string) {
	cmd := fmt.Sprintf(
		"PATH=$PATH:/var/lib/rancher/%[1]v/bin:/opt/%[1]v/bin; "+
			"KUBECONFIG=/etc/rancher/%[1]v/%[1]v.yaml ", cluster.Config.Product)
	cmd += "kubectl get pods -A -o wide --no-headers"
	podDetails, _ = CmdForPrivateNode(cluster, cmd, cluster.ServerIPs[0])

	return podDetails
}
