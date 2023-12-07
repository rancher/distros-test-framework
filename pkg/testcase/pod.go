package testcase

import (
	"strings"

	"github.com/rancher/distros-test-framework/pkg/assert"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/gomega"
)

const statusCompleted = "Completed"

// TestPodStatus test the status of the pods in the cluster using custom assert functions
func TestPodStatus(
	podAssertRestarts assert.PodAssertFunc,
	podAssertReady assert.PodAssertFunc,
	podAssertStatus assert.PodAssertFunc,
) {
	Eventually(func(g Gomega) {
		pods, err := shared.GetPods(false)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(pods).NotTo(BeEmpty())

		for _, pod := range pods {
			processPodStatus(g, pod, podAssertRestarts, podAssertReady, podAssertStatus)
		}
	}, "2500s", "10s").Should(Succeed())

	_, err := shared.GetPods(true)
	Expect(err).NotTo(HaveOccurred())
}

func processPodStatus(
	g Gomega,
	pod shared.Pod,
	podAssertRestarts, podAssertReady, podAssertStatus assert.PodAssertFunc,
) {
	if strings.Contains(pod.Name, "helm-install") {
		g.Expect(pod.Status).Should(Equal(statusCompleted), pod.Name)
	} else if strings.Contains(pod.Name, "apply") && strings.Contains(pod.NameSpace, "system-upgrade") {
		g.Expect(pod.Status).Should(SatisfyAny(
			ContainSubstring("Unknown"),
			ContainSubstring("Init:Error"),
			Equal(statusCompleted),
		), pod.Name)
	} else {
		g.Expect(pod.Status).Should(Equal(statusRunning), pod.Name)
		if podAssertRestarts != nil {
			podAssertRestarts(g, pod)
		}
		if podAssertReady != nil {
			podAssertReady(g, pod)
		}
		if podAssertStatus != nil {
			podAssertStatus(g, pod)
		}
	}
}
