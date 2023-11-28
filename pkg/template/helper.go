package template

import (
	"strings"
	"sync"

	"github.com/rancher/distros-test-framework/pkg/testcase"
	"github.com/rancher/distros-test-framework/shared"
)

// upgradeVersion upgrades the product version
func upgradeVersion(template VersionTestTemplate, version string) error {
	err := testcase.TestUpgradeClusterManually(version)
	if err != nil {
		return err
	}

	updateExpectedValue(template)

	return nil
}

// updateExpectedValue updates the expected values getting the values from flag ExpectedValueUpgrade
func updateExpectedValue(template VersionTestTemplate) {
	for i := range template.TestCombination.Run {
		template.TestCombination.Run[i].ExpectedValue =
			template.TestCombination.Run[i].ExpectedValueUpgrade
	}
}

// executeTestCombination get a template and pass it to `processTestCombination` to execute test combination on group of IPs
func executeTestCombination(v VersionTestTemplate) error {
	ips := shared.FetchNodeExternalIP()

	var wg sync.WaitGroup
	errorChanList := make(
		chan error,
		len(ips)*(len(v.TestCombination.Run)),
	)

	processTestCombination(errorChanList, &wg, ips, *v.TestCombination)

	wg.Wait()
	close(errorChanList)

	for chanErr := range errorChanList {
		if chanErr != nil {
			return chanErr
		}
	}

	if v.TestConfig != nil {
		testCaseWrapper(v)
	}

	return nil
}

// AddTestCases returns the test case based on the name to be used as customflag.
func AddTestCases(names []string) ([]testCase, error) {
	var testCases []testCase

	tcs := map[string]testCase{
		"TestDaemonset":                    testcase.TestDaemonset,
		"TestIngress":                      testcase.TestIngress,
		"TestDnsAccess":                    testcase.TestDnsAccess,
		"TestServiceClusterIP":             testcase.TestServiceClusterIp,
		"TestServiceNodePort":              testcase.TestServiceNodePort,
		"TestLocalPathProvisionerStorage":  testcase.TestLocalPathProvisionerStorage,
		"TestServiceLoadBalancer":          testcase.TestServiceLoadBalancer,
		"TestInternodeConnectivityMixedOS": testcase.TestInternodeConnectivityMixedOS,
	}

	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			testCases = append(testCases, func(applyWorkload, deleteWorkload bool) {})
		} else if test, ok := tcs[name]; ok {
			testCases = append(testCases, test)
		} else {
			return nil, shared.ReturnLogError("invalid test case name")
		}
	}

	return testCases, nil
}
