package testcase

import (
	"fmt"
	"strings"
	"time"

	"github.com/rancher/distros-test-framework/pkg/customflag"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/gomega"
)

func TestSecretsEncryption(cluster *shared.Cluster, flags *customflag.FlagConfig) {
	nodes, errGetNodes := shared.GetNodesByRoles("etcd", "control-plane")
	Expect(nodes).NotTo(BeEmpty())
	Expect(errGetNodes).NotTo(HaveOccurred(), "error getting etcd/control-plane nodes")

	product, _, err := shared.Product()
	Expect(err).NotTo(HaveOccurred(), "error getting product from config")

	errSecret := shared.CreateSecret("secret1", "default")
	Expect(errSecret).NotTo(HaveOccurred(), "error creating secret")

	index := len(nodes) - 1
	cpIp := nodes[index].ExternalIP
	seErr := performSecretsEncryption(flags.SecretsEncrypt.Method, product,
		cluster.Config.ServerFlags, cluster.ServerIPs[0], cpIp, nodes)
	Expect(seErr).NotTo(HaveOccurred(), "error performing secrets-encryption")
}

func performSecretsEncryption(method, product, serverFlags, primaryNodeIp, cpIP string, nodes []shared.Node) (err error) {
	actions := []string{"prepare", "rotate", "reencrypt", "rotate-keys"}
	switch method {
	case "reencrypt":
		shared.LogLevel("info", "Performing secrets-encryption using prepare, rotate and reencrypt...")
		for _, action := range actions[:len(actions)-1] {
			err = secretsEncryptOps(action, product, serverFlags, primaryNodeIp, cpIP, nodes)
			if err != nil {
				return shared.ReturnLogError("error performing secrets-encrypt %s operation on node: %s!", action, cpIP)
			}
		}
	case "rotate-keys":
		shared.LogLevel("info", "Performing secrets-encryption using rotate-keys...")
		action := actions[len(actions)-1]
		err = secretsEncryptOps(action, product, serverFlags, primaryNodeIp, cpIP, nodes)
		if err != nil {
			return shared.ReturnLogError("error performing secrets-encrypt %s operation on node: %s!", action, cpIP)
		}
	case "both":
		for _, action := range actions {
			err = secretsEncryptOps(action, product, serverFlags, primaryNodeIp, cpIP, nodes)
			if err != nil {
				return shared.ReturnLogError("error performing secrets-encrypt %s operation on node: %s!", action, cpIP)
			}
		}
	default:
		return shared.ReturnLogError("unsupported method %s! Supported methods are: reencrypt, rotate-keys and both", method)
	}

	return nil
}

