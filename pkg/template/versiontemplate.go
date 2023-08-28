package template

import (
	"fmt"
	"strings"

	"github.com/rancher/distros-test-framework/pkg/customflag"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/ginkgo/v2"
)

//	func VersionTemplate(test VersionTestTemplate) {
//		if customflag.ServiceFlag.TestConfig.WorkloadName != "" &&
//			strings.HasSuffix(customflag.ServiceFlag.TestConfig.WorkloadName, ".yaml") {
//			_, err := shared.ManageWorkload(
//				"apply",
//				customflag.ServiceFlag.TestConfig.WorkloadName,
//			)
//			if err != nil {
//				GinkgoT().Errorf("failed to apply workload: %v", err)
//			}
//		}
//
//		err := executeTestCombination(test)
//		Expect(err).NotTo(HaveOccurred(), "error checking version: %v", err)
//
//		if test.InstallMode != "" {
//			upgErr := upgradeVersion(test, test.InstallMode)
//			Expect(upgErr).NotTo(HaveOccurred(), "error upgrading version: %v", upgErr)
//
//			err = checkVersion(test)
//			Expect(err).NotTo(HaveOccurred(), "error checking version: %v", err)
//
//			if test.TestConfig != nil {
//				TestCaseWrapper(test)
//			}
//		}
//	}
func VersionTemplate(test VersionTestTemplate) {
	if customflag.ServiceFlag.TestConfig.WorkloadName != "" &&
		strings.HasSuffix(customflag.ServiceFlag.TestConfig.WorkloadName, ".yaml") {
		_, err := shared.ManageWorkload(
			"apply",
			customflag.ServiceFlag.TestConfig.WorkloadName,
		)
		if err != nil {
			GinkgoT().Errorf(err.Error())
			return
		}
	}

	err := executeTestCombination(test)
	if err != nil {
		GinkgoT().Errorf(err.Error())
		return
	}

	if test.InstallMode != "" {
		for _, version := range test.InstallMode {
			if GinkgoT().Failed() {
				fmt.Println("executeTestCombination failed, not proceeding to upgrade")
				return
			}

			upgErr := upgradeVersion(test, version)
			if upgErr != nil {
				GinkgoT().Errorf("error upgrading: %v\n", err)
				return
			}

			err = executeTestCombination(test)
			if err != nil {
				GinkgoT().Errorf(err.Error())
				return
			}

			if test.TestConfig != nil {
				TestCaseWrapper(test)
			}
		}
	}
}
