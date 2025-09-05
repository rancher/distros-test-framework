package testcase

import (
	"sort"
	"strings"

	"github.com/rancher/distros-test-framework/internal/pkg/assert"
	"github.com/rancher/distros-test-framework/internal/resources"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestDaemonset(applyWorkload, deleteWorkload bool) {
	var workloadErr error
	if applyWorkload {
		workloadErr = resources.ManageWorkload("apply", "daemonset.yaml")
		Expect(workloadErr).NotTo(HaveOccurred(), "Daemonset manifest not deployed")
	}

	cmd := "kubectl get pods -n test-daemonset" +
		" --field-selector=status.phase=Running " +
		" --kubeconfig=" + resources.KubeConfigFile
	err := assert.ValidateOnHost(cmd, statusRunning)
	Expect(err).NotTo(HaveOccurred(), err)

	cmd = "kubectl get pods -n test-daemonset" +
		` -o jsonpath='{range .items[*]}{.spec.nodeName}{"\n"}{end}'` +
		" --kubeconfig=" + resources.KubeConfigFile

	nodeNames, err := resources.RunCommandHost(cmd)
	if err != nil {
		GinkgoT().Errorf(err.Error())
	}

	var nodes []string
	n := strings.Split(nodeNames, "\n")
	for _, nodeName := range n {
		if nodeName != "" {
			nodes = append(nodes, nodeName)
		}
	}

	cmd = "kubectl get nodes -o custom-columns=NAME:.metadata.name,TAINTS:.spec.taints" +
		" --kubeconfig=" + resources.KubeConfigFile + ` | grep '<none>'`
	taints, err := resources.RunCommandHost(cmd)
	if err != nil {
		return
	}
	Expect(taints).To(ContainSubstring("<none>"))
	Expect(validateNodesEqual(strings.TrimSpace(taints), strings.TrimSpace(nodeNames))).To(BeTrue())

	pods, _ := resources.GetPods(false)
	Eventually(func(_ Gomega) int {
		return resources.CountOfStringInSlice("test-daemonset", pods)
	}, "10s", "5s").Should(Equal(len(nodes)),
		"Daemonset pod count does not match node count")

	if deleteWorkload {
		workloadErr = resources.ManageWorkload("delete", "daemonset.yaml")
		Expect(workloadErr).NotTo(HaveOccurred(), "Daemonset manifest not deleted")
	}
}

// validateNodesEqual checks if the nodes in the two strings are equal (ignoring order through sorting).
func validateNodesEqual(taints, nodeNames string) bool {
	s1 := strings.Split(taints, "\n")
	s2 := strings.Split(nodeNames, "\n")

	for i, node := range s1 {
		fields := strings.Fields(node)
		if len(fields) > 0 {
			s1[i] = fields[0]
		}
	}

	for i, node := range s2 {
		fields := strings.Fields(node)
		if len(fields) > 0 {
			s2[i] = fields[0]
		}
	}

	if len(s1) != len(s2) {
		return false
	}
	sort.Strings(s1)
	sort.Strings(s2)

	for i := range s1 {
		if s1[i] != s2[i] {
			return false
		}
	}

	return true
}
