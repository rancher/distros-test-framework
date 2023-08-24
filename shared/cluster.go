package shared

import (
	"errors"
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
	NodeIP    string
	Node      string
}

// ManageWorkload applies or deletes a workload based on the action: apply or delete.
func ManageWorkload(action string, workloads ...string) (string, error) {
	if action != "apply" && action != "delete" {
		return "", fmt.Errorf("invalid action: %s. Must be 'apply' or 'delete'", action)
	}
	
	var res string
	var err error

	resourceDir := BasePath() + "/distros-test-framework/workloads/" + Arch
	
	files, err := os.ReadDir(resourceDir)
	for _, workload := range workloads {
		if !fileExists(files, workload) {
			return "", fmt.Errorf("workload %s not found in %s", workload, resourceDir)
		}
		filename := filepath.Join(resourceDir, workload)
		if action == "apply" {
			err = applyWorkload(workload, filename)
			if err != nil {
				return "", fmt.Errorf("failed to apply workload %s: %s", workload, err)
			}
		} else {
			err = deleteWorkload(workload, filename)
			if err != nil {
				return "", fmt.Errorf("failed to delete workload %s: %s", workload, err)
			}
		}
	}

	return res, err
}

// applyWorkload applies a workload to the cluster.
func applyWorkload(workload, filename string) (error) {
	fmt.Println("\nApplying ", workload)
	
	cmd := "kubectl apply -f " + filename + " --kubeconfig=" + KubeConfigFile
	out, err := RunCommandHost(cmd)
	if err != nil || out == "" {
		return fmt.Errorf("failed to run kubectl apply: %v", err)
	}

	out, err = RunCommandHost("kubectl get all -A --kubeconfig=" + KubeConfigFile)
	if err != nil {
		return err
	}

	isApplied := !strings.Contains(out, "Creating") && strings.Contains(out, workload)
	if isApplied {
		return nil
	} 

	return err
}

// deleteWorkload deletes a workload and asserts that the workload is deleted.
func deleteWorkload(workload, filename string) error {
	fmt.Println("\nRemoving", workload)
	cmd := "kubectl delete -f " + filename + " --kubeconfig=" + KubeConfigFile

	_, err := RunCommandHost(cmd)
	if err != nil {
		return fmt.Errorf("failed to run kubectl delete: %v", err)
	}

	timeout := time.After(30 * time.Second)
	tick := time.Tick(2 * time.Second)

	for {
		select {
		case <-timeout:
			return errors.New("workload delete timed out")
		case <-tick:
			res, err := RunCommandHost("kubectl get all -A --kubeconfig=" + KubeConfigFile)
			if err != nil {
				return err
			}
			isDeleted := !strings.Contains(res, workload)
			if isDeleted {
				return nil
			}
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

	var res string
	var err error
	if destination == "host" {
		res, err = RunCommandHost(cmd)
		if err != nil {
			return "", err
		}
	} else if destination == "node" {
		ips := FetchNodeExternalIP()
		for _, ip := range ips {
			res, err = RunCommandOnNode(cmd, ip)
			if err != nil {
				return "", err
			}
		}
	} else {
		return "", fmt.Errorf("invalid destination: %s", destination)
	}

	return res, nil
}

// FetchClusterIP returns the cluster IP and port of the service.
func FetchClusterIP(namespace string, serviceName string) (string, string, error) {
	ip, err := RunCommandHost("kubectl get svc " + serviceName + " -n " + namespace +
		" -o jsonpath='{.spec.clusterIP}' --kubeconfig=" + KubeConfigFile)
	if err != nil {
		return "", "", err
	}

	port, err := RunCommandHost("kubectl get svc " + serviceName + " -n " + namespace +
		" -o jsonpath='{.spec.ports[0].port}' --kubeconfig=" + KubeConfigFile)
	if err != nil {
		return "", "", err
	}

	return ip, port, err
}

// FetchServiceNodePort returns the node port of the service
func FetchServiceNodePort(namespace, serviceName string) (string, error) {
	cmd := "kubectl get service -n " + namespace + " " + serviceName + " --kubeconfig=" + KubeConfigFile +
		" --output jsonpath=\"{.spec.ports[0].nodePort}\""
	nodeport, err := RunCommandHost(cmd)
	if err != nil {
		return "", err
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
func RestartCluster(product, ip string) (string, error) {
	return RunCommandOnNode(fmt.Sprintf("sudo systemctl restart %s*", product), ip)
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
		return nil, err
	}

	ingressIP := strings.Trim(res, " ")
	if ingressIP == "" {
		return nil, nil
	}
	ingressIPs := strings.Split(ingressIP, " ")

	return ingressIPs, nil
}

// SonobuoyMixedOS Executes scripts/mixedos_sonobuoy.sh script
// action	required install or cleanup sonobuoy plugin for mixed OS cluster
// version	optional sonobouy version to be installed
func SonobuoyMixedOS(action, version string) error{
	if action != "install" && action != "delete" {
		return fmt.Errorf("invalid action: %s. Must be 'install' or 'delete'", action)
	}

	scriptsDir := BasePath() + "/distros-test-framework/scripts/mixedos_sonobuoy.sh"
	err := os.Chmod(scriptsDir, 0755)
	if err != nil {
		return fmt.Errorf("failed to change script permissions: %v", err)
	}

	cmd := exec.Command("/bin/sh", scriptsDir, action, version)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to execute %s action sonobuoy: %v\nOutput: %s", action, err, output)
	}

	return err
}

// ParseNodes returns nodes parsed from kubectl get nodes.
func ParseNodes(print bool) ([]Node, error) {
	nodes := make([]Node, 0, 10)

	res, err := RunCommandHost("kubectl get nodes --no-headers -o wide --kubeconfig=" + KubeConfigFile)
	if err != nil {
		return nil, err
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
		return "", err
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
		return "", err
	}

	cmd := "kubectl exec -n local-path-storage  " + podName + " --kubeconfig=" + KubeConfigFile +
		" -- sh -c 'echo testing local path > /data/test' "

	return RunCommandHost(cmd)
}
