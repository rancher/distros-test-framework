package testcase

import (
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/gomega"
)

func TestRestartService(cluster *shared.Cluster) {
	ms := shared.NewManageService(1, 1)
	serverAction := shared.ServiceAction{
		Service:       cluster.Config.Product,
		Action:        "restart",
		NodeType:      "server",
	}
	for _, ip := range cluster.ServerIPs {
		_, err := ms.ManageService(ip, []shared.ServiceAction{serverAction})
		Expect(err).NotTo(HaveOccurred(), "error restarting %s server service on %s", cluster.Config.Product, ip)
	}

	if cluster.NumAgents > 0 {
		agentAction := shared.ServiceAction{
			Service:       cluster.Config.Product,
			Action:        "restart",
			NodeType:      "agent",
		}
		for _, ip := range cluster.AgentIPs {
			_, err := ms.ManageService(ip, []shared.ServiceAction{agentAction})
			Expect(err).NotTo(HaveOccurred(), "error restarting %s agent service on %s", cluster.Config.Product, ip)
		}
	}
}
