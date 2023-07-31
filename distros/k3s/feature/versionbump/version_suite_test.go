package versionbump

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/rancher/distros-test-framework/cmd"
	"github.com/rancher/distros-test-framework/lib/cluster"
	"github.com/rancher/distros-test-framework/component/template"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestMain(m *testing.M) {
	flag.Var(&cmd.ServiceFlag.ClusterConfig.Product, "product", "Distro to create cluster and run the tests")
	flag.StringVar(&template.TestMapTemplate.Cmd, "cmd", "", "Comma separated list of commands to execute")
	flag.StringVar(&template.TestMapTemplate.ExpectedValue, "expectedValue", "", "Comma separated list of expected values for commands")
	flag.StringVar(&template.TestMapTemplate.ExpectedValueUpgrade, "expectedValueUpgrade", "", "Expected value of the command ran after upgrading")
	flag.Var(&cmd.ServiceFlag.InstallUpgrade, "installVersionOrCommit", "Install upgrade cmd for version bump")
	flag.StringVar(&cmd.ServiceFlag.InstallType.Channel, "channel", "", "channel to use on install or upgrade")
	flag.Var(&cmd.TestCaseNameFlag, "testCase", "Comma separated list of test case names to run")
	flag.StringVar(&cmd.ServiceFlag.TestConfig.WorkloadName, "workloadName", "", "Name of the workload to a standalone deploy")
	flag.BoolVar(&cmd.ServiceFlag.TestConfig.DeployWorkload, "deployWorkload", false, "Deploy workload cmd for tests passed in")
	flag.Var(&cmd.ServiceFlag.ClusterConfig.Destroy, "destroy", "Destroy cluster after test")
	flag.Var(&cmd.ServiceFlag.ClusterConfig.Arch, "arch", "Architecture type")
	flag.StringVar(&cmd.ServiceFlag.TestConfig.Description, "description", "", "Description of the test")
	flag.Parse()

	if flag.Parsed() {
		installVersionOrCommit := strings.Split(cmd.ServiceFlag.InstallUpgrade.String(), ",")
		if len(installVersionOrCommit) == 1 && installVersionOrCommit[0] == "" {
			cmd.ServiceFlag.InstallUpgrade = nil
		} else {
			cmd.ServiceFlag.InstallUpgrade = installVersionOrCommit
		}
	}

	cmd.ServiceFlag.TestConfig.TestFuncNames = cmd.TestCaseNameFlag
	testFuncs, err := template.AddTestCases(cmd.ServiceFlag.TestConfig.TestFuncNames)
	if err != nil {
		fmt.Printf("error: %v\n", err)
		return
	}

	if len(testFuncs) > 0 {
		testCaseFlags := make([]cmd.TestCaseFlag, len(testFuncs))
		for i, j := range testFuncs {
			testCaseFlags[i] = cmd.TestCaseFlag(j)
		}
		cmd.ServiceFlag.TestConfig.TestFuncs = testCaseFlags
	}

	os.Exit(m.Run())
}

func TestVersionTestSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Version Test Suite")
}

var _ = AfterSuite(func() {
	g := GinkgoT()
	if cmd.ServiceFlag.ClusterConfig.Destroy {
		status, err := activity.DestroyCluster(g, cmd.ServiceFlag.ClusterConfig.Product.String())
		Expect(err).NotTo(HaveOccurred())
		Expect(status).To(Equal("cluster destroyed"))
	}
})
