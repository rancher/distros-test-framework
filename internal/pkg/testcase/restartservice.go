package testcase

import (
	"github.com/rancher/distros-test-framework/internal/provisioning/driver"
	"github.com/rancher/distros-test-framework/internal/resources"

	. "github.com/onsi/gomega"
)

func TestRestartService(cluster *driver.Cluster) {
	ms := resources.NewManageService(1, 1)
	serverAction := resources.ServiceAction{
		Service:  cluster.Config.Product,
		Action:   "restart",
		NodeType: "server",
	}
	for _, ip := range cluster.ServerIPs {
		_, err := ms.ManageService(ip, []resources.ServiceAction{serverAction})
		Expect(err).NotTo(HaveOccurred(), "error restarting %s server service on %s", cluster.Config.Product, ip)
	}

	if cluster.NumAgents > 0 {
		agentAction := resources.ServiceAction{
			Service:  cluster.Config.Product,
			Action:   "restart",
			NodeType: "agent",
		}
		for _, ip := range cluster.AgentIPs {
			_, err := ms.ManageService(ip, []resources.ServiceAction{agentAction})
			Expect(err).NotTo(HaveOccurred(), "error restarting %s agent service on %s", cluster.Config.Product, ip)
		}
	}
}
