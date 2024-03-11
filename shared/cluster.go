package shared

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/rancher/distros-test-framework/config"
	"github.com/rancher/distros-test-framework/factory"
)

type Node struct {
	Name       string
	Status     string
	Roles      string
	Version    string
	InternalIP string
	ExternalIP string
}

type Pod struct {
	NameSpace      string
	Name           string
	Ready          string
	Status         string
	Restarts       string
	Age            string
	NodeIP         string
	Node           string
	NominatedNode  string
	ReadinessGates string
}

// ManageWorkload applies or deletes a workload based on the action: apply or delete.
func ManageWorkload(action string, workloads ...string) error {
	cluster := factory.ClusterConfig()
	if action != "apply" && action != "delete" {
		return ReturnLogError("invalid action: %s. Must be 'apply' or 'delete'", action)
	}

	resourceDir := BasePath() + "/workloads/" + cluster.Config.Arch

	files, err := os.ReadDir(resourceDir)
	if err != nil {
		return ReturnLogError("Unable to read resource manifest file for: %s\n", resourceDir)
	}

	for _, workload := range workloads {
		if !fileExists(files, workload) {
			return ReturnLogError("workload %s not found", workload)
		}

		err := handleWorkload(action, resourceDir, workload)
		if err != nil {
			return err
		}
	}

	return nil
}

func handleWorkload(action, resourceDir, workload string) error {
	filename := filepath.Join(resourceDir, workload)

	switch action {
	case "apply":
		return applyWorkload(workload, filename)
	case "delete":
		return deleteWorkload(workload, filename)
	default:
		return ReturnLogError("invalid action: %s. Must be 'apply' or 'delete'", action)
	}
}

func applyWorkload(workload, filename string) error {
	fmt.Println("\nApplying ", workload)
	cmd := "kubectl apply -f " + filename + " --kubeconfig=" + factory.KubeConfigFile
	out, err := RunCommandHost(cmd)
	if err != nil || out == "" {
		if strings.Contains(out, "error when creating") {
			return fmt.Errorf("failed to apply workload %s: %s", workload, out)
		}
		return ReturnLogError("failed to run kubectl apply: %w", err)
	}

	out, err = RunCommandHost("kubectl get all -A --kubeconfig=" + factory.KubeConfigFile)
	if err != nil {
		return ReturnLogError("failed to run kubectl get all: %w\n", err)
	}

	if ok := !strings.Contains(out, "Creating") && strings.Contains(out, workload); ok {
		return ReturnLogError("failed to apply workload %s", workload)
	}

	return nil
}

// deleteWorkload deletes a workload and asserts that the workload is deleted.
func deleteWorkload(workload, filename string) error {
	fmt.Println("\nRemoving", workload)
	cmd := "kubectl delete -f " + filename + " --kubeconfig=" + factory.KubeConfigFile

	_, err := RunCommandHost(cmd)
	if err != nil {
		return err
	}

	timeout := time.After(30 * time.Second)
	tick := time.NewTicker(2 * time.Second)

	for {
		select {
		case <-tick.C:
			res, err := RunCommandHost("kubectl get all -A --kubeconfig=" + factory.KubeConfigFile)
			if err != nil {
				return ReturnLogError("failed to run kubectl get all: %v\n", err)
			}
			isDeleted := !strings.Contains(res, workload)
			if isDeleted {
				return nil
			}
		case <-timeout:
			return ReturnLogError("workload delete timed out")
		}
	}
}