func secretsEncryptOps(action, product, serverFlags, primaryNodeIp, cpIP string, nodes []shared.Node) (err error) {
	shared.LogLevel("info", "Secrets-Encryption %v action starting...", action)

	_, errStatusB4 := shared.SecretEncryptOps("status", cpIP, product)
	Expect(errStatusB4).NotTo(HaveOccurred(), "error: getting secret-encryption status before action")

	stdOutput, err := shared.SecretEncryptOps(action, cpIP, product)
	if err != nil {
		return shared.ReturnLogError("error: performing secret-encryption: %v", action)
	}
	verifyActionStdOut(action, stdOutput)
	verifyStatusProvider(serverFlags, stdOutput)

	if (action == "reencrypt") || (action == "rotate-keys") {
		shared.LogLevel("debug", "waiting for %s action completion - 20 seconds before service restarts", action)
		time.Sleep(20 * time.Second)
	}

	// Restart Primary Etcd Node First
	restartServerAndWait(primaryNodeIp, product)

	// Restart all other server nodes - etcd and control plane
	for _, node := range nodes {
		if node.ExternalIP == primaryNodeIp {
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

	secretEncryptStatus, err := waitForHashMatch(cpIP, product)
	if err != nil {
		return shared.ReturnLogError("error: getting secret-encryption status")
	}
	verifyStatusStdOut(action, secretEncryptStatus)
	verifyStatusProvider(serverFlags, secretEncryptStatus)

	err = logEncryptionFileContents(nodes, serverFlags, action, product)
	if err != nil {
		return shared.ReturnLogError("error: logging secret-encryption file contents")
	}
	shared.LogLevel("info", "Secrets-Encryption %s action is completed!", action)

	return nil
}

func restartServerAndWait(ip, product string) {
	ms := shared.NewManageService(0, 0)

	action := shared.ServiceAction{
		Service:  product,
		Action:   "restart",
		NodeType: "server",
	}
	_, err := ms.ManageService(ip, []shared.ServiceAction{action})
	Expect(err).NotTo(HaveOccurred(), "error: restarting %s server service on %s", product, ip)

	// Little lag needed between node restarts to avoid issues.
	shared.LogLevel("debug", "Sleep for 30 seconds before service restarts between servers")
	time.Sleep(30 * time.Second)
	waitEtcdErr := shared.WaitForPodsRunning(10, 3)
	if waitEtcdErr != nil {
		shared.LogLevel("warn", "pods not up after 30 seconds.")
	}
}

func waitForHashMatch(cpIP, product string) (stdOut string, err error) {
	// Max 3 minute wait time for hash match.
	defaultTime := time.Duration(10)
	times := 6 * 3
	for i := 0; i < times; i++ {
		stdOut, err = shared.SecretEncryptOps("status", cpIP, product)
		if err != nil {
			shared.LogLevel("debug", "error getting secret-encryption status. Retry.")
		}
		if stdOut != "" && strings.Contains(stdOut, "All hashes match") {
			shared.LogLevel("debug", "Hash matched after: %d seconds", i*int(defaultTime))

			return stdOut, nil
		}
		time.Sleep(defaultTime * time.Second)
	}
	shared.LogLevel("warn", "Hashes did not match after %d seconds", times*int(defaultTime))

	return stdOut, err
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
		Expect(stdout).To(ContainSubstring("keys rotated, reencryption finished"))
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

// verifyStatusProvider Verifies secrets-encryption provider type post different actions.
//
// Verifies std output contains the corresponding provider: sudo k3s|rke2 secrets-encryption status.
//
// post the actions -> prepare|rotate|reencrypt|rotate-keys and restart services have been completed.
func verifyStatusProvider(serverFlags, stdout string) {
	if strings.Contains(serverFlags, "secrets-encryption-provider: secretbox") {
		Expect(stdout).To(ContainSubstring("XSalsa20-POLY1305"))
	} else {
		Expect(stdout).To(ContainSubstring("AES-CBC"))
	}
}

func logEncryptionFileContents(nodes []shared.Node, serverFlags, action, product string) error {
	configFile := fmt.Sprintf("/var/lib/rancher/%s/server/cred/encryption-config.json", product)
	stateFile := fmt.Sprintf("/var/lib/rancher/%s/server/cred/encryption-state.json", product)
	cmdShowConfig := "sudo cat  " + configFile
	cmdShowState := "sudo cat  " + stateFile

	for _, node := range nodes {
		ip := node.ExternalIP
		configStdOut, errConfig := shared.RunCommandOnNode(cmdShowConfig, ip)
		if errConfig != nil {
			return shared.ReturnLogError("error cat of %v", configFile)
		}
		shared.LogLevel("debug", "cat %s:\n %s", configFile, configStdOut)
		currentTime := time.Now()
		if strings.Contains(serverFlags, "secrets-encryption-provider: secretbox") {
			Expect(configStdOut).To(ContainSubstring("secretboxkey-" + currentTime.Format("2006-01-02")))
		} else {
			Expect(configStdOut).To(ContainSubstring("aescbckey-" + currentTime.Format("2006-01-02")))
		}

		stateOut, errState := shared.RunCommandOnNode(cmdShowState, ip)
		shared.LogLevel("debug", "cat %s:\n %s", stateFile, stateOut)
		if errState != nil {
			return shared.ReturnLogError("error cat of %v", stateFile)
		}
		if (action == "reencrypt") || (action == "rotate-keys") {
			Expect(stateOut).To(ContainSubstring("reencrypt_finished"))
		} else {
			Expect(stateOut).To(ContainSubstring(action))
		}
	}

	return nil
}
