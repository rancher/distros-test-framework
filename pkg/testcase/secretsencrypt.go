package testcase

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/gomega"
)

func TestSecretsEncrypt() {
	etcdNodes, errGetEtcd := shared.GetNodesByRoles("etcd")
	Expect(etcdNodes).NotTo(BeEmpty())
	Expect(errGetEtcd).NotTo(HaveOccurred(), "error getting etcd nodes")

	cpNodes, errGetCP := shared.GetNodesByRoles("control-plane")
	Expect(cpNodes).NotTo(BeEmpty())
	Expect(errGetCP).NotTo(HaveOccurred(), "error getting control plane nodes")

	product, err := shared.Product()
	Expect(err).NotTo(HaveOccurred(), "error getting product from config")

	ips := getNodeIps(etcdNodes, cpNodes)

	errSecret := shared.CreateSecret("secret1", "default")
	Expect(errSecret).NotTo(HaveOccurred(), "error creating secret")
	shared.LogLevel("INFO", "TEST: 'CLASSIC' Secrets Encryption method")
	secretsEncryptOps("prepare", product, cpNodes[0].ExternalIP, ips)
	secretsEncryptOps("rotate", product, cpNodes[0].ExternalIP, ips)
	secretsEncryptOps("reencrypt", product, cpNodes[0].ExternalIP, ips)
	if strings.Contains(os.Getenv("TEST_TYPE"), "both") {
		shared.LogLevel("INFO", "TEST: 'NEW' Secrets Encryption method")
		secretsEncryptOps("rotate-keys", product, cpNodes[0].ExternalIP, ips)
	}
}

func secretsEncryptOps(action, product, cpIp string, ips []string) {
	shared.LogLevel("INFO", fmt.Sprintf("TEST: Secrets-Encryption: %s", action))
	_, errStatusB4 := shared.SecretEncryptOps("status", cpIp, product)
	Expect(errStatusB4).NotTo(HaveOccurred(), "error getting secret-encryption status before action")

	stdOutput, err := shared.SecretEncryptOps(action, cpIp, product)
	Expect(err).NotTo(HaveOccurred(), "error: secret-encryption: "+action)
	verifyStdOut(action, stdOutput)
	if (action == "reencrypt") || (action == "rotate-keys") {
		time.Sleep(20 * time.Second)
	}

	for _, node := range ips {
		nodearr := []string{node}
		nodeIp, errRestart := shared.ManageService(product, "restart", "server", nodearr)
		Expect(errRestart).NotTo(HaveOccurred(), "error restart service for node: "+nodeIp)
		// Order of reboot matters. Etcd first then control plane nodes.
		// Little lag needed between node restarts to avoid issues.
		shared.LogLevel("INFO", "Sleep 10 seconds - wait before restarting next node in cluster")
		time.Sleep(10 * time.Second)
	}
	switch product {
	case "k3s":
		shared.LogLevel("INFO", "Sleep 30 seconds - wait for services to come up")
		time.Sleep(30 * time.Second)
	case "rke2":
		shared.LogLevel("INFO", "Sleep 60 seconds - wait for services to come up")
		time.Sleep(60 * time.Second)
	}

	stdStatusOut, errStatus := shared.SecretEncryptOps("status", cpIp, product)
	Expect(errStatus).NotTo(HaveOccurred(), "error getting secret-encryption status")
	verifyStatusOutput(action, stdStatusOut)

	errLog := logEncryptionFileContents(ips, product)
	Expect(errLog).NotTo(HaveOccurred())
}

func verifyStdOut(action, stdout string) {
	switch action {
	case "prepare":
		Expect(stdout).To(ContainSubstring("prepare completed successfully"))
	case "rotate":
		Expect(stdout).To(ContainSubstring("rotate completed successfully"))
	case "reencrypt":
		Expect(stdout).To(ContainSubstring("reencryption started"))
	case "rotate-keys":
		Expect(stdout).To(ContainSubstring("keys rotated, reencryption started"))
	}
}

func verifyStatusOutput(action, stdout string) {
	switch action {
	case "prepare":
		Expect(stdout).To(ContainSubstring("Encryption Status: Enabled"))
		Expect(stdout).To(ContainSubstring("Current Rotation Stage: prepare"))
		Expect(stdout).To(ContainSubstring("Server Encryption Hashes: All hashes match"))
	case "rotate":
		Expect(stdout).To(ContainSubstring("Encryption Status: Enabled"))
		Expect(stdout).To(ContainSubstring("Current Rotation Stage: rotate"))
		Expect(stdout).To(ContainSubstring("Server Encryption Hashes: All hashes match"))
	case "reencrypt":
		Expect(stdout).To(ContainSubstring("Encryption Status: Enabled"))
		Expect(stdout).To(ContainSubstring("Current Rotation Stage: reencrypt_finished"))
		Expect(stdout).To(ContainSubstring("Server Encryption Hashes: All hashes match"))
	case "rotate-keys":
		Expect(stdout).To(ContainSubstring("Encryption Status: Enabled"))
		Expect(stdout).To(ContainSubstring("Current Rotation Stage: reencrypt_finished"))
		Expect(stdout).To(ContainSubstring("Server Encryption Hashes: All hashes match"))
	}
}

func getNodeIps(etcdNodes, cpNodes []shared.Node) []string {
	var nodeIps []string
	for _, node := range etcdNodes {
		nodeIps = append(nodeIps, node.ExternalIP)
	}
	for _, node := range cpNodes {
		nodeIps = append(nodeIps, node.ExternalIP)
	}
	return nodeIps
}

func logEncryptionFileContents(ips []string, product string) error {
	configFile := fmt.Sprintf("/var/lib/rancher/%s/server/cred/encryption-config.json", product)
	stateFile := fmt.Sprintf("/var/lib/rancher/%s/server/cred/encryption-state.json", product)
	cmdShowConfig := fmt.Sprintf("sudo cat %s", configFile)
	cmdShowState := fmt.Sprintf("sudo cat %s", stateFile)

	for _, ip := range ips {
		configStdOut, errConfig := shared.RunCommandOnNode(cmdShowConfig, ip)
		if errConfig != nil {
			return shared.ReturnLogError(fmt.Sprintf("Error cat of %s", configFile))
		}
		currentTime := time.Now()
		Expect(configStdOut).To(ContainSubstring(fmt.Sprintf("aescbckey-%s", currentTime.Format("2006-01-02"))))
		_, errState := shared.RunCommandOnNode(cmdShowState, ip)
		if errState != nil {
			return shared.ReturnLogError(fmt.Sprintf("Error cat of %s", stateFile))
		}
	}
	return nil
}
