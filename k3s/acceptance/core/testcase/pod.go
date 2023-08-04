package testcase

import (
	"fmt"
	"strings"

	"github.com/rancher/distros-test-framework/k3s/acceptance/core/service/assert"
	"github.com/rancher/distros-test-framework/k3s/acceptance/shared"

	. "github.com/onsi/gomega"
)

// TestPodStatus test the status of the pods in the cluster using 2 custom assert functions
func TestPodStatus(
	podAssertRestarts assert.PodAssertFunc,
	podAssertReady assert.PodAssertFunc,
	podAssertStatus assert.PodAssertFunc,

) {
	fmt.Println("\nFetching pod status")

	Eventually(func(g Gomega) {
		pods, err := shared.ParsePods(false)
		g.Expect(err).NotTo(HaveOccurred())

		for _, pod := range pods {
			if strings.Contains(pod.Name, "helm-install") {
				g.Expect(pod.Status).Should(Equal("Completed"), pod.Name)
			} else if strings.Contains(pod.Name, "apply") {
				g.Expect(pod.Status).Should(SatisfyAny(
					ContainSubstring("Error"),
					Equal("Completed"),
				), pod.Name)
			} else {
				g.Expect(pod.Status).Should(Equal("Running"), pod.Name)
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
	}, "600s", "5s").Should(Succeed())
}
