package template

import (
	"strings"

	"github.com/rancher/distros-test-framework/pkg/customflag"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/gomega"
)

func Template(test TestTemplate) {
	if customflag.ServiceFlag.TestConfig.WorkloadName != "" &&
		strings.HasSuffix(customflag.ServiceFlag.TestConfig.WorkloadName, ".yaml") {
		err := shared.ManageWorkload(
			"apply",
			customflag.ServiceFlag.TestConfig.WorkloadName,
		)
		Expect(err).NotTo(HaveOccurred())
	}

	err := executeTestCombination(test)
	Expect(err).NotTo(HaveOccurred(), "error checking version: %v", err)

	if test.InstallMode != "" {
		upgErr := upgradeVersion(test, test.InstallMode)
		Expect(upgErr).NotTo(HaveOccurred(), "error upgrading version: %v", upgErr)

		err = executeTestCombination(test)
		Expect(err).NotTo(HaveOccurred(), "error checking version: %v", err)

		if test.TestConfig != nil {
			wrapper(test)
		}
	}
}
