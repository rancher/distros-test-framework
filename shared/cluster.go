package shared

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var (
	KubeConfigFile string
	AwsUser        string
	AccessKey      string
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
	NodeIP    string
	Node      string
}

// ManageWorkload creates or deletes a workload based on the action: create or delete.
func ManageWorkload(action, workload, arch string) (string, error) {
	var res string
	var err error

	if action != "create" && action != "delete" {
		return "", ReturnLogError("invalid action: %s. Must be 'create' or 'delete'", action)
	}

	resourceDir := BasePath() + "/distros-test-framework/workloads/amd64/"
	if arch == "arm64" {
		resourceDir = BasePath() + "/distros-test-framework/workloads/arm/"
	}

	files, err := os.ReadDir(resourceDir)
	if err != nil {
		return "", ReturnLogError("%s : Unable to read resource manifest file for %s\n", err, workload)
	}

	for _, f := range files {
		filename := filepath.Join(resourceDir, f.Name())
		if strings.TrimSpace(f.Name()) == workload {
			if action == "create" {
				res, err = createWorkload(workload, filename)
				if err != nil {
					return "", ReturnLogError("failed to create workload %s: %s\n", workload, err)
				}
			} else {
				err = deleteWorkload(workload, filename)
				if err != nil {
					LogLevel("warn", "failed to delete workload %s: %s\n", workload, err)
					return "", err
				}
			}

			return res, err
		}
	}

	return "", ReturnLogError("workload %s not found", workload)
}

func createWorkload(workload, filename string) (string, error) {
	fmt.Println("\nDeploying", workload)
	cmd := "kubectl apply -f " + filename + " --kubeconfig=" + KubeConfigFile

	return RunCommandHost(cmd)
}

// deleteWorkload deletes a workload and asserts that the workload is deleted.
func deleteWorkload(workload, filename string) error {
	fmt.Println("\nRemoving", workload)
	cmd := "kubectl delete -f " + filename + " --kubeconfig=" + KubeConfigFile

	_, err := RunCommandHost(cmd)
	if err != nil {
		return err
	}

	timeout := time.After(60 * time.Second)
	tick := time.NewTicker(5 * time.Second)

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
			return ReturnLogError("workload deletion timed out")
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
		return "", err
	}

	return res, nil
}

func kubectlCmdOnNode(cmd string) (string, error) {
	ips := FetchNodeExternalIP()
	for _, ip := range ips {
		res, err := RunCommandOnNode(cmd, ip)
		if err != nil {
			return "", err
		}
		return res, nil
	}
	return "", nil
}

// FetchClusterIP returns the cluster IP and port of the service.
func FetchClusterIP(namespace string, serviceName string) (string, string, error) {
	ip, err := RunCommandHost("kubectl get svc " + serviceName + " -n " + namespace +
		" -o jsonpath='{.spec.clusterIP}' --kubeconfig=" + KubeConfigFile)
	if err != nil {
		return "", "", ReturnLogError("failed to fetch cluster IP: %v\n", err)
	}

	port, err := RunCommandHost("kubectl get svc " + serviceName + " -n " + namespace +
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
	time.Sleep(40 * time.Second)
}

// FetchIngressIP returns the ingress IP of the given namespace
func FetchIngressIP(namespace string) ([]string, error) {
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
	ingressIPs := strings.Split(ingressIP, " ")

	return ingressIPs, nil
}

// ParseNodes returns nodes parsed from kubectl get nodes.
func ParseNodes(print bool) ([]Node, error) {
	nodes := make([]Node, 0, 10)

	res, err := RunCommandHost("kubectl get nodes --no-headers -o wide --kubeconfig=" + KubeConfigFile)
	if err != nil {
		return nil, fmt.Errorf("failed to run kubectl get nodes: %v\n", err)
	}

	nodelist := strings.TrimSpace(res)
	split := strings.Split(nodelist, "\n")
	for _, rec := range split {
		if strings.TrimSpace(rec) != "" {
			fields := strings.Fields(rec)
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
	}
	if print {
		fmt.Println(nodelist)
	}

	return nodes, nil
}

// ParsePods returns pods parsed from kubectl get pods.
func ParsePods(print bool) ([]Pod, error) {
	pods := make([]Pod, 0, 10)

	cmd := "kubectl get pods -o wide --no-headers -A --kubeconfig=" + KubeConfigFile
	res, err := RunCommandHost(cmd)
	if err != nil {
		return nil, err
	}

	podList := strings.TrimSpace(res)
	split := strings.Split(podList, "\n")
	for _, rec := range split {
		fields := strings.Fields(rec)
		p := Pod{
			NameSpace: fields[0],
			Name:      fields[1],
			Ready:     fields[2],
			Status:    fields[3],
			Restarts:  fields[4],
			NodeIP:    fields[6],
			Node:      fields[7],
		}
		pods = append(pods, p)
	}
	if print {
		fmt.Println(podList)
	}

	return pods, nil
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
		return "", ReturnLogError("failed to fetch pod name: %v\n", err)
	}

	cmd := "kubectl exec -n local-path-storage " + podName + " --kubeconfig=" + KubeConfigFile +
		" -- cat /data/test"
	return RunCommandHost(cmd)
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
		return "", ReturnLogError("failed to fetch pod name: %v\n", err)
	}

	cmd := "kubectl exec -n local-path-storage  " + podName + " --kubeconfig=" + KubeConfigFile +
		" -- sh -c 'echo testing local path > /data/test' "

	return RunCommandHost(cmd)
}
