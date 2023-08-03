package testcase

import (
	"fmt"
	"sort"
	"strings"

	"github.com/rancher/distros-test-framework/pkg/customflag"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/gomega"
)

func TestDaemonset(deployWorkload bool) {
	if deployWorkload {
		_, err := shared.ManageWorkload(
			"create",
			"daemonset.yaml",
			customflag.ServiceFlag.ClusterConfig.Arch.String(),
		)
		Expect(err).NotTo(HaveOccurred(),
			"Daemonset manifest not deployed")
	}
	pods, _ := shared.ParsePods(false)

	cmd := fmt.Sprintf(`
		kubectl get pods -n test-daemonset -o wide --kubeconfig="%s" \
		| grep -A10 NODE | awk 'NR>1 {print $7}'
		`,
		shared.KubeConfigFile,
	)
	nodeNames, err := shared.RunCommandHost(cmd)
	if err != nil {
		return
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

	Eventually(func(g Gomega) int {
		return shared.CountOfStringInSlice("test-daemonset", pods)
	}, "10s", "5s").Should(Equal(len(nodes)),
		"Daemonset pod count does not match node count")

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
