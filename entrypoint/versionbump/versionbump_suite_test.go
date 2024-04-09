package versionbump

import (
	"flag"
	"fmt"
	"os"
	"testing"

	"github.com/rancher/distros-test-framework/config"
	"github.com/rancher/distros-test-framework/factory"
	"github.com/rancher/distros-test-framework/pkg/customflag"
	"github.com/rancher/distros-test-framework/pkg/template"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var cfg *config.Product

func TestMain(m *testing.M) {
	flag.StringVar(&template.TestMapTemplate.Cmd, "cmd", "", "Comma separated list of commands to execute")
	flag.StringVar(&template.TestMapTemplate.ExpectedValue, "expectedValue", "", "Comma separated list of expected values for commands")
	flag.StringVar(&template.TestMapTemplate.ExpectedValueUpgrade, "expectedValueUpgrade", "", "Expected value of the command ran after upgrading")
	flag.Var(&customflag.ServiceFlag.InstallMode, "installVersionOrCommit", "Upgrade with version or commit")
	flag.Var(&customflag.ServiceFlag.Channel, "channel", "channel to use on install or upgrade")
	flag.Var(&customflag.TestCaseNameFlag, "testCase", "Comma separated list of test case names to run")
	flag.StringVar(&customflag.ServiceFlag.TestConfig.WorkloadName, "workloadName", "", "Name of the workload to a standalone deploy")
	flag.BoolVar(&customflag.ServiceFlag.TestConfig.ApplyWorkload, "applyWorkload", false, "Deploy workload customflag for tests passed in")
	flag.BoolVar(&customflag.ServiceFlag.TestConfig.DeleteWorkload, "deleteWorkload", false, "Delete workload customflag for tests passed in")
	flag.BoolVar(&customflag.ServiceFlag.TestConfig.DebugMode, "debug", false, "Enable debug mode")
	flag.Var(&customflag.ServiceFlag.ClusterConfig.Destroy, "destroy", "Destroy cluster after test")
	flag.StringVar(&customflag.ServiceFlag.TestConfig.Description, "description", "", "Description of the test")
	flag.Parse()

	customflag.ServiceFlag.TestConfig.TestFuncNames = customflag.TestCaseNameFlag
	testFuncs, err := template.AddTestCases(customflag.ServiceFlag.TestConfig.TestFuncNames)
	if err != nil {
		shared.LogLevel("error", "error adding test cases to template: %w\n", err)
		return
	}

	if len(testFuncs) > 0 {
		testCaseFlags := make([]customflag.TestCaseFlag, len(testFuncs))
		for i, j := range testFuncs {
			testCaseFlags[i] = customflag.TestCaseFlag(j)
		}
		customflag.ServiceFlag.TestConfig.TestFuncs = testCaseFlags
	}

	cfg, err = shared.EnvConfig()
	if err != nil {
		shared.LogLevel("error", "error getting config: %w\n", err)
		return
	}

	if customflag.ServiceFlag.TestConfig.DebugMode == true {
		shared.LogLevel("info", "debug mode enabled on template\n\n")
	}

	os.Exit(m.Run())
}

func TestVersionBumpSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Version Test Suite")
}

var _ = AfterSuite(func() {
	g := GinkgoT()
	if customflag.ServiceFlag.ClusterConfig.Destroy {
		status, err := factory.DestroyCluster(g)
		Expect(err).NotTo(HaveOccurred())
		Expect(status).To(Equal("cluster destroyed"))
	}

	if err := config.SetEnv(shared.BasePath() + "/config/.env"); err != nil {
		Expect(err).To(BeNil(), fmt.Sprintf("error loading env vars: %v\n", err))
	}

	testTag := os.Getenv("TEST_TAG")
	if testTag == "components" {
		template.ComponentsBumpResults()
	}
	if testTag != "versionbump" {
		shared.PrintGetAll()
	}
})
