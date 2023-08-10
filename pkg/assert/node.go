package assert

import (
	"fmt"
	"strings"

	"github.com/rancher/distros-test-framework/pkg/customflag"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type NodeAssertFunc func(g Gomega, node shared.Node)

// NodeAssertVersionTypeUpgrade  custom assertion func that asserts that node version is as expected
func NodeAssertVersionTypeUpgrade(c customflag.FlagConfig) NodeAssertFunc {
	if c.InstallType.Version != "" {
		return assertVersion(c)
	} else if c.InstallType.Commit != "" {
		return assertCommit(c)
	}

	return func(g Gomega, node shared.Node) {
		GinkgoT().Errorf("no version or commit specified for upgrade assertion")
	}
}

// assertVersion returns the NodeAssertFunc for asserting version
func assertVersion(c customflag.FlagConfig) NodeAssertFunc {
	fmt.Printf("Asserting Version: %s\n", c.InstallType.Version)
	return func(g Gomega, node shared.Node) {
		g.Expect(node.Version).Should(ContainSubstring(c.InstallType.Version),
			"Nodes should all be upgraded to the specified version", node.Name)
	}
}

// assertCommit returns the NodeAssertFunc for asserting commit
func assertCommit(c customflag.FlagConfig) NodeAssertFunc {
	product, err := shared.GetProduct()
	Expect(err).NotTo(HaveOccurred(), "error getting product: %v", err)

	commit, err := shared.GetProductVersion(product)
	Expect(err).NotTo(HaveOccurred(), "error getting commit ID version: %v", err)

	initial := strings.Index(commit, "(")
	ending := strings.Index(commit, ")")
	commit = commit[initial+1 : ending]

	fmt.Printf("Asserting Commit: %s\n", c.InstallType.Commit)
	return func(g Gomega, node shared.Node) {
		g.Expect(c.InstallType.Commit).Should(ContainSubstring(commit),
			"Nodes should all be upgraded to the specified commit", node.Name)
	}
}

// NodeAssertVersionUpgraded custom assertion func that asserts that node version is as expected
func NodeAssertVersionUpgraded() NodeAssertFunc {
	return func(g Gomega, node shared.Node) {
		g.Expect(&customflag.ServiceFlag.UpgradeVersionSUC).Should(ContainSubstring(node.Version),
			"Nodes should all be upgraded to the specified version", node.Name)
	}
}

// NodeAssertReadyStatus custom assertion func that asserts that the node is in Ready state.
func NodeAssertReadyStatus() NodeAssertFunc {
	return func(g Gomega, node shared.Node) {
		g.Expect(node.Status).Should(Equal("Ready"),
			"Nodes should all be in Ready state")
	}
}

// CheckComponentCmdNode runs a command on a node and asserts that the value received
// contains the specified substring.
func CheckComponentCmdNode(cmd, assert, ip string) error {
	if cmd == "" || assert == "" {
		return shared.ReturnLogError("cmd and/or assert should not be sent empty")
	}

	Eventually(func(g Gomega) {
		res, err := shared.RunCommandOnNode(cmd, ip)
		Expect(err).ToNot(HaveOccurred())
		g.Expect(res).Should(ContainSubstring(assert))

		fmt.Println("\nResult:\n", res+"\nMatched with:\n", assert)
	}, "420s", "3s").Should(Succeed())

	return nil
}
