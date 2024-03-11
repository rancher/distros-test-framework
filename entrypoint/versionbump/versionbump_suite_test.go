package versionbump

import (
	"fmt"
	"os"
	"testing"

	"github.com/rancher/distros-test-framework/config"
	"github.com/rancher/distros-test-framework/entrypoint"
	"github.com/rancher/distros-test-framework/factory"
	"github.com/rancher/distros-test-framework/pkg/customflag"
	"github.com/rancher/distros-test-framework/pkg/template"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var cfg *config.Product

func TestMain(m *testing.M) {
	entrypoint.AddFlags("cmd", "expectedValue", "expectedValueUpgrade",
		"installVersionOrCommit", "channel", "testCase", "workloadName",
		"applyWorkload", "deleteWorkload", "destroy", "description")

	customflag.ServiceFlag.TestConfig.TestFuncNames = customflag.TestCaseNameFlag
	testFuncs, err := template.AddTestCases(customflag.ServiceFlag.TestConfig.TestFuncNames)
	if err != nil {
		fmt.Printf("error: %v\n", err)
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
		return
	}

	entrypoint.ValidateInstallFlag()

	os.Exit(m.Run())
}

func TestVersionTestSuite(t *testing.T) {
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
})
