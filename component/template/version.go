package template

import (
	"fmt"
	"strings"

	"github.com/rancher/distros-test-framework/cmd"
	"github.com/rancher/distros-test-framework/lib/shared"

	. "github.com/onsi/ginkgo/v2"
)

// VersionTemplate is a template for testing RKE2 versions + test cases and upgrading cluster if needed
func VersionTemplate(test VersionTestTemplate, product string) {
	if cmd.ServiceFlag.TestConfig.WorkloadName != "" &&
		strings.HasSuffix(cmd.ServiceFlag.TestConfig.WorkloadName, ".yaml") {
		_, err := shared.ManageWorkload(
			"create",
			cmd.ServiceFlag.TestConfig.WorkloadName,
			// cmd.ServiceFlag.ClusterConfig.Arch.String(),
		)
		if err != nil {
			GinkgoT().Errorf(err.Error())
			return
		}
	}

	err := checkVersion(test, product)
	if err != nil {
		GinkgoT().Errorf(err.Error())
		return
	}

	if test.InstallUpgrade != nil {
		for _, version := range test.InstallUpgrade {
			if GinkgoT().Failed() {
				fmt.Println("checkVersion failed, upgrade will not be performed")
				return
			}

			err := upgradeVersion(test, product, version)
			if err != nil {
				GinkgoT().Errorf("error upgrading: %v\n", err)
				return
			}

			err = checkVersion(test, product)
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