// KubectlCommand return results from various commands, it receives an "action" , source and args.
// it already has factory.KubeConfigFile
//
// destination = host or node
//
// action = get,describe...
//
// source = pods, node , exec, service ...
//
// args   = the rest of your command arguments
func KubectlCommand(destination, action, source string, args ...string) (string, error) {
	kubeconfigFlag := " --kubeconfig=" + factory.KubeConfigFile
	shortCmd := map[string]string{
		"get":      "kubectl get",
		"describe": "kubectl describe",
		"exec":     "kubectl exec",
		"delete":   "kubectl delete",
		"apply":    "kubectl apply",
	}

	cmdPrefix, ok := shortCmd[action]
	if !ok {
		cmdPrefix = action
	}

	cfg, err := config.AddEnv()
	if err != nil {
		return "", ReturnLogError("failed to get config path: %v\n", err)
	}

	if envErr := config.SetEnv(BasePath() + fmt.Sprintf("/config/%s.tfvars",
		cfg.Product)); envErr != nil {
		return "", ReturnLogError("error setting env: %w\n", envErr)
	}

	resourceName := os.Getenv("resource_name")

	var cmd string
	switch destination {
	case "host":
		cmd = cmdPrefix + " " + source + " " + strings.Join(args, " ") + kubeconfigFlag
		return kubectlCmdOnHost(cmd)
	case "node":
		serverIP, _, err := kubeCfgServerIP(resourceName)
		if err != nil {
			return "", ReturnLogError("failed to extract server IP: %w", err)
		}
		kubeconfigFlagRemotePath := fmt.Sprintf("/etc/rancher/%s/%s.yaml", cfg.Product, cfg.Product)
		kubeconfigFlagRemote := " --kubeconfig=" + kubeconfigFlagRemotePath
		cmd = cmdPrefix + " " + source + " " + strings.Join(args, " ") + kubeconfigFlagRemote
		return kubectlCmdOnNode(cmd, serverIP)
	default:
		return "", ReturnLogError("invalid destination: %s", destination)
	}
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
		" -o jsonpath='{.spec.clusterIPs[*]}' --kubeconfig=" + factory.KubeConfigFile
	ip, err = RunCommandHost(cmd)
	if err != nil {
		return "", "", ReturnLogError("failed to fetch cluster IPs: %v\n", err)
	}

	cmd = "kubectl get svc " + svc + " -n " + namespace +
		" -o jsonpath='{.spec.ports[0].port}' --kubeconfig=" + factory.KubeConfigFile
	port, err = RunCommandHost(cmd)
	if err != nil {
		return "", "", ReturnLogError("failed to fetch cluster port: %v\n", err)
	}

	return ip, port, err
}

// FetchServiceNodePort returns the node port of the service
func FetchServiceNodePort(namespace, serviceName string) (string, error) {
	cmd := "kubectl get service -n " + namespace + " " + serviceName + " --kubeconfig=" + factory.KubeConfigFile +
		" --output jsonpath=\"{.spec.ports[0].nodePort}\""
	nodeport, err := RunCommandHost(cmd)
	if err != nil {
		return "", ReturnLogError("failed to fetch service node port: %v", err)
	}

	return nodeport, nil
}

// FetchNodeExternalIP returns the external IP of the nodes.
func FetchNodeExternalIP() []string {
	res, _ := RunCommandHost("kubectl get nodes " +
		"--output=jsonpath='{.items[*].status.addresses[?(@.type==\"ExternalIP\")].address}' " +
		"--kubeconfig=" + factory.KubeConfigFile)
	nodeExternalIP := strings.Trim(res, " ")
	nodeExternalIPs := strings.Split(nodeExternalIP, " ")

	return nodeExternalIPs
}

// RestartCluster restarts the service on each node given by external IP.
func RestartCluster(product, ip string) {
	_, _ = RunCommandOnNode(fmt.Sprintf("sudo systemctl restart %s*", product), ip)
	time.Sleep(20 * time.Second)
}

// FetchIngressIP returns the ingress IP of the given namespace
func FetchIngressIP(namespace string) (ingressIPs []string, err error) {
	res, err := RunCommandHost(
		"kubectl get ingress -n " +
			namespace +
			"  -o jsonpath='{.items[0].status.loadBalancer.ingress[*].ip}' --kubeconfig=" +
			factory.KubeConfigFile,
	)
	if err != nil {
		return nil, ReturnLogError("failed to fetch ingress IP: %v\n", err)
	}

	ingressIP := strings.Trim(res, " ")
	if ingressIP == "" {
		return nil, nil
	}
	ingressIPs = strings.Split(ingressIP, " ")

	return ingressIPs, nil
}

