package testcase

import (
	"strings"
	"time"

	. "github.com/onsi/gomega"
	"github.com/rancher/distros-test-framework/internal/pkg/assert"
	"github.com/rancher/distros-test-framework/internal/pkg/testcase/support"
	"github.com/rancher/distros-test-framework/internal/provisioning/driver"
	"github.com/rancher/distros-test-framework/internal/resources"
)

const statusCompleted = "Completed"

var (
	ciliumPodsRunning    = 0
	ciliumPodsNotRunning = 0
)

// TestPodStatus test the status of the pods in the cluster using custom assert functions.
func TestPodStatus(
	cluster *driver.Cluster,
	podAssertRestarts,
	podAssertReady assert.PodAssertFunc,
) {
	cmd := "kubectl get pods -A --field-selector=status.phase!=Running | " +
		"kubectl get pods -A --field-selector=status.phase=Pending"
	Eventually(func(g Gomega) bool {
		pods, err := resources.GetPods(false)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(pods).NotTo(BeEmpty())

		res, _ := resources.RunCommandHost(cmd + " --kubeconfig=" + resources.KubeConfigFile)
		if res != "" {
			resources.LogLevel("debug", "Waiting for pod status to be Running or Completed... \n%s", res)
		}
		for i := range pods {
			processPodStatus(cluster, g, &pods[i], podAssertRestarts, podAssertReady)
		}

		return true
	}, "1000s", "10s").Should(BeTrue(), "Pods are not in desired state")

	_, err := resources.GetPods(true)
	Expect(err).NotTo(HaveOccurred())
}

// TestPodStatusUsingBastion test the status of the pods in the private cluster using custom assert functions.
func TestPodStatusUsingBastion(
	cluster *driver.Cluster,
	podAssertRestarts,
	podAssertReady assert.PodAssertFunc,
) {
	var podDetails string
	Eventually(func(g Gomega) {
		podDetails = support.GetPodsViaBastion(cluster)
		pods := resources.ParsePods(podDetails)
		g.Expect(pods).NotTo(BeEmpty())

		for i := range pods {
			processPodStatus(cluster, g, &pods[i], podAssertRestarts, podAssertReady)
		}
	}, "600s", "10s").Should(Succeed(), "\nfailed to process pods status\n%v\n", podDetails)
}

func processPodStatus(
	cluster *driver.Cluster,
	g Gomega,
	pod *resources.Pod,
	podAssertRestarts, podAssertReady assert.PodAssertFunc,
) {
	var ciliumPod bool

	switch {
	case strings.Contains(pod.Name, "helm-install") ||
		strings.Contains(pod.Name, "helm-delete") ||
		strings.Contains(pod.Name, "helm-operation") ||
		strings.Contains(pod.Name, "nvidia-cuda-validator-") ||
		strings.Contains(pod.Name, "-benchmark"):
		g.Expect(pod.Status).Should(Equal(statusCompleted), pod.Name)

	case strings.Contains(pod.Name, "apply") && strings.Contains(pod.NameSpace, "system-upgrade"):
		g.Expect(pod.Status).Should(SatisfyAny(
			ContainSubstring("Unknown"),
			ContainSubstring("Init:Error"),
			Equal(statusCompleted),
		), pod.Name)

	case strings.Contains(pod.Name, "cilium-operator") && cluster.Config.Product == "rke2" &&
		cluster.NumServers == 1 && cluster.NumAgents == 0:
		processCiliumStatus(pod)
		ciliumPod = true

	default:
		g.Expect(pod.Status).Should(Equal(statusRunning), pod.Name)

		if podAssertRestarts != nil {
			podAssertRestarts(g, *pod)
		}
		if podAssertReady != nil {
			podAssertReady(g, *pod)
		}
	}

	// at least one cilium pod should be running.
	if ciliumPod {
		switch {
		case ciliumPodsRunning == 0 && ciliumPodsNotRunning == 1:
			resources.LogLevel("warn", "no cilium pods running yet.")
		case ciliumPodsRunning == 0 && ciliumPodsNotRunning > 1:
			resources.LogLevel("error", "no cilium pods running, only pending: Name:%s,Status:%s", pod.Name, pod.Status)
			return
		default:
			Expect(ciliumPodsRunning).To(BeNumerically(">=", 1), "no cilium pods running",
				pod.Name, pod.Status)
		}
	}
}

func processCiliumStatus(pod *resources.Pod) {
	if strings.Contains(pod.Status, "Pending") {
		Expect(pod.Ready).To(Equal("0/1"), pod.Name, pod.Status)
		ciliumPodsNotRunning++
	} else if strings.Contains(pod.Status, statusRunning) {
		Expect(pod.Ready).To(Equal("1/1"), pod.Name, pod.Status)
		ciliumPodsRunning++
	}
}

func TestPodStatusForK3k(
	host *driver.HostCluster,
	podAssertRestarts,
	podAssertReady assert.PodAssertFunc,
	kubectlPath string,
	kubeconfig string,
	namespace string,
	k3kClusterName string,
) {
	retryErr := resources.MonitorPodsStatus(host.ServerIP, kubeconfig, namespace, 30*time.Second, 10)
	Expect(retryErr).NotTo(HaveOccurred())
}
