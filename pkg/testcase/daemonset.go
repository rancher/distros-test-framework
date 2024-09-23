package testcase

import (
	"fmt"
	"sort"
	"strings"

	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestDaemonset(applyWorkload, deleteWorkload bool) {
	var workloadErr error
	if applyWorkload {
		workloadErr = shared.ManageWorkload("apply", "daemonset.yaml")
		Expect(workloadErr).NotTo(HaveOccurred(), "Daemonset manifest not deployed")
	}
	pods, _ := shared.GetPods(false)

	cmd := "kubectl get pods -n test-daemonset" +
		` -o jsonpath='{range .items[*]}{.spec.nodeName}{"\n"}{end}'` +
		" --kubeconfig=" + shared.KubeConfigFile

	nodeNames, err := shared.RunCommandHost(cmd)
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

	cmd = fmt.Sprintf(`
		kubectl get nodes -o custom-columns=NAME:.metadata.name,TAINTS:.spec.taints \
		--kubeconfig="%s" | grep '<none>'
		`,
		shared.KubeConfigFile,
	)
	taints, err := shared.RunCommandHost(cmd)
	if err != nil {
		return
	}
	Expect(taints).To(ContainSubstring("<none>"))
	Expect(validateNodesEqual(strings.TrimSpace(taints), strings.TrimSpace(nodeNames))).To(BeTrue())

	Eventually(func(_ Gomega) int {
		return shared.CountOfStringInSlice("test-daemonset", pods)
	}, "10s", "5s").Should(Equal(len(nodes)),
		"Daemonset pod count does not match node count")

	if deleteWorkload {
		workloadErr = shared.ManageWorkload("delete", "daemonset.yaml")
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
