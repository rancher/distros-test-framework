package testcase

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/gomega"
)

func TestSecretsEncryption() {
	nodes, errGetNodes := shared.GetNodesByRoles("etcd", "control-plane")
	Expect(nodes).NotTo(BeEmpty())
	Expect(errGetNodes).NotTo(HaveOccurred(), "error getting etcd/control-plane nodes")

	product, err := shared.Product()
	Expect(err).NotTo(HaveOccurred(), "error getting product from config")

	ips := getNodeIps(nodes)

	errSecret := shared.CreateSecret("secret1", "default")
	Expect(errSecret).NotTo(HaveOccurred(), "error creating secret")
	shared.LogLevel("INFO", "TEST: 'CLASSIC' Secrets Encryption method")
	index := len(nodes) - 1
	secretsEncryptOps("prepare", product, nodes[index].ExternalIP, ips)
	secretsEncryptOps("rotate", product, nodes[index].ExternalIP, ips)
	secretsEncryptOps("reencrypt", product, nodes[index].ExternalIP, ips)

	if strings.Contains(os.Getenv("TEST_TYPE"), "both") {
		shared.LogLevel("INFO", "TEST: 'NEW' Secrets Encryption method")
		secretsEncryptOps("rotate-keys", product, nodes[index].ExternalIP, ips)
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
		shared.LogLevel("DEBUG", "reencrypt op needs some time to complete - Sleep for 20 seconds before service restarts")
		time.Sleep(20 * time.Second) // Wait for reencrypt action to complete before restarting services
	}
	for i, node := range ips {
		nodearr := []string{node}
		nodeIp, errRestart := shared.ManageService(product, "restart", "server", nodearr)
		Expect(errRestart).NotTo(HaveOccurred(), "error restart service for node: "+nodeIp)
		// Order of reboot matters. Etcd first then control plane nodes.
		// Little lag needed between node restarts to avoid issues.
		waitEtcdErr := shared.WaitForPodsRunning(5, 4, false)
		if waitEtcdErr != nil {
			shared.LogLevel("WARN", "pods not up after 20 seconds.")
			if i != len(ips) {
				shared.LogLevel("DEBUG", "continue service restarts")
			}
		}
	}
	switch product {
	case "k3s":
		waitPodsErr := shared.WaitForPodsRunning(5, 6, false)
		if waitPodsErr != nil {
			shared.LogLevel("WARN", "pods not up after 30 seconds")
		}
	case "rke2":
		waitPodsErr := shared.WaitForPodsRunning(5, 12, false)
		if waitPodsErr != nil {
			shared.LogLevel("WARN", "pods not up after 60 seconds")
		}
	}

	secretEncryptStatus, errGetStatus := waitForHashMatch(cpIp, product, 5, 36) // Max 3 minute wait time for hash to match
	Expect(errGetStatus).NotTo(HaveOccurred(), "error getting secret-encryption status")
	verifyStatusOutput(action, secretEncryptStatus)

	errLog := logEncryptionFileContents(ips, product)
	Expect(errLog).NotTo(HaveOccurred())
}

func waitForHashMatch(cpIp, product string, defaultTime time.Duration, times int) (string, error) {
	var secretEncryptStatus string
	var errGetStatus error
	for i := 1; i <= times; i++ {
		secretEncryptStatus, errGetStatus := shared.SecretEncryptOps("status", cpIp, product)
		if errGetStatus != nil {
			shared.LogLevel("DEBUG", "error getting secret-encryption status. Retry.")
		}
		if secretEncryptStatus != "" && strings.Contains(secretEncryptStatus, "All hashes match") {
			shared.LogLevel("DEBUG", "Total sleep time before hashes matched: %d seconds", i*int(defaultTime))
			return secretEncryptStatus, nil
		} else {
			time.Sleep(defaultTime * time.Second)
		}
	}
	shared.LogLevel("WARN", "Hashes did not match after %d seconds", times*int(defaultTime))
	return secretEncryptStatus, errGetStatus
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

func getNodeIps(nodes []shared.Node) []string {
	var nodeIps []string
	for _, node := range nodes {
		nodeIps = append(nodeIps, node.ExternalIP)
		shared.LogLevel("DEBUG", "Node details: name: %s status: %s roles: %s external ip: %s", node.Name, node.Status, node.Roles, node.ExternalIP)
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
		shared.LogLevel("DEBUG", "cat %s:\n %s", configFile, configStdOut)
		currentTime := time.Now()
		Expect(configStdOut).To(ContainSubstring(fmt.Sprintf("aescbckey-%s",
			currentTime.Format("2006-01-02"))))
		stateOut, errState := shared.RunCommandOnNode(cmdShowState, ip)
		shared.LogLevel("DEBUG", "cat %s:\n %s", stateFile, stateOut)
		if errState != nil {
			return shared.ReturnLogError(fmt.Sprintf("Error cat of %s", stateFile))
		}
	}
	return nil
}
