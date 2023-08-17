package testcase

import (
	"fmt"
	"strings"

	"github.com/rancher/distros-test-framework/pkg/assert"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/gomega"
)

// TestPodStatus test the status of the pods in the cluster using 2 custom assert functions
func TestPodStatus(
	podAssertRestarts assert.PodAssertFunc,
	podAssertReady assert.PodAssertFunc,
	podAssertStatus assert.PodAssertFunc,
) {
	fmt.Printf("\nFetching pod status\n")
	Eventually(func(g Gomega) {
		pods, err := shared.ParsePods(false)
		g.Expect(err).NotTo(HaveOccurred())

		for _, pod := range pods {
			processPodStatus(g, pod, podAssertRestarts, podAssertReady, podAssertStatus)
		}
	}, "1000s", "5s").Should(Succeed())

	_, err := shared.ParsePods(true)
	Expect(err).NotTo(HaveOccurred())
}

func processPodStatus(
	g Gomega,
	pod shared.Pod,
	podAssertRestarts, podAssertReady, podAssertStatus assert.PodAssertFunc,
) {
	if strings.Contains(pod.Name, "helm-install") {
		g.Expect(pod.Status).Should(Equal("Completed"), pod.Name)
	} else if strings.Contains(pod.Name, "apply") && strings.Contains(pod.NameSpace, "system-upgrade") {
		g.Expect(pod.Status).Should(SatisfyAny(
			ContainSubstring("Unknown"),
			ContainSubstring("Init:Error"),
			Equal("Completed"),
		), pod.Name)
	} else {
		g.Expect(pod.Status).Should(Equal(Running), pod.Name)
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
