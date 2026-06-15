package resources

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/rancher/distros-test-framework/internal/provisioning/driver"
)

// KubectlCommand return results from various commands, it receives an "action" , source and args.
// it already has KubeConfigFile
//
// destination = host or node
//
// action = get,describe...
//
// source = pods, node , exec, service ...
//
// args   = the rest of your command arguments.
func KubectlCommand(cluster *driver.Cluster, destination, action, source string, args ...string) (string, error) {
	shortCmd := map[string]string{
		"get":      "kubectl get",
		"describe": "kubectl describe",
		"exec":     "kubectl exec",
		"delete":   "kubectl delete",
		"apply":    "kubectl apply",
		"logs":     "kubectl logs",
	}

	cmdPrefix, ok := shortCmd[action]
	if !ok {
		cmdPrefix = action
	}

	resourceName := os.Getenv("resource_name")
	var cmd string
	switch destination {
	case "host":
		cmd = cmdPrefix + " " + source + " " + strings.Join(args, " ") + " --kubeconfig=" + KubeConfigFile

		return kubectlCmdOnHost(cmd)
	case "node":
		serverIP, _, err := ExtractServerIP(resourceName)
		if err != nil {
			return "", ReturnLogError("failed to extract server IP: %w", err)
		}
		kubeconfigFlagRemotePath := fmt.Sprintf("/etc/rancher/%s/%s.yaml", cluster.Config.Product, cluster.Config.Product)
		kubeconfigFlagRemote := " --kubeconfig=" + kubeconfigFlagRemotePath
		cmd = cmdPrefix + " " + source + " " + strings.Join(args, " ") + kubeconfigFlagRemote

		return kubectlCmdOnNode(cmd, serverIP)
	default:
		return "", ReturnLogError("invalid destination: %s", destination)
	}
}

// ExtractServerIP extracts the server IP from the kubeconfig file.
//
// returns the server ip and the kubeconfigContent as plain string.
func ExtractServerIP(resourceName string) (kubeConfigIP, kubeCfg string, err error) {
	if resourceName == "" {
		return "", "", ReturnLogError("resource name not sent\n")
	}

	localPath := fmt.Sprintf("/tmp/%s_kubeconfig", resourceName)
	kubeconfigContent, err := os.ReadFile(localPath)
	if err != nil {
		return "", "", ReturnLogError("failed to read kubeconfig file: %w\n", err)
	}
	// get server ip value from `server:` key.
	serverIP := strings.Split(string(kubeconfigContent), "server: ")[1]
	// removing newline.
	serverIP = strings.Split(serverIP, "\n")[0]
	// removing the https://.
	serverIP = strings.Join(strings.Split(serverIP, "https://")[1:], "")
	// removing the port.
	serverIP = strings.Split(serverIP, ":")[0]

	return serverIP, string(kubeconfigContent), nil
}

func kubectlCmdOnHost(cmd string) (string, error) {
	res, err := RunCommandHost(cmd)
	if err != nil {
		return "", ReturnLogError("failed to run kubectl command: %w\n", err)
	}

	return res, nil
}

func kubectlCmdOnNode(cmd, ip string) (string, error) {
	res, err := RunCommandOnNode(cmd, ip)
	if err != nil {
		return "", err
	}

	return res, nil
}

// FetchClusterIPs returns the cluster IPs and port of the service.
func FetchClusterIPs(namespace, svc string) (ip, port string, err error) {
	cmd := "kubectl get svc " + svc + " -n " + namespace +
		" -o jsonpath='{.spec.clusterIPs[*]}'  " + " --kubeconfig=" + KubeConfigFile
	ip, err = RunCommandHost(cmd)
	if err != nil {
		return "", "", ReturnLogError("failed to fetch cluster IPs: %v\n", err)
	}

	cmd = "kubectl get svc " + svc + " -n " + namespace +
		" -o jsonpath='{.spec.ports[0].port}' " + " --kubeconfig=" + KubeConfigFile
	port, err = RunCommandHost(cmd)
	if err != nil {
		return "", "", ReturnLogError("failed to fetch cluster port: %w\n", err)
	}

	return ip, port, err
}

