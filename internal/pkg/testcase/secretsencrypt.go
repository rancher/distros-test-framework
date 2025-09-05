package testcase

import (
	"fmt"
	"strings"
	"time"

	"github.com/rancher/distros-test-framework/internal/pkg/customflag"
	"github.com/rancher/distros-test-framework/internal/resources"

	. "github.com/onsi/gomega"
)

func TestSecretsEncryption(cluster *resources.Cluster, flags *customflag.FlagConfig) {
	nodes, errGetNodes := resources.GetNodesByRoles("etcd", "control-plane")
	Expect(nodes).NotTo(BeEmpty())
	Expect(errGetNodes).NotTo(HaveOccurred(), "error getting etcd/control-plane nodes\n%v", errGetNodes)

	product, _, err := resources.Product()
	Expect(err).NotTo(HaveOccurred(), "error getting product from config\n%v", err)

	errSecret := resources.CreateSecret("secret1", "default")
	Expect(errSecret).NotTo(HaveOccurred(), "error creating secret\n%v", errSecret)

	index := len(nodes) - 1
	cpIp := nodes[index].ExternalIP
	seErr := performSecretsEncryption(flags.SecretsEncrypt.Method, product,
		cluster.Config.ServerFlags, cluster.ServerIPs[0], cpIp, nodes)
	Expect(seErr).NotTo(HaveOccurred(), "error performing secrets-encryption\n%v", seErr)
}

func performSecretsEncryption(method, product, serverFlags, primaryNodeIp, cpIP string, nodes []resources.Node) (err error) {
	actions := []string{"prepare", "rotate", "reencrypt", "rotate-keys"}
	switch method {
	case "classic":
		resources.LogLevel("info", "Performing secrets-encryption using prepare, rotate and reencrypt...")
		for _, action := range actions[:len(actions)-1] {
			err = secretsEncryptOps(action, product, serverFlags, primaryNodeIp, cpIP, nodes)
			if err != nil {
				return resources.ReturnLogError("error on node: %s!\n%v", cpIP, err)
			}
		}
	case "rotate-keys":
		resources.LogLevel("info", "Performing secrets-encryption using rotate-keys...")
		action := actions[len(actions)-1]
		err = secretsEncryptOps(action, product, serverFlags, primaryNodeIp, cpIP, nodes)
		if err != nil {
			return resources.ReturnLogError("error on node: %s!\n%v", cpIP, err)
		}
	case "both":
		for _, action := range actions {
			err = secretsEncryptOps(action, product, serverFlags, primaryNodeIp, cpIP, nodes)
			if err != nil {
				return resources.ReturnLogError("error on node: %s!\n%v", cpIP, err)
			}
		}
	default:
		return resources.ReturnLogError("unsupported method %s! Supported methods are: classic, rotate-keys and both", method)
	}

	return nil
}

func secretsEncryptOps(action, product, serverFlags, primaryNodeIp, cpIP string, nodes []resources.Node) (err error) {
	resources.LogLevel("info", "%s secrets-encrypt %v starting...", product, action)

	statusOut, errStatusB4 := resources.SecretEncryptOps("status", cpIP, product)
	Expect(errStatusB4).NotTo(HaveOccurred(), "error: getting secret-encryption status before action")

	stdOutput, err := resources.SecretEncryptOps(action, cpIP, product)
	if err != nil {
		return resources.ReturnLogError("error on %s secret-encrypt %s:\n%v", product, action, err)
	}
	verifyActionStdOut(action, stdOutput)
	verifyStatusProvider(serverFlags, statusOut)

	if (action == "reencrypt") || (action == "rotate-keys") {
		resources.LogLevel("debug", "waiting for %s action completion - 20 seconds before service restarts", action)
		time.Sleep(20 * time.Second)
	}

	err = restartService(product, primaryNodeIp, nodes)
	if err != nil {
		return resources.ReturnLogError("%v", err)
	}

	switch product {
	case "k3s":
		waitPodsErr := resources.WaitForPodsRunning(10, 3)
		if waitPodsErr != nil {
			resources.LogLevel("warn", "pods not up after 30 seconds")
		}
	case "rke2":
		waitPodsErr := resources.WaitForPodsRunning(10, 6)
		if waitPodsErr != nil {
			resources.LogLevel("warn", "pods not up after 60 seconds")
		}
	}

	secretEncryptStatus, err := waitForHashMatch(cpIP, product)
	if err != nil {
		return resources.ReturnLogError("error on %s secret-encrypt status on node: %s\n%v", product, cpIP, err)
	}
	verifyStatusStdOut(action, secretEncryptStatus)
	verifyStatusProvider(serverFlags, secretEncryptStatus)

	err = logEncryptionFileContents(nodes, serverFlags, action, product)
	if err != nil {
		return resources.ReturnLogError("error logging secret-encryption file contents...\n%v", err)
	}
	resources.LogLevel("info", "%s secrets-encrypt %s is completed!", product, action)

	return nil
}

