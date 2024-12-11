package testcase

import (
	"fmt"
	"strings"
	"time"

	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/gomega"
)

func TestSecretsEncryption(cluster *shared.Cluster) {
	Expect(cluster.Status).To(Equal("cluster created"))
	Expect(cluster.ServerIPs).ShouldNot(BeEmpty())

	nodes, errGetNodes := shared.GetNodesByRoles("etcd", "control-plane")
	Expect(nodes).NotTo(BeEmpty())
	Expect(errGetNodes).NotTo(HaveOccurred(), "error getting etcd/control-plane nodes")

	product, _, err := shared.Product()
	Expect(err).NotTo(HaveOccurred(), "error getting product from config")

	errSecret := shared.CreateSecret("secret1", "default")
	Expect(errSecret).NotTo(HaveOccurred(), "error creating secret")

	index := len(nodes) - 1
	cpIp := nodes[index].ExternalIP

	secretsEncryptOps("prepare", product, cluster.ServerIPs[0], cpIp, nodes)
	secretsEncryptOps("rotate", product, cluster.ServerIPs[0], cpIp, nodes)
	secretsEncryptOps("reencrypt", product, cluster.ServerIPs[0], cpIp, nodes)
	secretsEncryptOps("rotate-keys", product, cluster.ServerIPs[0], cpIp, nodes)
}

func secretsEncryptOps(action, product, primaryEtcdIp, cpIP string, nodes []shared.Node) {
	shared.LogLevel("info", "TEST: Secrets-Encryption:  "+action)
	_, errStatusB4 := shared.SecretEncryptOps("status", cpIP, product)
	Expect(errStatusB4).NotTo(HaveOccurred(), "error getting secret-encryption status before action")

	stdOutput, err := shared.SecretEncryptOps(action, cpIP, product)
	Expect(err).NotTo(HaveOccurred(), "error: secret-encryption: "+action)
	verifyActionStdOut(action, stdOutput)

	shared.LogLevel("debug", "secrets-encrypt ops need to complete - Sleep for 30 seconds before service restarts")
	time.Sleep(30 * time.Second)

	// Restart Primary Etcd Node First
	restartServerAndWait(primaryEtcdIp, product)

	// Restart all other server nodes - etcd and control plane
	for _, node := range nodes {
		if node.ExternalIP == primaryEtcdIp {
			continue
		}
		restartServerAndWait(node.ExternalIP, product)
	}

	switch product {
	case "k3s":
		waitPodsErr := shared.WaitForPodsRunning(10, 3)
		if waitPodsErr != nil {
			shared.LogLevel("warn", "pods not up after 30 seconds")
		}
	case "rke2":
		waitPodsErr := shared.WaitForPodsRunning(10, 6)
		if waitPodsErr != nil {
			shared.LogLevel("warn", "pods not up after 60 seconds")
		}
	}

	secretEncryptStatus, errGetStatus := waitForHashMatch(cpIP, product)
	Expect(errGetStatus).NotTo(HaveOccurred(), "error getting secret-encryption status")
	verifyStatusStdOut(action, secretEncryptStatus)

	errLog := logEncryptionFileContents(nodes, action, product)
	Expect(errLog).NotTo(HaveOccurred())
}

func restartServerAndWait(ip, product string) {
	nodearr := []string{ip}
	nodeIP, errRestart := shared.ManageService(product, "restart", "server", nodearr)
	Expect(errRestart).NotTo(HaveOccurred(), "error restart service for node: "+nodeIP)
	// Little lag needed between node restarts to avoid issues.
	shared.LogLevel("debug", "Sleep for 30 seconds before service restarts between servers")
	time.Sleep(30 * time.Second)
	waitEtcdErr := shared.WaitForPodsRunning(10, 3)
	if waitEtcdErr != nil {
		shared.LogLevel("warn", "pods not up after 30 seconds.")
	}
}

func waitForHashMatch(cpIP, product string) (string, error) {
	// Max 3 minute wait time for hash match.
	defaultTime := time.Duration(10)
	times := 6 * 3
	var secretEncryptStatus string
	var errGetStatus error
	for i := 1; i <= times; i++ {
		secretEncryptStatus, errGetStatus = shared.SecretEncryptOps("status", cpIP, product)
		if errGetStatus != nil {
			shared.LogLevel("debug", "error getting secret-encryption status. Retry.")
		}
		if secretEncryptStatus != "" && strings.Contains(secretEncryptStatus, "All hashes match") {
			shared.LogLevel("debug", "Hash matched after: %d seconds", i*int(defaultTime))

			return secretEncryptStatus, nil
		}
		time.Sleep(defaultTime * time.Second)
	}
	shared.LogLevel("warn", "Hashes did not match after %d seconds", times*int(defaultTime))

	return secretEncryptStatus, errGetStatus
}

// verifyActionStdOut Verifies secrets-encryption action outputs.
//
// Verifies std outputs of: sudo k3s|rke2 secrets-encryption prepare|rotate|reencrypt|rotate-keys actions.
func verifyActionStdOut(action, stdout string) {
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

// verifyStatusStdOut Verifies secrets-encryption status outputs post different actions.
//
// Verifies std output of: sudo k3s|rke2 secrets-encryption status.
//
// post the action -prepare|rotate|reencrypt|rotate-keys and restart services have been completed.
func verifyStatusStdOut(action, stdout string) {
	Expect(stdout).To(ContainSubstring("Encryption Status: Enabled"))
	Expect(stdout).To(ContainSubstring("Server Encryption Hashes: All hashes match"))
	switch action {
	case "prepare":
		Expect(stdout).To(ContainSubstring("Current Rotation Stage: prepare"))
	case "rotate":
		Expect(stdout).To(ContainSubstring("Current Rotation Stage: rotate"))
	default:
		Expect(stdout).To(ContainSubstring("Current Rotation Stage: reencrypt_finished"))
	}
}

func logEncryptionFileContents(nodes []shared.Node, action, product string) error {
	configFile := fmt.Sprintf("/var/lib/rancher/%s/server/cred/encryption-config.json", product)
	stateFile := fmt.Sprintf("/var/lib/rancher/%s/server/cred/encryption-state.json", product)
	cmdShowConfig := "sudo cat  " + configFile
	cmdShowState := "sudo cat  " + stateFile

	for _, node := range nodes {
		ip := node.ExternalIP
		configStdOut, errConfig := shared.RunCommandOnNode(cmdShowConfig, ip)
		if errConfig != nil {
			return shared.ReturnLogError("error cat of " + configFile)
		}
		shared.LogLevel("debug", "cat %s:\n %s", configFile, configStdOut)
		currentTime := time.Now()
		Expect(configStdOut).To(ContainSubstring("aescbckey-" + currentTime.Format("2006-01-02")))

		stateOut, errState := shared.RunCommandOnNode(cmdShowState, ip)
		shared.LogLevel("debug", "cat %s:\n %s", stateFile, stateOut)
		if errState != nil {
			return shared.ReturnLogError("error cat of " + stateFile)
		}
		if (action == "reencrypt") || (action == "rotate-keys") {
			Expect(stateOut).To(ContainSubstring("reencrypt_finished"))
		} else {
			Expect(stateOut).To(ContainSubstring(action))
		}
	}

	return nil
}
