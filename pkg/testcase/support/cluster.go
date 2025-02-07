package support

import "github.com/rancher/distros-test-framework/shared"

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