func restartServerAndWait(ip, product string) (err error) {
	ms := resources.NewManageService(5, 5)

	action := resources.ServiceAction{
		Service:  product,
		Action:   "restart",
		NodeType: "server",
	}
	_, err = ms.ManageService(ip, []resources.ServiceAction{action})
	if err != nil {
		return resources.ReturnLogError("error restarting %s service on server node: %s\n%v", product, ip, err)
	}

	// Little lag needed between node restarts to avoid issues.
	resources.LogLevel("debug", "Sleep for 30 seconds before service restarts between servers")
	time.Sleep(30 * time.Second)
	waitEtcdErr := resources.WaitForPodsRunning(10, 3)
	if waitEtcdErr != nil {
		resources.LogLevel("warn", "pods not up after 30 seconds.")
	}

	return nil
}

func restartService(product, primaryNodeIp string, nodes []resources.Node) (err error) {
	// Restart Primary Etcd Node First
	err = restartServerAndWait(primaryNodeIp, product)
	if err != nil {
		return resources.ReturnLogError("error restarting %s service on node: %s\n%v", product, primaryNodeIp, err)
	}

	// Restart all other server nodes - etcd and control plane
	for _, node := range nodes {
		if node.ExternalIP == primaryNodeIp {
			continue
		}
		err = restartServerAndWait(node.ExternalIP, product)
		if err != nil {
			return resources.ReturnLogError("error restarting %s service on node: %s\n%v", product, node.ExternalIP, err)
		}
	}

	return nil
}

func waitForHashMatch(cpIP, product string) (statusOut string, err error) {
	// Max 3 minute wait time for hash match.
	defaultTime := time.Duration(10)
	times := 6 * 3
	for i := 0; i < times; i++ {
		statusOut, err = resources.SecretEncryptOps("status", cpIP, product)
		if err != nil {
			resources.LogLevel("debug", "error getting secret-encryption status. Retry.")
		}
		if statusOut != "" && strings.Contains(statusOut, "All hashes match") {
			resources.LogLevel("debug", "Hash matched after: %d seconds", i*int(defaultTime))

			return statusOut, nil
		}
		time.Sleep(defaultTime * time.Second)
	}
	resources.LogLevel("warn", "Hashes did not match after %d seconds", times*int(defaultTime))

	return statusOut, err
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

func logEncryptionFileContents(nodes []resources.Node, serverFlags, action, product string) error {
	configFile := fmt.Sprintf("/var/lib/rancher/%s/server/cred/encryption-config.json", product)
	stateFile := fmt.Sprintf("/var/lib/rancher/%s/server/cred/encryption-state.json", product)
	cmdShowConfig := "sudo cat  " + configFile
	cmdShowState := "sudo cat  " + stateFile

	for _, node := range nodes {
		ip := node.ExternalIP
		configStdOut, errConfig := resources.RunCommandOnNode(cmdShowConfig, ip)
		if errConfig != nil {
			return resources.ReturnLogError("error cat of %v", configFile)
		}
		resources.LogLevel("debug", "cat %s:\n %s", configFile, configStdOut)
		currentTime := time.Now()
		if strings.Contains(serverFlags, "secrets-encryption-provider: secretbox") {
			Expect(configStdOut).To(ContainSubstring("secretboxkey-" + currentTime.Format("2006-01-02")))
		} else {
			Expect(configStdOut).To(ContainSubstring("aescbckey-" + currentTime.Format("2006-01-02")))
		}

		stateOut, errState := resources.RunCommandOnNode(cmdShowState, ip)
		resources.LogLevel("debug", "cat %s:\n %s", stateFile, stateOut)
		if errState != nil {
			return resources.ReturnLogError("error cat of %v", stateFile)
		}
		if (action == "reencrypt") || (action == "rotate-keys") {
			Expect(stateOut).To(ContainSubstring("reencrypt_finished"))
		} else {
			Expect(stateOut).To(ContainSubstring(action))
		}
	}

	return nil
}
