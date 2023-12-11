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
	if c.InstallMode.Version != "" {
		return assertVersion(c)
	} else if c.InstallMode.Commit != "" {
		return assertCommit(c)
	}

	return func(g Gomega, node shared.Node) {
		GinkgoT().Errorf("no version or commit specified for upgrade assertion")
	}
}

// assertVersion returns the NodeAssertFunc for asserting version
func assertVersion(c customflag.FlagConfig) NodeAssertFunc {
	fmt.Printf("Asserting Version: %s\n", c.InstallMode.Version)
	return func(g Gomega, node shared.Node) {
		version := strings.Split(c.InstallMode.Version, "-")
		g.Expect(node.Version).Should(ContainSubstring(version[0]),
			"Nodes should all be upgraded to the specified version", node.Name)
	}
}

// assertCommit returns the NodeAssertFunc for asserting commit
func assertCommit(c customflag.FlagConfig) NodeAssertFunc {
	product, err := shared.Product()
	Expect(err).NotTo(HaveOccurred(), "error getting product: %v", err)

	commit, err := shared.ProductVersion(product)
	Expect(err).NotTo(HaveOccurred(), "error getting commit ID version: %v", err)

	initial := strings.Index(commit, "(")
	ending := strings.Index(commit, ")")
	commit = commit[initial+1 : ending]

	fmt.Printf("Asserting Commit: %s\n", c.InstallMode.Commit)
	return func(g Gomega, node shared.Node) {
		g.Expect(c.InstallMode.Commit).Should(ContainSubstring(commit),
			"Nodes should all be upgraded to the specified commit", node.Name)
	}
}

// NodeAssertVersionUpgraded custom assertion func that asserts that node version is as expected
func NodeAssertVersionUpgraded() NodeAssertFunc {
	return func(g Gomega, node shared.Node) {
		version := strings.Split(customflag.ServiceFlag.SUCUpgradeVersion.String(), "-")
		g.Expect(node.Version).Should(ContainSubstring(version[0]),
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
func CheckComponentCmdNode(cmd, ip string, asserts ...string) error {
	if cmd == "" {
		return shared.ReturnLogError("cmd should not be sent empty")
	}
	for _, assert := range asserts {
		if assert == "" {
			return shared.ReturnLogError("asserts should not be sent empty")
		}
	}

	Eventually(func(g Gomega) error {
		fmt.Println("\nExecuting cmd: ", cmd)
		res, err := shared.RunCmdNode(cmd, ip)
		Expect(err).ToNot(HaveOccurred())

		for _, assert := range asserts {
			g.Expect(res).Should(ContainSubstring(assert))
			fmt.Println("\nResult:\n", res+"\nMatched with:\n", assert)
		}

		return nil

	}, "420s", "5s").Should(Succeed())

	return nil
}
