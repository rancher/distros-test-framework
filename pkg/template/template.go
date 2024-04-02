package template

import (
	"strings"

	"github.com/rancher/distros-test-framework/pkg/productflag"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/gomega"
)

func Template(test TestTemplate) {
	if productflag.ServiceFlag.TestTemplateConfig.WorkloadName != "" &&
		strings.HasSuffix(productflag.ServiceFlag.TestTemplateConfig.WorkloadName, ".yaml") {
		err := shared.ManageWorkload(
			"apply",
			productflag.ServiceFlag.TestTemplateConfig.WorkloadName,
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
			testCaseWrapper(test)
		}
	}
}
