package testcase

import (
	"fmt"
	"strings"

	"github.com/rancher/distros-test-framework/internal/provisioning/driver"
	"github.com/rancher/distros-test-framework/internal/resources"

	. "github.com/onsi/gomega"
)

func TestCertRotate(cluster *driver.Cluster) {
	ms := resources.NewManageService(5, 5)
	certRotate(ms, cluster.Config.Product, cluster.ServerIPs)

	actions := []resources.ServiceAction{
		{
			Service:  cluster.Config.Product,
			Action:   "restart",
			NodeType: "agent",
		},
		{
			Service:  cluster.Config.Product,
			Action:   "status",
			NodeType: "agent",
		},
	}
	for _, agentIP := range cluster.AgentIPs {
		output, err := ms.ManageService(agentIP, actions)
		if output != "" {
			Expect(output).To(ContainSubstring("active "), fmt.Sprintf("error restarting %s agent service for node ip: %s",
				cluster.Config.Product, agentIP))
		}
		Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("error restarting %s service on %s", cluster.Config.Product, agentIP))
	}

	verifyTLSDirContent(cluster.Config.Product, cluster.ServerIPs)
}

// certRotate Rotate certificate for etcd only and cp only nodes.
func certRotate(ms *resources.ManageService, product string, ips []string) {
	for _, ip := range ips {
		actions := []resources.ServiceAction{
			{
				Service:  product,
				Action:   "stop",
				NodeType: "server",
			},
			{
				Service: product,
				Action:  "rotate",
			},
			{
				Service:  product,
				Action:   "start",
				NodeType: "server",
			},
			{
				Service:  product,
				Action:   "status",
				NodeType: "server",
			},
		}

		output, err := ms.ManageService(ip, actions)
		if output != "" {
			Expect(output).To(ContainSubstring("active "),
				fmt.Sprintf("error restarting %s service for node ip: %s", product, ip))
		}
		Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("error rotating certificate for %s service on %s", product, ip))
	}
}

// verifyIdenticalFiles Verify the actual and expected identical file lists match.
func verifyIdenticalFiles(identicalFileList string) {
	expectedFileList := []string{
		"client-ca.crt", "client-ca.key", "client-ca.nochain.crt",
		"peer-ca.crt", "peer-ca.key",
		"server-ca.crt", "server-ca.key",
		"request-header-ca.crt", "request-header-ca.key",
		"server-ca.crt", "server-ca.key", "server-ca.nochain.crt",
		"service.current.key", "service.key",
	}

	newFileList := strings.Split(strings.TrimSpace(identicalFileList), "\n")
	err := resources.MatchWithPath(newFileList[1:], expectedFileList)
	Expect(err).NotTo(HaveOccurred(), "FAIL: Verifying identical file list match")
}

// verifyTLSDirContent Compare TLS Directories before and after cert rotation to display identical files.
func verifyTLSDirContent(product string, ips []string) {
	dataDir := "/var/lib/rancher/" + product
	serverDir := dataDir + "/server"
	origTLSDir := serverDir + "/tls"

	cmd := "sudo ls -lt " + serverDir + "/ | grep tls | awk {'print $9'} | sed -n '2 p'"

	for _, ip := range ips {
		resources.LogLevel("info", "Working on node with ip: %v", ip)

		tlsDir, tlsError := resources.RunCommandOnNode(cmd, ip)
		Expect(tlsError).NotTo(HaveOccurred(), "Unable to get new TLS Directory name for "+ip)

		resources.LogLevel("info", "TLS Directory name:  %v", tlsDir)
		newTLSDir := fmt.Sprintf("%s/%s", serverDir, tlsDir)

		resources.LogLevel("info", "Comparing Directories: %s and %s", origTLSDir, newTLSDir)
		cmd2 := fmt.Sprintf("sudo diff -sr %s/ %s/ | grep -i identical | "+
			"awk '{print $2}' | xargs basename -a | "+
			"awk 'BEGIN{print \"Identical Files:  \"}; {print $1}'", origTLSDir, newTLSDir)

		identicalFileList, err := resources.RunCommandOnNode(cmd2, ip)
		Expect(err).NotTo(HaveOccurred(), "error getting identical files on "+ip)

		resources.LogLevel("debug", "Identical Files Output for %s: %s", ip, identicalFileList)

		verifyIdenticalFiles(identicalFileList)
	}
}
