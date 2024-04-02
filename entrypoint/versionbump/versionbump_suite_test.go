package versionbump

import (
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
	productflag.AddFlags("cmd", "expectedValue", "expectedValueUpgrade",
		"installVersionOrCommit", "channel", "testCase", "workloadName",
		"applyWorkload", "deleteWorkload", "destroy", "description")

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
