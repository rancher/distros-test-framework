package support

import (
	"fmt"

	"github.com/rancher/distros-test-framework/internal/resources"

	"github.com/rancher/distros-test-framework/internal/provisioning/driver"
)

// LogAgentNodeIPs Prints out the Agent node IPs.
func LogAgentNodeIPs(agentNum int, agentIPs []string, isWindows bool) {
	info := "Agent Node IPs:"
	if isWindows {
		info = "Windows " + info
	}
	if agentNum > 0 {
		resources.LogLevel("info", info+"  %v", agentIPs)
	}
}

func GetNodesViaBastion(cluster *driver.Cluster) (nodeDetails string, err error) {
	cmd := fmt.Sprintf(
		"KUBECONFIG=/tmp/%v_kubeconf.yaml ",
		cluster.Config.Product)
	cmd += "kubectl get nodes -o wide --no-headers"
	nodeDetails, err = resources.RunCommandOnNode(cmd, cluster.Bastion.PublicIPv4Addr)

	return nodeDetails, err
}

func GetPodsViaBastion(cluster *driver.Cluster) (podDetails string) {
	cmd := fmt.Sprintf(
		"KUBECONFIG=/tmp/%v_kubeconf.yaml ",
		cluster.Config.Product)
	cmd += "kubectl get pods -A -o wide --no-headers"
	podDetails, _ = resources.RunCommandOnNode(cmd, cluster.Bastion.PublicIPv4Addr)

	return podDetails
}
