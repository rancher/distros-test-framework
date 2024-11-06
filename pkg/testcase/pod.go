package testcase

import (
	"fmt"
	"strings"

	"github.com/rancher/distros-test-framework/pkg/assert"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/gomega"
)

const statusCompleted = "Completed"

var (
	ciliumPodsRunning    = 0
	ciliumPodsNotRunning = 0
)

// TestPodStatus test the status of the pods in the cluster using custom assert functions.
func TestPodStatus(
	cluster *shared.Cluster,
	podAssertRestarts,
	podAssertReady assert.PodAssertFunc,
) {
	Eventually(func(g Gomega) {
		pods, err := shared.GetPods(false)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(pods).NotTo(BeEmpty())

		for i := range pods {
			processPodStatus(cluster, g, &pods[i], podAssertRestarts, podAssertReady)
		}
	}, "2400s", "10s").Should(Succeed(), "failed to process pods status")

	_, err := shared.GetPods(true)
	Expect(err).NotTo(HaveOccurred())
}

// TestAirgapClusterPodStatus test the status of the pods in the private cluster using custom assert functions.
func TestAirgapClusterPodStatus(
	cluster *shared.Cluster,
	podAssertRestarts,
	podAssertReady assert.PodAssertFunc,
) {
	var podDetails string
	Eventually(func(g Gomega) {
		podDetails = getPrivatePods(cluster)
		pods := shared.ParsePods(podDetails)
		g.Expect(pods).NotTo(BeEmpty())

		for i := range pods {
			processPodStatus(cluster, g, &pods[i], podAssertRestarts, podAssertReady)
		}
	}, "2400s", "10s").Should(Succeed(), "\nfailed to process pods status\n%v\n", podDetails)
}

func getPrivatePods(cluster *shared.Cluster) (podDetails string) {
	cmd := fmt.Sprintf(
		"PATH=$PATH:/var/lib/rancher/%[1]v/bin:/opt/%[1]v/bin; "+
			"KUBECONFIG=/etc/rancher/%[1]v/%[1]v.yaml ", cluster.Config.Product)
	cmd += "kubectl get pods -A -o wide --no-headers"
	podDetails, _ = shared.CmdForPrivateNode(cluster, cmd, cluster.ServerIPs[0])

	return podDetails
}

func processPodStatus(
	cluster *shared.Cluster,
	g Gomega,
	pod *shared.Pod,
	podAssertRestarts, podAssertReady assert.PodAssertFunc,
) {
	var ciliumPod bool

	switch {
	case strings.Contains(pod.Name, "helm-install") ||
		strings.Contains(pod.Name, "helm-delete") ||
		strings.Contains(pod.Name, "helm-operation"):
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
			shared.LogLevel("warn", "no cilium pods running yet.")
		case ciliumPodsRunning == 0 && ciliumPodsNotRunning > 1:
			shared.LogLevel("error", "no cilium pods running, only pending: Name:%s,Status:%s", pod.Name, pod.Status)
			return
		default:
			Expect(ciliumPodsRunning).To(BeNumerically(">=", 1), "no cilium pods running",
				pod.Name, pod.Status)
		}
	}
}

func processCiliumStatus(pod *shared.Pod) {
	if strings.Contains(pod.Status, "Pending") {
		Expect(pod.Ready).To(Equal("0/1"), pod.Name, pod.Status)
		ciliumPodsNotRunning++
	} else if strings.Contains(pod.Status, statusRunning) {
		Expect(pod.Ready).To(Equal("1/1"), pod.Name, pod.Status)
		ciliumPodsRunning++
	}
}
