package testcase

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rancher/distros-test-framework/factory"
	"github.com/rancher/distros-test-framework/shared"
)

// Rotate certificate for etcd only and cp only nodes
func certRotate(product string, ip string) {
	// Stop service on server1
	shared.StopService(product, ip, "server")
	// Rotate certificate
	shared.CertRotate(product, ip)
	// start service on server1
	shared.StartService(product, ip, "server")
}

// type Cluster struct {
// 	Status       string
// 	ServerIPs    []string
// 	AgentIPs     []string
// 	WinAgentIPs  []string
// 	NumWinAgents int
// 	NumServers   int
// 	NumAgents    int
// 	Config       clusterConfig
// }

func TestCertRotate() {
	// Node Types needed for this test:
	// Server1 - Etcd only node
	// Server2 - Control Plane only node
	// Agent1 - Worker node
	// Initialize the cluster
	cluster := factory.AddCluster(GinkgoT())
	// Get server1, server2, product info from cluster
	server1 := cluster.ServerIPs[0]
	server2 := cluster.ServerIPs[1]
	agent1 := cluster.AgentIPs[0]
	product := cluster.Config.Product
	// Stop / rotate / start for server1
	certRotate(product, server1)
	// Stop / rotate / start for server2
	certRotate(product, server2)
	// stop / start service for agent1
	shared.StopStartService(product, agent1, "agent")
	// compare and display identical files for server1
	idFiles, error := shared.CompareTLSDir(product, server1)
	if error != nil {
		shared.LogLevel("error", error.Error())
		Expect(error).NotTo(HaveOccurred())
	}
	shared.LogLevel("info", fmt.Sprintf("Etcd Only Server: %s \nIdentical Files Output: %s", server1, idFiles))
	// compare and display identical files for server2
	idFiles2, error2 := shared.CompareTLSDir(product, server2)
	if error2 != nil {
		shared.LogLevel("error", error2.Error())
		Expect(error2).NotTo(HaveOccurred())
	}
	shared.LogLevel("info", fmt.Sprintf("Control Plane Only Server Node: %s \nIdentical Files Output: %s", server2, idFiles2))

	fileNames := []string{"client-ca.crt",
		"client-ca.key",
		"client-ca.nochain.crt",
		"client-supervisor.crt",
		"client-supervisor.key",
		"peer-ca.crt",
		"peer-ca.key",
		"server-ca.crt",
		"server-ca.key",
		"request-header-ca.crt",
		"request-header-ca.key",
		"server-ca.crt",
		"server-ca.key",
		"server-ca.nochain.crt",
		"service.current.key",
		"service.key"}
	for i := 0; i < len(fileNames); i++ {

		Expect(idFiles).To(ContainSubstring(fileNames[i]))
		Expect(idFiles2).To(ContainSubstring(fileNames[i]))
		shared.LogLevel("info", fmt.Sprintf("SUCCESS: Looking for: %s", fileNames[i]))
	}

}
