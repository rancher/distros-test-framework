package template

import (
	"github.com/rancher/distros-test-framework/pkg/customflag"
)

var TestMapTemplate TestMap

// VersionTestTemplate represents a version test scenario with test configurations and commands.
type VersionTestTemplate struct {
	TestCombination *RunCmd
	InstallMode     string
	TestConfig      *TestConfig
	Description     string
}

// RunCmd represents the command sets to run on host and node.
type RunCmd struct {
	Run []TestMap
}

// TestMap represents a single test command with key:value pairs.
type TestMap struct {
	Cmd                  string
	ExpectedValue        string
	ExpectedValueUpgrade string
}

// TestConfig represents the testcase function configuration
type TestConfig struct {
	TestFunc       []testCase
	ApplyWorkload bool
	DeleteWorkload bool
	WorkloadName   string
}

// testCase is a custom type representing the test function.
type testCase func(applyWorkload, deleteWorkload bool)

// testCaseWrapper wraps a test function and calls it with the given GinkgoTInterface and VersionTestTemplate.
func testCaseWrapper(v VersionTestTemplate) {
	for _, testFunc := range v.TestConfig.TestFunc {
		testFunc(v.TestConfig.ApplyWorkload, v.TestConfig.DeleteWorkload)
	}
}

// ConvertToTestCase converts the TestCaseFlag to testCase
func ConvertToTestCase(testCaseFlags []customflag.TestCaseFlag) []testCase {
	var testCases []testCase
	for _, tcf := range testCaseFlags {
		tc := testCase(tcf)
		testCases = append(testCases, tc)
	}

	return testCases
}
