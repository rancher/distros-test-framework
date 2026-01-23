package resources

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/avast/retry-go"

	"github.com/rancher/distros-test-framework/internal/provisioning/driver"
)

// Pod represents a Kubernetes pod.
type Pod struct {
	NameSpace      string
	Name           string
	Ready          string
	Status         string
	Restarts       string
	Age            string
	IP             string
	Node           string
	NominatedNode  string
	ReadinessGates string
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

// GetPods returns pods parsed from kubectl get pods.
func GetPodsForK3k(display bool, ip, kubeconfig, namespace string) ([]Pod, error) {
	var appendString string
	if namespace == "" {
		appendString = "-A"
	} else {
		appendString = "-n " + namespace
	}
	cmd := fmt.Sprintf("kubectl get pods %s -o wide --no-headers --kubeconfig=%s", appendString, kubeconfig)
	LogLevel("debug", "Running command: \n%s\n", cmd)
	res, err := RunCommandOnNode(cmd, ip)
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

// GetPods returns result string from kubectl get <resource> command.
// resourceType can be: pods, all, sc, pv, pvc, ingress, ns, etc.
func GetResourcesForK3k(display bool, ip, kubeconfig, namespace, resourceType string) (string, error) {
	var appendString string
	if namespace == "" {
		appendString = "-A"
	} else {
		appendString = "-n " + namespace
	}
	cmd := fmt.Sprintf("kubectl get %s %s -o wide --no-headers --kubeconfig=%s", resourceType, appendString, kubeconfig)
	LogLevel("debug", "Running command: \n%s\n", cmd)
	res, err := RunCommandOnNode(cmd, ip)
	if err != nil {
		return res, ReturnLogError("failed to get pods: %w\n", err)
	}

	// pods := ParsePods(res)
	if display {
		LogLevel("info", "\n\nCluster resource:\n")
		fmt.Println(res)
	}

	return res, nil
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
func ReadDataPod(cluster *driver.Cluster, namespace string) (string, error) {
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
func WriteDataPod(cluster *driver.Cluster, namespace string) (string, error) {
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

// GetPodsFromNamespace returns pods from a specific namespace.
func GetPodsFromNamespace(namespace string) ([]Pod, error) {
	cmd := fmt.Sprintf("kubectl get pods -n %s -o wide --kubeconfig=%s", namespace, KubeConfigFile)
	res, err := RunCommandHost(cmd)
	if err != nil {
		return nil, ReturnLogError("failed to get pods: %w\n", err)
	}

	return ParsePods(res), nil
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

// MonitorPodsStatus checks all pods are Running or Completed, waits/retries if any are Pending.
// Returns nil if all pods are healthy, error if any pod is in Error/CrashLoopBackOff status.
func MonitorPodsStatus(ip, kubeconfig, namespace string, retryInterval time.Duration, maxAttempts int) error {
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		pods, err := GetPodsForK3k(true, ip, kubeconfig, namespace)
		if err != nil {
			// Expect(err).NotTo(HaveOccurred())
			return ReturnLogError("failed to get pods: %w", err)
		}
		var pendingFound, failedFound bool
		var failedPods []string

		for _, pod := range pods {
			switch pod.Status {
			case "Running", "Completed":
				continue
			case "Pending", "ContainerCreating":
				pendingFound = true
			case "Error", "CrashLoopBackOff":
				if attempt == maxAttempts {
					LogLevel("error", "Pod %s/%s in %s state after max attempts.", pod.NameSpace, pod.Name, pod.Status)
					failedFound = true
					failedPods = append(failedPods, fmt.Sprintf("%s/%s: %s", pod.NameSpace, pod.Name, pod.Status))
				} else {
					LogLevel("warn", "Pod %s/%s in %s state, will retry.", pod.NameSpace, pod.Name, pod.Status)
				}
			default:
				// treat other statuses as not ready
				pendingFound = true
			}
		}

		if attempt == maxAttempts && failedFound {
			return ReturnLogError("Pods failed: %v", failedPods)
		}
		if !pendingFound && !failedFound {
			// All pods are Running or Completed
			return nil
		}
		LogLevel("info", "Some pods are Pending, waiting for %v before retrying (attempt %d/%d)", retryInterval, attempt, maxAttempts)
		time.Sleep(retryInterval)
	}

	return ReturnLogError("Timeout: Pods are still Pending after %d attempts", maxAttempts)
}

// DescribePod Runs 'kubectl describe pod' command and logs output.
func DescribePod(cluster *driver.Cluster, pod *Pod) {
	cmd := fmt.Sprintf("%s -n %s", pod.Name, pod.NameSpace)
	output, describeErr := KubectlCommand(cluster, "node", "describe", "pod", cmd)
	if describeErr != nil {
		LogLevel(
			"error", "error getting describe pod information for pod %s on namespace %s", pod.Name, pod.NameSpace)
	}
	if output != "" {
		LogLevel("debug", "Output for: $ kubectl describe pod %s -n %s is:\n %s", pod.Name, pod.NameSpace, output)
	}
}

// PodLogs Runs 'kubectl logs' command and logs output.
func PodLogs(cluster *driver.Cluster, pod *Pod) {
	if pod.NameSpace == "" || pod.Name == "" {
		LogLevel("warn", "Name or Namespace info in pod data is empty. kubectl logs cmd may not work")
	}
	cmd := fmt.Sprintf("%s -n %s", pod.Name, pod.NameSpace)
	output, logsErr := KubectlCommand(cluster, "node", "logs", "", cmd)
	if logsErr != nil {
		LogLevel(
			"error", "error getting logs for pod %s on namespace %s", pod.Name, pod.NameSpace)
	}
	if output != "" {
		LogLevel("debug", "Output for: $ kubectl logs %s -n %s is:\n %s", pod.Name, pod.NameSpace, output)
	}
}

// LogAllPodsForNamespace
// Given a namespace, this function:
// 1.  Filters ALL pods in the namespace.
// 2.  logs both 'kubectl describe pod' and 'kubectl logs' output for each pod in the namespace.
func LogAllPodsForNamespace(cluster *driver.Cluster, namespace string) {
	LogLevel("debug", "logging pod logs and describe pod output for all pods with namespace: %s", namespace)
	filters := map[string]string{
		"namespace": namespace,
	}
	pods, getErr := GetPodsFiltered(filters)
	if getErr != nil {
		LogLevel("error", "possibly no pods found with namespace: %s", namespace)
	}
	for i := range pods {
		if pods[i].NameSpace == "" {
			pods[i].NameSpace = namespace
		}
		PodLogs(cluster, &pods[i])
		DescribePod(cluster, &pods[i])
	}
}

// FindPodAndLog
// Search and log for a particular pod(s) given its unique name substring and namespace. Ex: coredns, kube-system.
// 1. Filter based on the name substring, and find the right pod(s).
// 2. For the pods matching the name, logs: 'kubectl describe pod' and 'kubectl logs' output.
// In the given example, it will filter all 'coredns' named pods in 'kube-system' namespace and log their outputs.
func FindPodAndLog(cluster *driver.Cluster, name, namespace string) {
	LogLevel("debug",
		"find and log(pod logs and describe pod) for pod starting with %s for namespace %s", name, namespace)
	filters := map[string]string{
		"namespace": namespace,
	}

	pods, getPodErr := GetPodsFiltered(filters)
	if getPodErr != nil {
		LogLevel("error", "error getting pods with namespace: %s", namespace)
	}
	for i := range pods {
		if strings.Contains(pods[i].Name, name) {
			if pods[i].NameSpace == "" {
				pods[i].NameSpace = namespace
			}
			PodLogs(cluster, &pods[i])
			DescribePod(cluster, &pods[i])
		}
	}
}