// FetchServiceNodePort returns the node port of the service.
func FetchServiceNodePort(namespace, serviceName string) (string, error) {
	cmd := "kubectl get service -n " + namespace + " " + serviceName + " --kubeconfig=" + KubeConfigFile +
		" --output jsonpath=\"{.spec.ports[0].nodePort}\""
	nodeport, err := RunCommandHost(cmd)
	if err != nil {
		return "", ReturnLogError("failed to fetch service node port: %w", err)
	}

	return nodeport, nil
}

// FetchNodeExternalIPs returns the external IP of the nodes.
func FetchNodeExternalIPs() []string {
	res, err := RunCommandHost("kubectl get nodes " +
		"--output=jsonpath='{.items[*].status.addresses[?(@.type==\"ExternalIP\")].address}' " +
		"--kubeconfig=" + KubeConfigFile)
	if err != nil {
		LogLevel("error", "%w", err)
	}

	nodeExternalIP := strings.Trim(res, " ")
	nodeExternalIPs := strings.Split(nodeExternalIP, " ")

	return nodeExternalIPs
}

// InstallSonobuoy Executes scripts/install_sonobuoy.sh script.
// action	required install or cleanup sonobuoy plugin for mixed OS cluster.
// version	optional sonobouy version to be installed.
func InstallSonobuoy(action, version string) error {
	if action != "install" && action != "delete" {
		return ReturnLogError("invalid action: %s. Must be 'install' or 'delete'", action)
	}

	scriptsDir := BasePath() + "/scripts/install_sonobuoy.sh"
	err := os.Chmod(scriptsDir, 0o755)
	if err != nil {
		return ReturnLogError("failed to change script permissions: %w", err)
	}

	// Run the script directly so its #!/bin/bash shebang applies — it uses
	// bash-only syntax (here-strings, [[ ]]) that /bin/sh can't parse.
	cmd := exec.Command(scriptsDir, action, version)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return ReturnLogError("failed to execute %s action sonobuoy: %w\nOutput: %s", action, err, output)
	}

	return err
}

// WaitForKubeAPIReady waits until `kubectl get nodes` succeeds *continuously*
// for stableFor (default 30s). Any failure resets the streak.
func WaitForKubeAPIReady(timeout time.Duration) error {
	const (
		pollInterval = 5 * time.Second
		stableFor    = 30 * time.Second
	)

	deadline := time.Now().Add(timeout)
	cmd := "kubectl get nodes --kubeconfig=" + KubeConfigFile

	var streakStart time.Time
	for time.Now().Before(deadline) {
		_, err := RunCommandHost(cmd)
		if err == nil {
			if streakStart.IsZero() {
				streakStart = time.Now()
			}
			if time.Since(streakStart) >= stableFor {
				LogLevel("info", "kube-API stable for %s", stableFor)
				return nil
			}
		} else {
			if !streakStart.IsZero() {
				LogLevel("debug", "kube-API flapped after %s of stability — resetting", time.Since(streakStart))
			}
			streakStart = time.Time{}
		}
		time.Sleep(pollInterval)
	}

	return fmt.Errorf("kube-API not stable within %v", timeout)
}

// PrintClusterState prints the output of kubectl get nodes,pods -A -o wide.
func PrintClusterState() {
	cmd := "kubectl get nodes,pods -A -o wide --kubeconfig=" + KubeConfigFile
	res, err := RunCommandHost(cmd)
	if err != nil {
		_ = ReturnLogError("failed to print cluster state: %w\n", err)
	}
	LogLevel("info", "Current cluster state:\n%s\n", res)
}

