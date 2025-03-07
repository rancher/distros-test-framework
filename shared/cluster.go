package shared

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/avast/retry-go"

	"github.com/rancher/distros-test-framework/config"
)

// ManageWorkload applies or deletes a workload based on the action: apply or delete.
func ManageWorkload(action string, workloads ...string) error {
	if action != "apply" && action != "delete" {
		return ReturnLogError("invalid action: %s. Must be 'apply' or 'delete'", action)
	}

	arch := os.Getenv("arch")
	resourceDir := BasePath() + "/workloads/" + arch

	files, readErr := os.ReadDir(resourceDir)
	if readErr != nil {
		return ReturnLogError("Unable to read resource manifest file for: %s\n with error:%w", resourceDir, readErr)
	}

	for _, workload := range workloads {
		if !fileExists(files, workload) {
			return ReturnLogError("workload %s not found", workload)
		}

		workloadErr := handleWorkload(action, resourceDir, workload)
		if workloadErr != nil {
			return workloadErr
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
	LogLevel("info", "Applying %s", workload)

	cmd := "kubectl apply -f " + filename + " --kubeconfig=" + KubeConfigFile
	out, err := RunCommandHost(cmd)
	fmt.Println(out)
	if err != nil || out == "" {
		if strings.Contains(out, "Invalid value") {
			return fmt.Errorf("failed to apply workload %s: %s", workload, out)
		}

		return ReturnLogError("failed to run kubectl apply: %w", err)
	}

	out, err = RunCommandHost("kubectl get all -A " + " --kubeconfig=" + KubeConfigFile)
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
	LogLevel("info", "Removing %s", workload)

	cmd := "kubectl delete -f " + filename + " --kubeconfig=" + KubeConfigFile
	_, err := RunCommandHost(cmd)
	if err != nil {
		return err
	}

	timeout := time.After(30 * time.Second)
	tick := time.NewTicker(2 * time.Second)

	for {
		select {
		case <-tick.C:
			res, err := RunCommandHost("kubectl get all -A " + " --kubeconfig=" + KubeConfigFile)
			if err != nil {
				return ReturnLogError("failed to run kubectl get all: %w\n", err)
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
// it already has KubeConfigFile
//
// destination = host or node
//
// action = get,describe...
//
// source = pods, node , exec, service ...
//
// args   = the rest of your command arguments.
func KubectlCommand(cluster *Cluster, destination, action, source string, args ...string) (string, error) {
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

// RestartCluster restarts the service on each node given by external IP.
func RestartCluster(product, ip string) error {
	_, err := RunCommandOnNode(fmt.Sprintf("sudo systemctl restart %s*", product), ip)
	if err != nil {
		return ReturnLogError("failed to restart %s: on ip: %s %v\n", product, ip, err)
	}

	return nil
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

// SonobuoyMixedOS Executes scripts/mixedos_sonobuoy.sh script.
// action	required install or cleanup sonobuoy plugin for mixed OS cluster.
// version	optional sonobouy version to be installed.
func SonobuoyMixedOS(action, version string) error {
	if action != "install" && action != "delete" {
		return ReturnLogError("invalid action: %s. Must be 'install' or 'delete'", action)
	}

	scriptsDir := BasePath() + "/scripts/mixedos_sonobuoy.sh"
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

// GetNodes returns nodes parsed from kubectl get nodes.
func GetNodes(display bool) ([]Node, error) {
	res, err := RunCommandHost("kubectl get nodes -o wide --no-headers --kubeconfig=" + KubeConfigFile)
	if err != nil {
		return nil, fmt.Errorf("failed to get nodes: %w", err)
	}

	nodes := ParseNodes(res)
	if display {
		LogLevel("info", "\n\nCluster nodes:\n")
		fmt.Println(res)
	}

	return nodes, nil
}

// GetNodesByRoles takes in one or multiple node roles and returns the slice of nodes that have those roles.
// Valid values for roles are: etcd, control-plane, worker.
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
			" --no-headers --kubeconfig=" + KubeConfigFile +
			" -l role-" + role
		res, err := RunCommandHost(cmd)
		if err != nil {
			return nil, err
		}

		matchedNodes = append(matchedNodes, ParseNodes(res)...)
	}

	for i := range matchedNodes {
		nodes = appendNodeIfMissing(nodes, &matchedNodes[i])
	}

	return nodes, nil
}

// ParseNodes returns nodes parsed from kubectl get nodes.
func ParseNodes(res string) []Node {
	nodes := make([]Node, 0, 10)
	nodeList := strings.Split(strings.TrimSpace(res), "\n")
	for _, rec := range nodeList {
		if strings.TrimSpace(rec) == "" {
			continue
		}

		fields := strings.Fields(rec)
		if len(fields) < 8 {
			continue
		}

		n := Node{
			Name:              fields[0],
			Status:            fields[1],
			Roles:             fields[2],
			Version:           fields[4],
			InternalIP:        fields[5],
			ExternalIP:        fields[6],
			OperationalSystem: fields[7],
		}
		nodes = append(nodes, n)
	}

	return nodes
}

// GetPods returns pods parsed from kubectl get pods.
func GetPods(display bool) ([]Pod, error) {
	cmd := "kubectl get pods -o wide --no-headers -A --kubeconfig=" + KubeConfigFile
	res, err := RunCommandHost(cmd)
	if err != nil {
		return nil, ReturnLogError("failed to get pods: %w\n", err)
	}

	pods := ParsePods(res)
	if display {
		LogLevel("info", "\n\nCluster pods:\n")
		fmt.Println(res)
	}

	return pods, nil
}

// GetPodsFiltered returns pods parsed from kubectl get pods with any specific filters.
// Example filters are: namespace, label, --field-selector.
func GetPodsFiltered(filters map[string]string) ([]Pod, error) {
	cmd := "kubectl get pods -o wide --no-headers --kubeconfig=" + KubeConfigFile
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

	pods := ParsePods(res)

	return pods, nil
}

// ParsePods parses the pods from the kubeclt get pods command.
func ParsePods(res string) []Pod {
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
		p.IP = fields[offset+5]
		p.Node = fields[offset+6]
		p.NominatedNode = fields[offset+7]
		p.ReadinessGates = fields[offset+8]

		pods = append(pods, p)
	}

	return pods
}

// ReadDataPod reads the data from the pod.
func ReadDataPod(cluster *Cluster, namespace string) (string, error) {
	podName, err := KubectlCommand(
		cluster,
		"host",
		"get",
		"pods",
		"-n "+namespace+" -o jsonpath={.items[0].metadata.name}",
	)
	if err != nil {
		LogLevel("error", "failed to fetch pod name: \n%w", err)
		os.Exit(1)
	}

	cmd := "kubectl exec -n local-path-storage " + podName + " --kubeconfig=" + KubeConfigFile +
		" -- cat /opt/data/test"

	res, err := RunCommandHost(cmd)
	if err != nil {
		return "", err
	}

	return res, nil
}

// WriteDataPod writes data to the pod.
func WriteDataPod(cluster *Cluster, namespace string) (string, error) {
	podName, err := KubectlCommand(
		cluster,
		"host",
		"get",
		"pods",
		"-n "+namespace+" -o jsonpath={.items[0].metadata.name}",
	)
	if err != nil {
		return "", ReturnLogError("failed to fetch pod name: \n%w", err)
	}

	cmd := "kubectl exec -n local-path-storage  " + podName + " --kubeconfig=" + KubeConfigFile +
		" -- sh -c 'echo testing local path > /opt/data/test' "

	return RunCommandHost(cmd)
}

// GetNodeArgsMap returns list of nodeArgs map.
func GetNodeArgsMap(cluster *Cluster, nodeType string) (map[string]string, error) {
	res, err := KubectlCommand(
		cluster,
		"host",
		"get",
		"nodes "+
			fmt.Sprintf(
				`-o jsonpath='{range .items[*]}{.metadata.annotations.%s\.io/node-args}{end}'`,
				cluster.Config.Product),
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

// DeleteNode deletes a node from the cluster filtering the name out by the IP.
func DeleteNode(ip string) error {
	if ip == "" {
		return ReturnLogError("must send a ip: %s\n", ip)
	}

	name, err := GetNodeNameByIP(ip)
	if err != nil {
		return ReturnLogError("failed to get node name by ip: %w\n", err)
	}

	_, delErr := RunCommandHost("kubectl delete node " + name + " --wait=false  --kubeconfig=" + KubeConfigFile)
	if delErr != nil {
		return ReturnLogError("failed to delete node: %w\n", delErr)
	}

	// delay not meant to wait if node is deleted.
	// but rather to give time for the node to be removed from the cluster.
	delay := time.After(20 * time.Second)
	<-delay

	return nil
}

// GetNodeNameByIP returns the node name by the given IP.
func GetNodeNameByIP(ip string) (string, error) {
	ticker := time.NewTicker(3 * time.Second)
	timeout := time.After(45 * time.Second)
	defer ticker.Stop()

	cmd := "kubectl get nodes -o custom-columns=NAME:.metadata.name,INTERNAL-IP:.status.addresses[*].address --kubeconfig=" +
		KubeConfigFile + " | grep " + ip + " | awk '{print $1}'"

	for {
		select {
		case <-timeout:
			return "", ReturnLogError("kubectl get nodes timed out for cmd: %s\n", cmd)
		case <-ticker.C:
			i := 0
			nodeName, err := RunCommandHost(cmd)
			if err != nil {
				i++
				LogLevel("warn", "error from RunCommandHost: %v\nwith res: %s  Retrying...", err, nodeName)
				if i > 5 {
					return "", ReturnLogError("kubectl get nodes returned error: %w\n", err)
				}

				continue
			}
			if nodeName == "" {
				continue
			}

			return strings.TrimSpace(nodeName), nil
		}
	}
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

func checkPodStatus() bool {
	pods, errGetPods := GetPods(false)
	if errGetPods != nil || len(pods) == 0 {
		LogLevel("debug", "Error getting pods. Retry.")
		return false
	}

	podReady := 0
	podNotReady := 0
	for i := range pods {
		if pods[i].Status == "Running" || pods[i].Status == "Completed" {
			podReady++
		} else {
			podNotReady++
			LogLevel("debug", "Pod Not Ready. Pod details: Name: %s Status: %s", pods[i].Name, pods[i].Status)
		}
	}

	if podReady+podNotReady != len(pods) {
		LogLevel("debug", "Length of pods %d != Ready pods: %d + Not Ready Pods: %d", len(pods), podReady, podNotReady)
	}
	if podNotReady == 0 {
		return true
	}

	return true
}

// WaitForPodsRunning Waits for pods to reach running state.
func WaitForPodsRunning(defaultTime time.Duration, attempts uint) error {
	return retry.Do(
		func() error {
			if !checkPodStatus() {
				return ReturnLogError("not all pods are ready yet")
			}
			return nil
		},
		retry.Attempts(attempts),
		retry.Delay(defaultTime),
		retry.OnRetry(func(n uint, _ error) {
			LogLevel("debug", "Attempt %d: Pods not ready, retrying...", n+1)
		}),
	)
}

// AddProductCfg its a helper function to add env config on this pkg.
func AddProductCfg() *config.Env {
	cfg, err := config.AddEnv()
	if err != nil {
		LogLevel("error", "error adding env vars: %w\n", err)
	}

	return cfg
}
