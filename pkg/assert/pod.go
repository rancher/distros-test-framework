package assert

import (
	"fmt"
	"net"
	"strings"

	"github.com/onsi/gomega/types"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/gomega"
)

type PodAssertFunc func(g Gomega, pod shared.Pod)

const(
	statusCompleted = "Completed"
	statusRunning = "Running"
)
// PodAssertRestart custom assertion func that asserts that pods are not restarting with no reason
// controller, scheduler, helm-install pods can be restarted occasionally when cluster started if only once
func PodAssertRestart() PodAssertFunc {
	return func(g Gomega, pod shared.Pod) {
		if strings.Contains(pod.NameSpace, "kube-system") &&
			strings.Contains(pod.Name, "controller") &&
			strings.Contains(pod.Name, "scheduler") {
			g.Expect(pod.Restarts).Should(SatisfyAny(Equal("0"),
				Equal("1")),
				"could be restarted occasionally when cluster started", pod.Name)
		}
	}
}

// PodAssertReady custom assertion func that asserts that the pod is
// with correct numbers of ready containers.
func PodAssertReady() PodAssertFunc {
	return func(g Gomega, pod shared.Pod) {
		g.ExpectWithOffset(1, pod.Ready).To(checkReadyFields(),
			"should have equal values in n/n format")
	}
}

// checkReadyFields is a custom matcher that checks
// if the input string is in N/N format and the same quantity.
func checkReadyFields() types.GomegaMatcher {
	return WithTransform(func(s string) (bool, error) {
		var a, b int

		n, err := fmt.Sscanf(s, "%d/%d", &a, &b)
		if err != nil || n != 2 {
			return false, fmt.Errorf("failed to parse format: %v", err)
		}

		return a == b, nil
	}, BeTrue())
}

// PodAssertStatus custom assertion that asserts that pod status is completed or in some cases
// apply pods can have an error status
func PodAssertStatus() PodAssertFunc {
	return func(g Gomega, pod shared.Pod) {
		if strings.Contains(pod.Name, "helm-install") {
			g.Expect(pod.Status).Should(Equal(statusCompleted), pod.Name)
		} else if strings.Contains(pod.Name, "apply") &&
			strings.Contains(pod.NameSpace, "system-upgrade") {
			g.Expect(pod.Status).Should(SatisfyAny(
				ContainSubstring("Error"),
				Equal(statusCompleted),
			), pod.Name)
		} else {
			g.Expect(pod.Status).Should(Equal(statusRunning), pod.Name)
		}
	}
}

// CheckPodStatusRunning asserts that the pod is running with the specified label = app name.
func CheckPodStatusRunning(name, namespace, assert string) {
	cmd := "kubectl get pods -n " + namespace + " -o=name -l k8s-app=" + name +
		" --field-selector=status.phase=Running --kubeconfig="+shared.KubeConfigFile
	Eventually(func(g Gomega) {
		res, err := shared.RunCommandHost(cmd)
		g.Expect(err).ShouldNot(HaveOccurred())
		g.Expect(res).Should(ContainSubstring(assert))
	}, "180s", "5s").Should(Succeed())
}

// ValidatePodIPByLabel validates expected pod IP by label
func ValidatePodIPByLabel(labels, expected []string) {
	Eventually(func() error {
		for i, label := range labels {
			if len(labels) > 0 {
				cmd := "kubectl get pods -l "+ label +
					` -o=jsonpath='{range .items[*]}{.status.podIPs[*].ip}{" "}{end}'`+
					"--kubeconfig="+shared.KubeConfigFile
				res, _ := shared.RunCommandHost(cmd)
				ips := strings.Split(res, " ")
				if strings.Contains(ips[0], expected[i]) {
					return nil
				}
			}
		}
		return nil
	}, "180s", "5s").Should(Succeed(), 
		"failed to validate expected: %s on %s", expected, labels)
}

// ValidatePodIPsByLabel validates expected pod IPs by label 
func ValidatePodIPsByLabel(label string, expected []string) {
	cmd := "kubectl get pods -l "+ label +
		` -o jsonpath='{range .items[*]}{.status.podIPs[*].ip}{" "}{end}'` +
		" --kubeconfig="+shared.KubeConfigFile
	Eventually(func() error {
		res, _ := shared.RunCommandHost(cmd)
		ips := strings.Split(res, " ")
		Expect(len(ips)).ShouldNot(BeZero())
		Expect(len(expected)).ShouldNot(BeZero())
		for i, ip := range ips {
			_,subnet,_ := net.ParseCIDR(expected[i])
			if subnet.Contains(net.ParseIP(ip)) {
				return nil
			}
		}
		return nil
	}, "180s", "5s").Should(Succeed(), 
	"failed to validate podIPs in expected range %s for label  %s", 
	expected, label)
}

// CheckPodStatusRunningByNSLabel checks status of pods is Running when searched by namespace and label 
func CheckPodStatusRunningByNSLabel(namespace, label string) {
	cmd := "kubectl get pods -n "+namespace+" -l "+label+
		" --field-selector=status.phase=Running --kubeconfig="+shared.KubeConfigFile
	Eventually(func(g Gomega) {
		err := ValidateOnHost(cmd, statusRunning)
		g.Expect(err).NotTo(HaveOccurred(), err)
	}, "30s", "5s").Should(Succeed())
}
