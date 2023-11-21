package testcase

import (
	"fmt"
	"strings"

	"github.com/rancher/distros-test-framework/factory"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestCertRotate() {
	cluster := factory.AddCluster(GinkgoT())
	serverIPs := cluster.ServerIPs
	agentIPs := cluster.AgentIPs
	product, err := shared.GetProductObject()
	Expect(err).NotTo(HaveOccurred(), "error getting product from config")

	certRotate(product, serverIPs)

	ip, manageError := product.ManageService("restart", "agent", agentIPs)
	Expect(manageError).NotTo(HaveOccurred(), fmt.Sprintf("error restarting agent node ip %s", ip))

	verifyTLSDirContent(product, serverIPs)

}

// certRotate Rotate certificate for etcd only and cp only nodes
func certRotate(product shared.Product, ips []string) {
	ip, stopError := product.ManageService("stop", "server", ips)
	Expect(stopError).NotTo(HaveOccurred(), fmt.Sprintf("error stopping %s service for node ip: %s", product, ip))

	ip, rotateError := product.CertRotate(ips)
	Expect(rotateError).NotTo(HaveOccurred(), fmt.Sprintf("error running certificate rotate for %s service on %s", product, ip))

	ip, restartError := product.ManageService("restart", "server", ips)
	Expect(restartError).NotTo(HaveOccurred(), fmt.Sprintf("error restarting %s service for node ip: %s", product, ip))
}

// verifyIdenticalFiles Verify the actual and expected identical file lists match
func verifyIdenticalFiles(identicalFileList string) {
	expectedFileList := []string{
		"client-ca.crt", "client-ca.key", "client-ca.nochain.crt",
		"client-supervisor.crt", "client-supervisor.key",
		"dynamic-cert.json",
		"peer-ca.crt", "peer-ca.key",
		"server-ca.crt", "server-ca.key",
		"request-header-ca.crt", "request-header-ca.key",
		"server-ca.crt", "server-ca.key", "server-ca.nochain.crt",
		"service.current.key", "service.key"}
	// shared.VerifyFileMatch(identicalFileList, expectedFileList)
	identicalFileList = strings.TrimSpace(identicalFileList)
	newSlice := strings.Split(identicalFileList, "\n")
	shared.VerifyFileMatchWithPath(newSlice[1:], expectedFileList, "")
}

// verifyTLSDirContent Compare TLS Directories before and after cert rotation to display identical files
func verifyTLSDirContent(product shared.Product, ips []string) {
	dataDir := fmt.Sprintf("/var/lib/rancher/%s", product)
	serverDir := fmt.Sprintf("%s/server", dataDir)
	origTLSDir := fmt.Sprintf("%s/tls", serverDir)

	cmd := fmt.Sprintf("sudo ls -lt %s/ | grep tls | awk {'print $9'} | sed -n '2 p'", serverDir)

	for _, ip := range ips {
		shared.LogLevel("info", fmt.Sprintf("Working on node with ip: %s", ip))

		tlsDir, tlsError := shared.RunCommandOnNode(cmd, ip)
		Expect(tlsError).NotTo(HaveOccurred(), fmt.Sprintf("Unable to get new TLS Directory name for %s", ip))

		shared.LogLevel("info", fmt.Sprintf("TLS Directory name: %s", tlsDir))
		newTLSDir := fmt.Sprintf("%s/%s", serverDir, tlsDir)

		shared.LogLevel("info", "Comparing Directories: %s and %s", origTLSDir, newTLSDir)
		cmd2 := fmt.Sprintf("sudo diff -sr %s/ %s/ | grep -i identical | "+
			"awk '{print $2}' | xargs basename -a | "+
			"awk 'BEGIN{print \"Identical Files:  \"}; {print $1}'", origTLSDir, newTLSDir)

		idFiles, err := shared.RunCommandOnNode(cmd2, ip)
		Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("error getting identical files on %s", ip))

		shared.LogLevel("debug", fmt.Sprintf("Identical Files Output for %s: %s", ip, idFiles))

		verifyIdenticalFiles(idFiles)
	}
}
