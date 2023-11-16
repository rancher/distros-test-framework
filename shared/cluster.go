package shared

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	NameSpace string
	Name      string
	Ready     string
	Status    string
	Restarts  string
	Age       string
	NodeIP    string
	Node      string
}

// ManageWorkload applies or deletes a workload based on the action: apply or delete.
func ManageWorkload(action string, workloads ...string) (string, error) {
	if action != "apply" && action != "delete" {
		return "", ReturnLogError("invalid action: %s. Must be 'apply' or 'delete'", action)
	}

	resourceDir := BasePath() + "/distros-test-framework/workloads/" + Arch

	files, err := os.ReadDir(resourceDir)
	if err != nil {
		return "", ReturnLogError("Unable to read resource manifest file for: %s\n", resourceDir)
	}

	for _, workload := range workloads {
		if !fileExists(files, workload) {
			return "", ReturnLogError("workload %s not found", workload)
		}

		err := handleWorkload(action, resourceDir, workload)
		if err != nil {
			return "", err
		}
	}

	return "", nil
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

	scriptsDir := BasePath() + "/distros-test-framework/scripts/mixedos_sonobuoy.sh"
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
		fmt.Println(res)
	}

	return pods, nil
}

// parsePods parses the pods from the kubeclt get pods command.
func parsePods(res string) []Pod {
	pods := make([]Pod, 0, 10)
	podList := strings.Split(strings.TrimSpace(res), "\n")

	for _, rec := range podList {
		fields := strings.Fields(rec)
		if strings.TrimSpace(rec) == "" {
			continue
		}

		if len(fields) < 9 {
			continue
		}

		p := Pod{
			NameSpace: fields[0],
			Name:      fields[1],
			Ready:     fields[2],
			Status:    fields[3],
			Restarts:  fields[4],
			Age:       fields[5],
			NodeIP:    fields[6],
			Node:      fields[7],
		}
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

type ServiceType string

const (
	Server ServiceType = "server"
	Agent  ServiceType = "agent"
)

func (s ServiceType) IsValidServiceType() bool {
	valid := []ServiceType{Server, Agent}
	for _, i := range valid {
		if s == i {
			return true
		}
	}

	return false
}

type ServiceAction string

const (
	Stop    ServiceAction = "stop"
	Start   ServiceAction = "start"
	Restart ServiceAction = "restart"
	Status  ServiceAction = "status"
)

func (s ServiceAction) IsValidServiceAction() bool {
	valid := []ServiceAction{Stop, Start, Restart, Status}
	for _, i := range valid {
		if s == i {
			return true
		}
	}

	return false
}

func (s ServiceAction) isStartAction() bool {
	startAction := []ServiceAction{Start, Restart}
	for _, i := range startAction {
		if s == i {
			return true
		}
	}

	return false
}

// func (s ServiceAction) getServiceActionCmd(serviceName string) string {
// 	if s.isStartAction() {
// 		return fmt.Sprintf("timeout 2m sudo systemctl --no-block %s %s; sleep 60", s, serviceName)
// 	}
// 	return fmt.Sprintf("timeout 2m sudo systemctl --no-block %s %s", s, serviceName)
// }

type Product string

const (
	K3S  Product = "k3s"
	RKE2 Product = "rke2"
)

func (p Product) IsValidProduct() bool {
	valid := []Product{K3S, RKE2}
	for _, i := range valid {
		if p == i {
			return true
		}
	}

	return false
}

func (p Product) getString() string {
	if p == K3S {
		return "k3s"
	}

	return "rke2"
}

// getServiceName Get service name. Used to work with stop/start k3s/rke2 services
func (product Product) getServiceName(serviceType ServiceType) string {
	if !serviceType.IsValidServiceType() {
		ReturnLogError("serviceType is not valid; has to be set to ONLY: server | agent")
	}
	var serviceName string
	if product == K3S && serviceType == Server {
		serviceName = product.getString() // k3s
	} else { // k3s-agent, rke2-server, rke2-agent
		serviceName = fmt.Sprintf("%s-%s", product, serviceType)
	}

	return serviceName
}

func (product Product) GetServiceCmd(action ServiceAction, serviceType ServiceType) string {
	if action.isStartAction() {
		return fmt.Sprintf("timeout 2m sudo systemctl --no-block %s %s; sleep 60", action, product.getServiceName(serviceType))
	}
	return fmt.Sprintf("timeout 2m sudo systemctl --no-block %s %s", action, product.getServiceName(serviceType))
}

// ManageClusterService action:stop/start/restart/status product:rke2/k3s ips:ips array for serviceType:agent/server
func (product Product) ManageClusterService(action ServiceAction, serviceType ServiceType, ips []string) error {
	if !product.IsValidProduct() {
		ReturnLogError("product needs to be one of: k3s | rke2")
	}
	if !action.IsValidServiceAction() {
		ReturnLogError("action needs to be one of: start | stop | restart | status")
	}
	if !serviceType.IsValidServiceType() {
		ReturnLogError("serviceType needs to be one of: server | agent")
	}

	for _, ip := range ips {
		cmd := product.GetServiceCmd(action, serviceType)
		_, err := RunCommandOnNode(cmd, ip)
		if err != nil {
			return err
		}
	}

	return nil
}

// CertRotate certificate rotate for k3s or rke2
func (product Product) CertRotate(ips []string) (string, error) {
	if !product.IsValidProduct() {
		ReturnLogError("Product needs to be one of: k3s | rke2")
	}
	for _, ip := range ips {
		cmd := fmt.Sprintf("sudo %s certificate rotate", product)
		_, err := RunCommandOnNode(cmd, ip)
		if err != nil {
			return ip, err
		}
	}

	return "", nil
}
