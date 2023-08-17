package template

import (
	"strings"
	"sync"

	"github.com/rancher/distros-test-framework/pkg/testcase"
	"github.com/rancher/distros-test-framework/shared"
)

// upgradeVersion upgrades the version of RKE2 and updates the expected values
func upgradeVersion(template VersionTestTemplate, version string) error {
	err := testcase.TestUpgradeClusterManually(version)
	if err != nil {
		return shared.ReturnLogError("failed to upgrade cluster: %v", err)
	}

	for i := range template.TestCombination.Run {
		template.TestCombination.Run[i].ExpectedValue =
			template.TestCombination.Run[i].ExpectedValueUpgrade
	}

	return nil
}

// checkVersion checks the version of RKE2 by calling processTestCombination
func checkVersion(v VersionTestTemplate) error {
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
			return shared.ReturnLogError("failed to process test combination: %v", chanErr)
		}
	}

	if v.TestConfig != nil {
		TestCaseWrapper(v)
	}

	return nil
}

// AddTestCases returns the test case based on the name to be used as customflag.
func AddTestCases(names []string) ([]TestCase, error) {
	var testCases []TestCase

	testCase := map[string]TestCase{
		"TestDaemonset":                   testcase.TestDaemonset,
		"TestIngress":                     testcase.TestIngress,
		"TestDnsAccess":                   testcase.TestDnsAccess,
		"TestServiceClusterIp":            testcase.TestServiceClusterIp,
		"TestServiceNodePort":             testcase.TestServiceNodePort,
		"TestLocalPathProvisionerStorage": testcase.TestLocalPathProvisionerStorage,
		"TestServiceLoadBalancer":         testcase.TestServiceLoadBalancer,
	}

	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			testCases = append(testCases, func(deployWorkload bool) {})
		} else if test, ok := testCase[name]; ok {
			testCases = append(testCases, test)
		} else {
			return nil, shared.ReturnLogError("invalid test case name")
		}
	}

	return testCases, nil
}
