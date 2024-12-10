package testcase

import (
	"fmt"
	"strings"

	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/gomega"
)

func TestCertRotate(cluster *shared.Cluster) {
	certRotate(cluster.Config.Product, cluster.ServerIPs)

	ip, manageError := shared.ManageService(cluster.Config.Product, "restart", "agent", cluster.AgentIPs)
	Expect(manageError).NotTo(HaveOccurred(), "error restarting agent node ip"+ip)

	verifyTLSDirContent(cluster.Config.Product, cluster.ServerIPs)
}

// certRotate Rotate certificate for etcd only and cp only nodes.
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

// verifyIdenticalFiles Verify the actual and expected identical file lists match.
func verifyIdenticalFiles(identicalFileList string) {
	expectedFileList := []string{
		"client-ca.crt", "client-ca.key", "client-ca.nochain.crt",
		"peer-ca.crt", "peer-ca.key",
		"server-ca.crt", "server-ca.key",
		"request-header-ca.crt", "request-header-ca.key",
		"server-ca.crt", "server-ca.key", "server-ca.nochain.crt",
		"service.current.key", "service.key"}

	newFileList := strings.Split(strings.TrimSpace(identicalFileList), "\n")
	err := shared.MatchWithPath(newFileList[1:], expectedFileList)
	Expect(err).NotTo(HaveOccurred(), "FAIL: Verifying identical file list match")
}

// verifyTLSDirContent Compare TLS Directories before and after cert rotation to display identical files.
func verifyTLSDirContent(product string, ips []string) {
	dataDir := "/var/lib/rancher/" + product
	serverDir := dataDir + "/server"
	origTLSDir := serverDir + "/tls"

	cmd := "sudo ls -lt " + serverDir + "/ | grep tls | awk {'print $9'} | sed -n '2 p'"

	for _, ip := range ips {
		shared.LogLevel("info", "Working on node with ip: %v", ip)

		tlsDir, tlsError := shared.RunCommandOnNode(cmd, ip)
		Expect(tlsError).NotTo(HaveOccurred(), "Unable to get new TLS Directory name for "+ip)

		shared.LogLevel("info", "TLS Directory name:  %v", tlsDir)
		newTLSDir := fmt.Sprintf("%s/%s", serverDir, tlsDir)

		shared.LogLevel("info", "Comparing Directories: %s and %s", origTLSDir, newTLSDir)
		cmd2 := fmt.Sprintf("sudo diff -sr %s/ %s/ | grep -i identical | "+
			"awk '{print $2}' | xargs basename -a | "+
			"awk 'BEGIN{print \"Identical Files:  \"}; {print $1}'", origTLSDir, newTLSDir)

		identicalFileList, err := shared.RunCommandOnNode(cmd2, ip)
		Expect(err).NotTo(HaveOccurred(), "error getting identical files on "+ip)

		shared.LogLevel("debug", "Identical Files Output for %s: %s", ip, identicalFileList)

		verifyIdenticalFiles(identicalFileList)
	}
}
