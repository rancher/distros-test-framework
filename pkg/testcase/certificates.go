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
	// Stop service on server
	shared.StopService(product, ip, "server")
	// Rotate certificate
	shared.CertRotate(product, ip)
	// start service on server
	shared.StartService(product, ip, "server")
}

// Compare TLS Directories before and after cert rotation to display identical files
func compareTLSDir(product string, ip string) (string, error) {
	dataDir := fmt.Sprintf("/var/lib/rancher/%s", product)
	serverDir := fmt.Sprintf("%s/server", dataDir)
	origTLSDir := fmt.Sprintf("%s/tls", serverDir)
	cmd := fmt.Sprintf("sudo ls -lt %s/ | grep tls | awk {'print $9'} | sed -n '2 p'", serverDir)
	tlsDir, error := shared.RunCommandOnNode(cmd, ip)
	if error != nil {
		shared.LogLevel("warn", "Unable to get new TLS Directory name")
	}
	shared.LogLevel("info", fmt.Sprintf("TLS Directory name: %s", tlsDir))
	newTLSDir := fmt.Sprintf("%s/%s", serverDir, tlsDir)
	shared.LogLevel("info", "Comparing Directories: %s and %s", origTLSDir, newTLSDir)
	cmd2 := fmt.Sprintf("sudo diff -sr %s/ %s/ | grep -i identical | awk '{print $2}' | xargs basename -a | awk 'BEGIN{print \"Identical Files:  \"}; {print $1}'", origTLSDir, newTLSDir)
	return shared.RunCommandOnNode(cmd2, ip)
}

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
	// Stop; cert rotate; start for server1
	certRotate(product, server1)
	// Stop; cert rotate; start for server2
	certRotate(product, server2)
	// stop; start service for agent1
	shared.StopStartService(product, agent1, "agent")
	// compare and display identical files for server1
	idFiles, error := compareTLSDir(product, server1)
	if error != nil {
		shared.LogLevel("error", error.Error())
		Expect(error).NotTo(HaveOccurred())
	}
	shared.LogLevel("debug", fmt.Sprintf("Etcd Only Server: %s \nIdentical Files Output: %s", server1, idFiles))
	// compare and display identical files for server2
	idFiles2, error2 := compareTLSDir(product, server2)
	if error2 != nil {
		shared.LogLevel("error", error2.Error())
		Expect(error2).NotTo(HaveOccurred())
	}
	shared.LogLevel("debug", fmt.Sprintf("Control Plane Only Server Node: %s \nIdentical Files Output: %s", server2, idFiles2))

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
		shared.LogLevel("info", fmt.Sprintf("PASS: Looking for: %s", fileNames[i]))
	}

}
