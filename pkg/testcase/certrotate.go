package testcase

import (
	"fmt"

	"github.com/rancher/distros-test-framework/factory"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestCertRotate() {
	// Please refer to docs/testing.md for instructions on setup for this test

	cluster := factory.AddCluster(GinkgoT())
	serverIPs := cluster.ServerIPs
	agentIPs := cluster.AgentIPs
	p, _ := shared.GetProduct()
	var product shared.Product
	if p == "k3s" {
		product = shared.K3S
	} else {
		product = shared.RKE2
	}

	certRotate(product, serverIPs)

	product.ManageClusterService(shared.Restart, shared.Agent, agentIPs)

	compareAndVerifyTLSDirContent(product, serverIPs)

}

// certRotate Rotate certificate for etcd only and cp only nodes
func certRotate(product shared.Product, ips []string) {

	stopError := product.ManageClusterService(shared.Stop, shared.Server, ips)
	if stopError != nil {
		shared.ReturnLogError(fmt.Sprintf("Error stopping %s service", product))
	}

	ip, rotateError := product.CertRotate(ips)
	if rotateError != nil {
		shared.ReturnLogError(fmt.Sprintf("Error running certificate rotate for %s service on %s", product, ip))
	}

	startError := product.ManageClusterService(shared.Start, shared.Server, ips)
	if startError != nil {
		shared.ReturnLogError(fmt.Sprintf("Error starting %s service", product))
	}
	// Preventative restart - sometimes, start hangs after a cert rotate which gets fixed on a second restart
	restartError := product.ManageClusterService(shared.Restart, shared.Server, ips)
	if restartError != nil {
		shared.ReturnLogError(fmt.Sprintf("Error restarting %s service", product))
	}
}

// getExpectedFileNames Used for verification of file list
func getExpectedFileNames() []string {
	return []string{"client-ca.crt", "client-ca.key", "client-ca.nochain.crt", "client-supervisor.crt",
		"client-supervisor.key", "peer-ca.crt", "peer-ca.key", "server-ca.crt", "server-ca.key", "request-header-ca.crt",
		"request-header-ca.key", "server-ca.crt", "server-ca.key", "server-ca.nochain.crt", "service.current.key", "service.key"}
}

// verifyIdenticalFiles Verify the actual and expected identical file lists match
func verifyIdenticalFiles(identicalFileList string) {
	expectedFileList := getExpectedFileNames()
	for i := 0; i < len(expectedFileList); i++ {
		Expect(identicalFileList).To(ContainSubstring(expectedFileList[i]))
		shared.LogLevel("info", fmt.Sprintf("PASS: Looking for: %s", expectedFileList[i]))
	}
}

// compareAndVerifyTLSDirContent Compare TLS Directories before and after cert rotation to display identical files
func compareAndVerifyTLSDirContent(product shared.Product, ips []string) {

	dataDir := fmt.Sprintf("/var/lib/rancher/%s", product)
	serverDir := fmt.Sprintf("%s/server", dataDir)
	origTLSDir := fmt.Sprintf("%s/tls", serverDir)

	cmd := fmt.Sprintf("sudo ls -lt %s/ | grep tls | awk {'print $9'} | sed -n '2 p'", serverDir)

	for _, ip := range ips {

		shared.LogLevel("info", fmt.Sprintf("Working on node with ip: %s", ip))

		// Get new tls directory name
		tlsDir, tlsError := shared.RunCommandOnNode(cmd, ip)
		if tlsError != nil {
			shared.ReturnLogError(fmt.Sprintf("Unable to get new TLS Directory name for %s", ip))
		}
		shared.LogLevel("info", fmt.Sprintf("TLS Directory name: %s", tlsDir))
		newTLSDir := fmt.Sprintf("%s/%s", serverDir, tlsDir)

		// Compare original and new tls directories
		shared.LogLevel("info", "Comparing Directories: %s and %s", origTLSDir, newTLSDir)
		cmd2 := fmt.Sprintf("sudo diff -sr %s/ %s/ | grep -i identical | "+
			"awk '{print $2}' | xargs basename -a | "+
			"awk 'BEGIN{print \"Identical Files:  \"}; {print $1}'", origTLSDir, newTLSDir)
		idFiles, err := shared.RunCommandOnNode(cmd2, ip)
		if err != nil {
			shared.LogLevel("error", err.Error())
			Expect(err).NotTo(HaveOccurred())
		}
		shared.LogLevel("debug", fmt.Sprintf("Identical Files Output for %s: %s", ip, idFiles))
		verifyIdenticalFiles(idFiles)
	}
}
