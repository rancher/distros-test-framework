package template

import (
	"fmt"
	"strings"

	"github.com/rancher/distros-test-framework/pkg/customflag"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/ginkgo/v2"
)

func VersionTemplate(test VersionTestTemplate) {
	if customflag.ServiceFlag.TestConfig.WorkloadName != "" &&
		strings.HasSuffix(customflag.ServiceFlag.TestConfig.WorkloadName, ".yaml") {
		_, err := shared.ManageWorkload(
			"create",
			customflag.ServiceFlag.TestConfig.WorkloadName,
			customflag.ServiceFlag.ClusterConfig.Arch.String(),
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

	if test.InstallUpgrade != nil {
		for _, version := range test.InstallUpgrade {
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
