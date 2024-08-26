package testcase

import (
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/gomega"
)

func TestRestartService(cluster *shared.Cluster) {
	ip, err := shared.ManageService(cluster.Config.Product, "restart", "server", cluster.ServerIPs)
	Expect(err).NotTo(HaveOccurred(), "failed restart server for ip %s", ip)

	if cluster.NumAgents > 0 {
		ip, err = shared.ManageService(cluster.Config.Product, "restart", "agent", cluster.AgentIPs)
		Expect(err).NotTo(HaveOccurred(), "failed to restart agent for ip %s", ip)
	}
}
