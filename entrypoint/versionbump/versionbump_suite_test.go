package versionbump

import (
	"flag"
	"os"
	"testing"

	"github.com/rancher/distros-test-framework/config"
	"github.com/rancher/distros-test-framework/factory"
	"github.com/rancher/distros-test-framework/pkg/productflag"
	"github.com/rancher/distros-test-framework/pkg/template"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var cfg *config.Product

func TestMain(m *testing.M) {
	flag.StringVar(&productflag.TestMap.Cmd, "cmd", "", "Comma separated list of commands to execute")
	flag.StringVar(&productflag.TestMap.ExpectedValue, "expectedValue", "", "Comma separated list of expected values for commands")
	flag.StringVar(&productflag.TestMap.ExpectedValueUpgrade, "expectedValueUpgrade", "", "Expected value of the command ran after upgrading")
	flag.Var(&productflag.ServiceFlag.InstallMode, "installVersionOrCommit", "Upgrade with version or commit")
	flag.Var(&productflag.ServiceFlag.Channel, "channel", "channel to use on install or upgrade")
	flag.Var(&productflag.TestCaseNameFlag, "testCase", "Comma separated list of test case names to run")
	flag.StringVar(&productflag.ServiceFlag.TestTemplateConfig.WorkloadName, "workloadName", "", "Name of the workload to a standalone deploy")
	flag.BoolVar(&productflag.ServiceFlag.TestTemplateConfig.ApplyWorkload, "applyWorkload", false, "Deploy workload customflag for tests passed in")
	flag.BoolVar(&productflag.ServiceFlag.TestTemplateConfig.DeleteWorkload, "deleteWorkload", false, "Delete workload customflag for tests passed in")
	flag.Var(&productflag.ServiceFlag.Destroy, "destroy", "Destroy cluster after test")
	flag.StringVar(&productflag.ServiceFlag.TestTemplateConfig.Description, "description", "", "Description of the test")
	flag.Parse()

	productflag.ValidateTemplateFlags()

	var err error
	cfg, err = shared.EnvConfig()
	if err != nil {
		return
	}

	// validating and adding test cases field on template testConfigFlag.
	productflag.ServiceFlag.TestTemplateConfig.TestFuncNames = productflag.TestCaseNameFlag
	testFuncs, err := template.AddTestCases(productflag.ServiceFlag.TestTemplateConfig.TestFuncNames)
	if err != nil {
		shared.LogLevel("error", "error on adding test cases to testConfigFlag: %w", err)
		return
	}

	if len(testFuncs) > 0 {
		testCaseFlags := make([]productflag.TestCaseFlag, len(testFuncs))
		for i, j := range testFuncs {
			testCaseFlags[i] = productflag.TestCaseFlag(j)
		}
		productflag.ServiceFlag.TestTemplateConfig.TestFuncs = testCaseFlags
	}

	os.Exit(m.Run())
}

func TestVersionTestSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Version Test Suite")
}

var _ = AfterSuite(func() {
	g := GinkgoT()
	if productflag.ServiceFlag.Destroy {
		status, err := factory.DestroyCluster(g)
		Expect(err).NotTo(HaveOccurred())
		Expect(status).To(Equal("cluster destroyed"))
	}
})