// FetchIngressIP returns the ingress IP of the given namespace.
func FetchIngressIP(namespace string) (ingressIPs []string, err error) {
	res, err := RunCommandHost(
		"kubectl get ingress -n " +
			namespace +
			"  -o jsonpath='{.items[0].status.loadBalancer.ingress[*].ip}' --kubeconfig=" +
			KubeConfigFile,
	)
	if err != nil {
		return nil, ReturnLogError("failed to fetch ingress IP: %w\n", err)
	}

	ingressIP := strings.Trim(res, " ")
	if ingressIP == "" {
		return nil, nil
	}
	ingressIPs = strings.Split(ingressIP, " ")

	return ingressIPs, nil
}

func FetchToken(product, ip string) (string, error) {
	token, err := RunCommandOnNode(fmt.Sprintf("sudo cat /var/lib/rancher/%s/server/node-token", product), ip)
	if err != nil {
		return "", ReturnLogError("failed to fetch token: %w\n", err)
	}

	return token, nil
}

// PrintGetAll prints the output of kubectl get all -A -o wide and kubectl get nodes -o wide.
func PrintGetAll() {
	kubeconfigFile := " --kubeconfig=" + KubeConfigFile
	cmd := "kubectl get all -A -o wide  " + kubeconfigFile + " && kubectl get nodes -o wide " + kubeconfigFile
	res, err := RunCommandHost(cmd)
	if err != nil {
		LogLevel("error", "error from RunCommandHost: %v\n", err)
		return
	}

	fmt.Printf("\n\n\n-----------------  Results from kubectl get all -A -o wide"+
		"  -------------------\n\n%v\n\n\n\n", res)
}

func CreateSecret(secret, namespace string) error {
	kubectl := "kubectl --kubeconfig  " + KubeConfigFile

	if namespace == "" {
		namespace = "default"
	}
	if secret == "" {
		secret = "defaultSecret"
	}

	cmd := fmt.Sprintf("%s create secret generic %s -n %s --from-literal=mykey=mydata",
		kubectl, secret, namespace)
	createStdOut, err := RunCommandHost(cmd)
	if err != nil {
		return ReturnLogError("failed to create secret: \n%w", err)
	}
	if strings.Contains(createStdOut, "failed to create secret") {
		return ReturnLogError("failed to create secret: \n%w", err)
	}

	return nil
}

func ExtractKubeImageVersion() string {
	prod, serverVersion, err := Product()
	if err != nil {
		LogLevel("error", "error retrieving version of product: %s", err)
		os.Exit(1)
	}

	version := strings.Split(serverVersion, "+")[0]
	version = strings.TrimPrefix(version, prod+" version ")
	version = strings.TrimSpace(version)

	if strings.Contains(version, "-rc") {
		version = strings.Split(version, "-rc")[0]
	}

	if version == "" {
		LogLevel("error", "%s failed to resolve to server version string: %s", serverVersion, err)
		os.Exit(1)
	}
	LogLevel("info", "serverVersionReturnValue: %s", version)

	return version
}

// InstallProduct installs the product on the server node only.
// TODO: add support for installing on all nodes with all necessary flags.
func InstallProduct(cluster *driver.Cluster, publicIP, version string) error {
	err := setConfigFile(cluster, publicIP)
	if err != nil {
		return ReturnLogError("failed to set config file: %w", err)
	}

	installCmd := GetInstallCmd(cluster, version, "server")
	if cluster.Config.Product == "k3s" {
		skipInstall := fmt.Sprintf(" INSTALL_%s_SKIP_ENABLE=true ", strings.ToUpper(cluster.Config.Product))
		installCmd = strings.Replace(installCmd, "sh", skipInstall+" "+"  sh", 1)
	}

	LogLevel("info", "Install command: %s", installCmd)

	_, installCmdErr := RunCommandOnNode(installCmd, publicIP)
	if installCmdErr != nil {
		return ReturnLogError("failed to install product: \n%w", installCmdErr)
	}

	LogLevel("info", "%s successfully installed on server: %s", cluster.Config.Product, publicIP)

	return nil
}

