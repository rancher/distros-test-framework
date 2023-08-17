package template

import (
	"strings"

	"github.com/rancher/distros-test-framework/pkg/customflag"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func VersionTemplate(test VersionTestTemplate) {
	if customflag.ServiceFlag.TestConfig.WorkloadName != "" &&
		strings.HasSuffix(customflag.ServiceFlag.TestConfig.WorkloadName, ".yaml") {
		_, err := shared.ManageWorkload(
			"apply",
			customflag.ServiceFlag.TestConfig.WorkloadName,
			customflag.ServiceFlag.ClusterConfig.Arch.String(),
		)
		if err != nil {
			GinkgoT().Errorf("failed to apply workload: %v", err)
		}
	}

	err := checkVersion(test)
	Expect(err).NotTo(HaveOccurred(), "error checking version: %v", err)

	if test.InstallMode != "" {
		upgErr := upgradeVersion(test, test.InstallMode)
		Expect(upgErr).NotTo(HaveOccurred(), "error upgrading version: %v", upgErr)

		err = checkVersion(test)
		Expect(err).NotTo(HaveOccurred(), "error checking version: %v", err)

		if test.TestConfig != nil {
			TestCaseWrapper(test)
		}
	}
}
