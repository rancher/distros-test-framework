package resources

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

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

	cmd := exec.Command("/bin/sh", scriptsDir, action, version)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return ReturnLogError("failed to execute %s action sonobuoy: %w\nOutput: %s", action, err, output)
	}

	return err
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

func PrintGetAllForK3k(host *driver.HostCluster, namespace, kubectlKubeConfigPath string) {
	cmd := fmt.Sprintf("%s get all -n %s -o wide && %s get nodes -o wide ", kubectlKubeConfigPath, namespace, kubectlKubeConfigPath)
	res, err := RunCommandOnNode(cmd, host.ServerIP)
	if err != nil {
		LogLevel("error", "error from RunCommandOnNode: %v\n", err)
		return
	}

	fmt.Printf("\n\n\n-----------------  $ %s get all -A -o wide"+
		"  -------------------\n\n%v\n\n\n\n", kubectlKubeConfigPath, res)
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

func setConfigFile(cluster *driver.Cluster, publicIP string) error {
	serverFlags := os.Getenv("server_flags")
	if serverFlags == "" {
		serverFlags = "write-kubeconfig-mode: 644"
	}
	serverFlags = strings.ReplaceAll(serverFlags, `\n`, "\n")

	tempFilePath := "/tmp/config.yaml"
	tempFile, err := os.Create(tempFilePath)
	if err != nil {
		return ReturnLogError("failed to create temp file: %w", err)
	}
	defer tempFile.Close()

	_, writeErr := fmt.Fprintf(tempFile, "node-external-ip: %s\n", publicIP)
	if writeErr != nil {
		return ReturnLogError("failed to write to temp file: %w", writeErr)
	}

	flagValues := strings.Split(serverFlags, "\n")
	for _, entry := range flagValues {
		entry = strings.TrimSpace(entry)
		if entry != "" {
			_, err := fmt.Fprintf(tempFile, "%s\n", entry)
			if err != nil {
				return ReturnLogError("failed to write to temp file: %w", err)
			}
		}
	}

	remoteDir := fmt.Sprintf("/etc/rancher/%s/", cluster.Config.Product)
	user := os.Getenv("aws_user")
	cmd := fmt.Sprintf("sudo mkdir -p %s && sudo chown %s %s ", remoteDir, user, remoteDir)

	_, mkdirCmdErr := RunCommandOnNode(cmd, publicIP)
	if mkdirCmdErr != nil {
		return ReturnLogError("failed to create remote directory: %w", mkdirCmdErr)
	}

	scpErr := RunScp(cluster, publicIP, []string{tempFile.Name()}, []string{remoteDir + "config.yaml"})
	if scpErr != nil {
		return ReturnLogError("failed to copy file: %w", scpErr)
	}

	return nil
}
