package assert

import (
	"fmt"
	"log"
	"strings"

	"github.com/rancher/distros-test-framework/pkg/customflag"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type NodeAssertFunc func(g Gomega, node shared.Node)

// NodeAssertVersionTypeUpgrade  custom assertion func that asserts that node version is as expected
func NodeAssertVersionTypeUpgrade(installType customflag.FlagConfig) NodeAssertFunc {
	if installType.InstallUpgrade != nil {
		if strings.HasPrefix(customflag.ServiceFlag.InstallUpgrade.String(), "v") {
			return assertVersion(installType)
		}
		return assertCommit(installType)
	}

	return func(g Gomega, node shared.Node) {
		GinkgoT().Errorf("no version or commit specified for upgrade assertion")
	}
}

// assertVersion returns the NodeAssertFunc for asserting version
func assertVersion(installType customflag.FlagConfig) NodeAssertFunc {
	fmt.Printf("Asserting Version: %s\n", installType.InstallUpgrade.String())
	return func(g Gomega, node shared.Node) {
		g.Expect(node.Version).Should(ContainSubstring(installType.InstallUpgrade.String()),
			"Nodes should all be upgraded to the specified version", node.Name)
	}
}

// assertCommit returns the NodeAssertFunc for asserting commit
func assertCommit(installType customflag.FlagConfig) NodeAssertFunc {
	product, err := shared.GetProduct()
	if err != nil {
		log.Println(err)
	}

	commit, err := shared.GetProductVersion(product)
	if err != nil {
		log.Println(err)
	}

	fmt.Printf("Asserting Commit: %s\n", installType.InstallUpgrade.String())
	return func(g Gomega, node shared.Node) {
		g.Expect(commit).Should(ContainSubstring(installType.InstallUpgrade.String()),
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
func CheckComponentCmdNode(cmd, ip string, asserts ...string) error{
var err error
	Eventually(func() error {
		fmt.Println("Executing cmd: ", cmd)
		res, cmdErr := shared.RunCommandOnNode(cmd, ip)
		if err != nil {
			return fmt.Errorf("error on RunCommandNode: %v", cmdErr)
		}

		for _, assert := range asserts {
            if !strings.Contains(res, assert) {
                return fmt.Errorf("expected substring %q not found in result %q", assert, res)
            }
            fmt.Println("\nResult:\n", res+"\nMatched with assert:\n", assert)
        }

        return nil
	}, "420s", "3s").Should(Succeed())
	return err // Return the error from Eventually
}

// CheckNotPresentOnNode runs a command on a node and asserts that the value received
// is NOT present in the given output.
func CheckNotPresentOnNode(cmd, ip string, notExpOutput ...string) {
	Eventually(func() error {
		fmt.Println("Executing cmd: ", cmd)
		res, err := shared.RunCommandOnNode(cmd, ip)
		if err != nil {
			return fmt.Errorf("error on RunCommandNode: %v", err)
		}

		for _, assert := range notExpOutput {
            if strings.Contains(res, assert) {
               return fmt.Errorf("%q was found in the output %q", assert, res)
            }
            fmt.Println("Result:\n",res+"\nPassed. Output should not match with:\n", assert)
        }

        return nil
	}, "420s", "3s").Should(Succeed())
}