package testcase

import (
	"fmt"
	"strings"

	"github.com/rancher/distros-test-framework/factory"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/gomega"
)

func TestCertRotate(cluster *factory.Cluster) {
	certRotate(cluster.Config.Product, cluster.ServerIPs)

	ip, manageError := shared.ManageService(cluster.Config.Product, "restart", "agent", cluster.AgentIPs)
	Expect(manageError).NotTo(HaveOccurred(), fmt.Sprintf("error restarting agent node ip %s", ip))

	verifyTLSDirContent(cluster.Config.Product, cluster.ServerIPs)
}

// certRotate Rotate certificate for etcd only and cp only nodes
func certRotate(product string, ips []string) {
	ip, stopError := shared.ManageService(product, "stop", "server", ips)
	Expect(stopError).NotTo(HaveOccurred(),
		fmt.Sprintf("error stopping %s service for node ip: %s", product, ip))

	ip, rotateError := shared.CertRotate(product, ips)
	Expect(rotateError).NotTo(HaveOccurred(),
		fmt.Sprintf("error running certificate rotate for %s service on %s", product, ip))

	ip, startError := shared.ManageService(product, "start", "server", ips)
	Expect(startError).NotTo(HaveOccurred(),
		fmt.Sprintf("error starting %s service for node ip: %s", product, ip))
}

// verifyIdenticalFiles Verify the actual and expected identical file lists match
func verifyIdenticalFiles(identicalFileList string) {
	expectedFileList := []string{
		"client-ca.crt", "client-ca.key", "client-ca.nochain.crt",
		"client-supervisor.crt", "client-supervisor.key",
		"peer-ca.crt", "peer-ca.key",
		"server-ca.crt", "server-ca.key",
		"request-header-ca.crt", "request-header-ca.key",
		"server-ca.crt", "server-ca.key", "server-ca.nochain.crt",
		"service.current.key", "service.key"}

	newFileList := strings.Split(strings.TrimSpace(identicalFileList), "\n")
	err := shared.MatchWithPath(newFileList[1:], expectedFileList)
	Expect(err).NotTo(HaveOccurred(), "FAIL: Verifying identical file list match")
}

// verifyTLSDirContent Compare TLS Directories before and after cert rotation to display identical files
func verifyTLSDirContent(product string, ips []string) {
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

		identicalFileList, err := shared.RunCommandOnNode(cmd2, ip)
		Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("error getting identical files on %s", ip))

		shared.LogLevel("debug", fmt.Sprintf("Identical Files Output for %s: %s", ip, identicalFileList))

		verifyIdenticalFiles(identicalFileList)
	}
}