// SonobuoyMixedOS Executes scripts/mixedos_sonobuoy.sh script
// action	required install or cleanup sonobuoy plugin for mixed OS cluster
// version	optional sonobouy version to be installed
func SonobuoyMixedOS(action, version string) error {
	if action != "install" && action != "delete" {
		return ReturnLogError("invalid action: %s. Must be 'install' or 'delete'", action)
	}

	scriptsDir := BasePath() + "/scripts/mixedos_sonobuoy.sh"
	err := os.Chmod(scriptsDir, 0755)
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

// PrintClusterState prints the output of kubectl get nodes,pods -A -o wide
func PrintClusterState() {
	cmd := "kubectl get nodes,pods -A -o wide --kubeconfig=" + factory.KubeConfigFile
	res, err := RunCommandHost(cmd)
	if err != nil {
		_ = ReturnLogError("failed to print cluster state: %w\n", err)
	}
	fmt.Println("\n", res)
}

// GetNodes returns nodes parsed from kubectl get nodes.
func GetNodes(print bool) ([]Node, error) {
	res, err := RunCommandHost("kubectl get nodes -o wide --no-headers --kubeconfig=" + factory.KubeConfigFile)
	if err != nil {
		return nil, err
	}

	nodes := parseNodes(res)
	if print {
		fmt.Println(res)
	}

	return nodes, nil
}

// GetNodesByRoles takes in one or multiple node roles and returns the slice of nodes that have those roles
// Valid values for roles are: etcd, control-plane, worker
func GetNodesByRoles(roles ...string) ([]Node, error) {
	var nodes []Node
	var matchedNodes []Node

	if roles == nil {
		return nil, ReturnLogError("no roles provided")
	}

	validRoles := map[string]bool{
		"etcd":          true,
		"control-plane": true,
		"worker":        true,
	}

	for _, role := range roles {
		if !validRoles[role] {
			return nil, ReturnLogError("invalid role: %s", role)
		}

		cmd := "kubectl get nodes -o wide --sort-by '{.metadata.name}'" +
			" --no-headers --kubeconfig=" + factory.KubeConfigFile +
			" -l role-" + role
		res, err := RunCommandHost(cmd)
		if err != nil {
			return nil, err
		}
		matchedNodes = append(matchedNodes, parseNodes(res)...)
	}

	for _, matchedNode := range matchedNodes {
		nodes = appendNodeIfMissing(nodes, matchedNode)
	}

	return nodes, nil
}

// parseNodes parses the nodes from the kubeclt get nodes command.
func parseNodes(res string) []Node {
	nodes := make([]Node, 0, 10)
	nodeList := strings.Split(strings.TrimSpace(res), "\n")
	for _, rec := range nodeList {
		if strings.TrimSpace(rec) == "" {
			continue
		}

		fields := strings.Fields(rec)
		if len(fields) < 7 {
			continue
		}

		n := Node{
			Name:       fields[0],
			Status:     fields[1],
			Roles:      fields[2],
			Version:    fields[4],
			InternalIP: fields[5],
			ExternalIP: fields[6],
		}
		nodes = append(nodes, n)
	}

	return nodes
}

// GetPods returns pods parsed from kubectl get pods.
func GetPods(print bool) ([]Pod, error) {
	cmd := "kubectl get pods -o wide --no-headers -A --kubeconfig=" + factory.KubeConfigFile
	res, err := RunCommandHost(cmd)
	if err != nil {
		return nil, ReturnLogError("failed to get pods: %w\n", err)
	}

	pods := parsePods(res)
	if print {
		fmt.Println("\nCluster pods:\n", res)
	}

	return pods, nil
}

// GetPodsFiltered returns pods parsed from kubectl get pods with any specific filters
// Example filters are: namespace, label, --field-selector
func GetPodsFiltered(filters map[string]string) ([]Pod, error) {
	cmd := fmt.Sprintf("kubectl get pods -o wide --no-headers --kubeconfig=%s", factory.KubeConfigFile)
	for option, value := range filters {
		var opt string

		switch option {
		case "namespace":
			opt = "-n"
		case "label":
			opt = "-l"
		default:
			opt = option
		}
		cmd = strings.Join([]string{cmd, opt, value}, " ")
	}

	res, err := RunCommandHost(cmd)
	if err != nil {
		return nil, ReturnLogError("failed to get pods: %w\n", err)
	}

	pods := parsePods(res)

	return pods, nil
}

// parsePods parses the pods from the kubeclt get pods command.
func parsePods(res string) []Pod {
	pods := make([]Pod, 0, 10)
	podList := strings.Split(strings.TrimSpace(res), "\n")

	for _, rec := range podList {
		offset := 0
		fields := regexp.MustCompile(`\s{2,}`).Split(rec, -1)
		if strings.TrimSpace(rec) == "" || len(fields) < 9 {
			continue
		}
		var p Pod
		if len(fields) == 10 {
			p.NameSpace = fields[0]
			offset = 1
		}
		p.Name = fields[offset]
		p.Ready = fields[offset+1]
		p.Status = fields[offset+2]
		p.Restarts = regexp.MustCompile(`\([^\)]+\)`).Split(fields[offset+3], -1)[0]
		p.Age = fields[offset+4]
		p.NodeIP = fields[offset+5]
		p.Node = fields[offset+6]
		p.NominatedNode = fields[offset+7]
		p.ReadinessGates = fields[offset+8]

		pods = append(pods, p)
	}
	return pods
}

// ReadDataPod reads the data from the pod
func ReadDataPod(namespace string) (string, error) {
	podName, err := KubectlCommand(
		"host",
		"get",
		"pods",
		"-n "+namespace+" -o jsonpath={.items[0].metadata.name}",
	)
	if err != nil {
		LogLevel("error", "failed to fetch pod name: \n%w", err)
		os.Exit(1)
	}

	cmd := "kubectl exec -n local-path-storage " + podName + " --kubeconfig=" + factory.KubeConfigFile +
		" -- cat /data/test"

	res, err := RunCommandHost(cmd)
	if err != nil {
		return "", err
	}

	return res, nil
}

// WriteDataPod writes data to the pod
func WriteDataPod(namespace string) (string, error) {
	podName, err := KubectlCommand(
		"host",
		"get",
		"pods",
		"-n "+namespace+" -o jsonpath={.items[0].metadata.name}",
	)
	if err != nil {
		return "", ReturnLogError("failed to fetch pod name: \n%w", err)
	}

	cmd := "kubectl exec -n local-path-storage  " + podName + " --kubeconfig=" + factory.KubeConfigFile +
		" -- sh -c 'echo testing local path > /data/test' "

	return RunCommandHost(cmd)
}

// kubeCfgServerIP extracts the server IP from the kubeconfig file.
func kubeCfgServerIP(resourceName string) (kubeConfigIP, kubeCfg string, err error) {
	if resourceName == "" {
		return "", "", ReturnLogError("resource name not sent\n")
	}

	localPath := fmt.Sprintf("/tmp/%s_kubeconfig", resourceName)
	kubeconfigContent, err := os.ReadFile(localPath)
	if err != nil {
		return "", "", ReturnLogError("failed to read kubeconfig file: %w\n", err)
	}
	// get server ip value from `server:` key
	serverIP := strings.Split(string(kubeconfigContent), "server: ")[1]
	// removing newline
	serverIP = strings.Split(serverIP, "\n")[0]
	// removing the https://
	serverIP = strings.Join(strings.Split(serverIP, "https://")[1:], "")
	// removing the port
	serverIP = strings.Split(serverIP, ":")[0]

	LogLevel("info", "Extracted from local kube config file server ip: %s", serverIP)

	return serverIP, string(kubeconfigContent), nil
}

// GetNodeArgsMap returns list of nodeArgs map
func GetNodeArgsMap(nodeType string) (map[string]string, error) {
	product, err := Product()
	if err != nil {
		return nil, err
	}
	res, err := KubectlCommand(
		"host",
		"get",
		"nodes "+
			fmt.Sprintf(
				`-o jsonpath='{range .items[*]}{.metadata.annotations.%s\.io/node-args}{end}'`,
				product),
	)
	if err != nil {
		return nil, err
	}

	nodeArgsMapSlice := processNodeArgs(res)

	for _, nodeArgsMap := range nodeArgsMapSlice {
		if nodeArgsMap["node-type"] == nodeType {
			return nodeArgsMap, nil
		}
	}

	return nil, nil
}

func processNodeArgs(nodeArgs string) (nodeArgsMapSlice []map[string]string) {
	nodeArgsSlice := strings.Split(nodeArgs, "]")

	for _, item := range nodeArgsSlice[:(len(nodeArgsSlice) - 1)] {
		items := strings.Split(item, `","`)
		nodeArgsMap := map[string]string{}

		for range items[1:] {
			nodeArgsMap["node-type"] = strings.Trim(items[0], `["`)
			regxCompile := regexp.MustCompile(`--|"`)

			for i := 1; i < len(items); i += 2 {
				if i < (len(items) - 1) {
					key := regxCompile.ReplaceAllString(items[i], "")
					value := regxCompile.ReplaceAllString(items[i+1], "")
					nodeArgsMap[key] = value
				}
			}
		}
		nodeArgsMapSlice = append(nodeArgsMapSlice, nodeArgsMap)
	}

	return nodeArgsMapSlice
}
