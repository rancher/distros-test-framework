package shared

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var (
	KubeConfigFile string
	AwsUser        string
	AccessKey      string
	Arch           string
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
	if action != "apply" && action != "delete" {
		return ReturnLogError("invalid action: %s. Must be 'apply' or 'delete'", action)
	}

	resourceDir := BasePath() + "/workloads/" + Arch

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
	cmd := "kubectl apply -f " + filename + " --kubeconfig=" + KubeConfigFile
	out, err := RunCommandHost(cmd)
	if err != nil || out == "" {
		return ReturnLogError("failed to run kubectl apply: %w", err)
	}

	out, err = RunCommandHost("kubectl get all -A --kubeconfig=" + KubeConfigFile)
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
			res, err := RunCommandHost("kubectl get all -A --kubeconfig=" + KubeConfigFile)
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
// it already has KubeConfigFile
//
// destination = host or node
//
// action = get,describe...
//
// source = pods, node , exec, service ...
//
// args   = the rest of your command arguments.
func KubectlCommand(destination, action, source string, args ...string) (string, error) {
	kubeconfigFlag := " --kubeconfig=" + KubeConfigFile
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

	cmd := cmdPrefix + " " + source + " " + strings.Join(args, " ") + kubeconfigFlag

	switch destination {
	case "host":
		return kubectlCmdOnHost(cmd)
	case "node":
		return kubectlCmdOnNode(cmd)
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

func kubectlCmdOnNode(cmd string) (string, error) {
	ips := FetchNodeExternalIP()
	var finalRes string

	for _, ip := range ips {
		res, err := RunCommandOnNode(cmd, ip)
		if err != nil {
			return "", err
		}
		finalRes += res
	}

	return finalRes, nil
}

// FetchClusterIP returns the cluster IP and port of the service.
func FetchClusterIP(namespace, serviceName string) (ip, port string, err error) {
	ip, err = RunCommandHost("kubectl get svc " + serviceName + " -n " + namespace +
		" -o jsonpath='{.spec.clusterIP}' --kubeconfig=" + KubeConfigFile)
	if err != nil {
		return "", "", ReturnLogError("failed to fetch cluster IP: %v\n", err)
	}

	port, err = RunCommandHost("kubectl get svc " + serviceName + " -n " + namespace +
		" -o jsonpath='{.spec.ports[0].port}' --kubeconfig=" + KubeConfigFile)
	if err != nil {
		return "", "", ReturnLogError("failed to fetch cluster port: %v\n", err)
	}

	return ip, port, err
}

// FetchServiceNodePort returns the node port of the service
func FetchServiceNodePort(namespace, serviceName string) (string, error) {
	cmd := "kubectl get service -n " + namespace + " " + serviceName + " --kubeconfig=" + KubeConfigFile +
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
		"--kubeconfig=" + KubeConfigFile)
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
			KubeConfigFile,
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
	cmd := "kubectl get nodes,pods -A -o wide --kubeconfig=" + KubeConfigFile
	res, err := RunCommandHost(cmd)
	if err != nil {
		_ = ReturnLogError("failed to print cluster state: %w\n", err)
	}
	fmt.Println("\n", res)
}

// GetNodes returns nodes parsed from kubectl get nodes.
func GetNodes(print bool) ([]Node, error) {
	res, err := RunCommandHost("kubectl get nodes -o wide --no-headers --kubeconfig=" + KubeConfigFile)
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

	for _, role := range roles {
		cmd := "kubectl get nodes -o wide --sort-by '{.metadata.name}'" +
			" --no-headers --kubeconfig=" + KubeConfigFile +
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
	cmd := "kubectl get pods -o wide --no-headers -A --kubeconfig=" + KubeConfigFile
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

// GetPodsByNamespace returns pods parsed from kubectl get pods in a specific namespace
func GetPodsByNamespace(namespace string, print bool) ([]Pod, error) {
	cmd := fmt.Sprintf("kubectl get pods -o wide --no-headers -n %s --kubeconfig=%s",
		namespace, KubeConfigFile)
	res, err := RunCommandHost(cmd)
	if err != nil {
		return nil, ReturnLogError("failed to get pods: %w\n", err)
	}

	pods := parsePods(res)
	if print {
		fmt.Println(res)
	}

	return pods, nil
}

// GetPodsByNamespaceAndLabel returns pods parsed from kubectl get pods in a specific namespace
// with a specific label
func GetPodsByNamespaceAndLabel(namespace, label string, print bool) ([]Pod, error) {
	cmd := fmt.Sprintf("kubectl get pods -o wide --no-headers -n %s -l %s --kubeconfig=%s",
		namespace, label, KubeConfigFile)
	res, err := RunCommandHost(cmd)
	if err != nil {
		return nil, ReturnLogError("failed to get pods: %w\n", err)
	}

	pods := parsePods(res)
	if print {
		fmt.Println(res)
	}

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

	cmd := "kubectl exec -n local-path-storage " + podName + " --kubeconfig=" + KubeConfigFile +
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

	cmd := "kubectl exec -n local-path-storage  " + podName + " --kubeconfig=" + KubeConfigFile +
		" -- sh -c 'echo testing local path > /data/test' "

	return RunCommandHost(cmd)
}
