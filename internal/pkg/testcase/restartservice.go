package testcase

import (
	. "github.com/rancher/distros-test-framework/internal/pkg/service"
	"github.com/rancher/distros-test-framework/internal/resources"

	. "github.com/onsi/gomega"
)

func TestRestartService(cluster *resources.Cluster) {
	ms := NewManageService(1, 1)
	serverAction := ServiceAction{
		Service:  cluster.Config.Product,
		Action:   "restart",
		NodeType: "server",
	}
	for _, ip := range cluster.ServerIPs {
		_, err := ms.ManageService(ip, []ServiceAction{serverAction})
		Expect(err).NotTo(HaveOccurred(), "error restarting %s server service on %s", cluster.Config.Product, ip)
	}

	if cluster.NumAgents > 0 {
		agentAction := ServiceAction{
			Service:  cluster.Config.Product,
			Action:   "restart",
			NodeType: "agent",
		}
		for _, ip := range cluster.AgentIPs {
			_, err := ms.ManageService(ip, []ServiceAction{agentAction})
			Expect(err).NotTo(HaveOccurred(), "error restarting %s agent service on %s", cluster.Config.Product, ip)
		}
	}
}