// writeRestoreConfigFile builds the local /tmp/config.yaml with node-external-ip,
// the server flags, and secrets-encryption when the cluster is CIS-hardened.
// Returns the path of the written temp file.
func writeRestoreConfigFile(publicIP string) (string, error) {
	serverFlags := os.Getenv("SERVER_FLAGS")
	if serverFlags == "" {
		serverFlags = "write-kubeconfig-mode: 644"
	}
	serverFlags = strings.ReplaceAll(serverFlags, `\n`, "\n")

	tempFilePath := "/tmp/config.yaml"
	tempFile, err := os.Create(tempFilePath)
	if err != nil {
		return "", ReturnLogError("failed to create temp file: %w", err)
	}
	defer tempFile.Close()

	if _, writeErr := fmt.Fprintf(tempFile, "node-external-ip: %s\n", publicIP); writeErr != nil {
		return "", ReturnLogError("failed to write to temp file: %w", writeErr)
	}

	for _, entry := range strings.Split(serverFlags, "\n") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if _, err := fmt.Fprintf(tempFile, "%s\n", entry); err != nil {
			return "", ReturnLogError("failed to write to temp file: %w", err)
		}
	}

	// The qainfra ansible template enables CIS hardening (including
	// secrets-encryption) whenever server flags contain
	// protect-kernel-defaults. A snapshot taken from such a cluster holds
	// encrypted secrets, so a server restoring it must also enable
	// secrets-encryption or the apiserver never goes healthy
	// ("identity transformer tried to read encrypted data").
	hardened := strings.Contains(serverFlags, "protect-kernel-defaults")
	if hardened && !strings.Contains(serverFlags, "secrets-encryption") {
		if _, err := fmt.Fprintf(tempFile, "secrets-encryption: true\n"); err != nil {
			return "", ReturnLogError("failed to write to temp file: %w", err)
		}
	}

	return tempFile.Name(), nil
}

func setConfigFile(cluster *driver.Cluster, publicIP string) error {
	localConfig, err := writeRestoreConfigFile(publicIP)
	if err != nil {
		return err
	}

	remoteDir := fmt.Sprintf("/etc/rancher/%s/", cluster.Config.Product)
	remoteConfig := remoteDir + "config.yaml"

	// /etc/rancher is mode 0700 root-owned by K3s/RKE2 convention. Rather than
	// chmod'ing the parent (which would weaken the intentional protection),
	// SCP to /tmp (always writable by SSH_USER) and sudo-mv into place.
	stagingPath := "/tmp/config.yaml.staging"

	if _, err := RunCommandOnNode("sudo mkdir -p "+remoteDir, publicIP); err != nil {
		return ReturnLogError("failed to create remote directory: %w", err)
	}

	if err := RunScp(cluster, publicIP, []string{localConfig}, []string{stagingPath}); err != nil {
		return ReturnLogError("failed to scp config to staging: %w", err)
	}

	// `restorecon -F` resets the SELinux label to match policy (etc_t) since
	// the file inherits user_tmp_t from /tmp when scp'd. K3s reads as root so
	// it'd work either way, but the correct label avoids future policy
	// surprises. `|| true` because non-SELinux hosts (Ubuntu) lack the tool.
	moveCmd := fmt.Sprintf(
		"sudo mv %s %s && sudo chown root:root %s && sudo chmod 644 %s && "+
			"(command -v restorecon >/dev/null && sudo restorecon -F %s || true)",
		stagingPath, remoteConfig, remoteConfig, remoteConfig, remoteConfig,
	)
	if _, err := RunCommandOnNode(moveCmd, publicIP); err != nil {
		return ReturnLogError("failed to install config file: %w", err)
	}

	return nil
}
