package template

import (
	"github.com/rancher/distros-test-framework/pkg/customflag"
)

// TestTemplate represents a version test scenario with test configurations and commands.
type TestTemplate struct {
	TestCombination *RunCmd
	InstallMode     string
	TestConfig      *TestConfig
	Description     string
	DebugMode       bool
}

// RunCmd represents the command sets to run on host and node.
type RunCmd struct {
	Run []customflag.TestMapConfig
}

// TestConfig represents the testcase function configuration.
type TestConfig struct {
	TestFunc       []testCase
	ApplyWorkload  bool
	DeleteWorkload bool
	WorkloadName   string
}

// testCase is a custom type representing the test function.
type testCase func(applyWorkload, deleteWorkload bool)

// testCaseWrapper wraps a test function and calls it with the given GinkgoTInterface and TestTemplate.
func testCaseWrapper(t TestTemplate) {
	for _, testFunc := range t.TestConfig.TestFunc {
		testFunc(t.TestConfig.ApplyWorkload, t.TestConfig.DeleteWorkload)
	}
}

// ConvertToTestCase converts the TestCaseFlag to testCase.
func ConvertToTestCase(testCaseFlags []customflag.TestCaseFlag) []testCase {
	var testCases []testCase
	for _, tcf := range testCaseFlags {
		tc := testCase(tcf)
		testCases = append(testCases, tc)
	}

	return testCases
}
