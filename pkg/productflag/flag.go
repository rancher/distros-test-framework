package productflag

import (
	"os"
	"regexp"
	"strings"

	"github.com/rancher/distros-test-framework/shared"
)

// ValidateTemplateFlags validates version bump template flags that were set.
func ValidateTemplateFlags() {
	if TestMap.Cmd == "" {
		shared.LogLevel("error", "cmd was not sent")
		os.Exit(1)
	}
	if TestMap.ExpectedValue == "" {
		shared.LogLevel("error", "expected value was not sent")
		os.Exit(1)
	}

	// for now we are validating that the length of commands and expected/upgraded values are the same.
	cmds := strings.Split(TestMap.Cmd, ",")
	expectedValues := strings.Split(TestMap.ExpectedValue, ",")
	if len(cmds) != len(expectedValues) {
		shared.LogLevel("error", "mismatched length commands x expected values: %s x %s", cmds, expectedValues)
		os.Exit(1)
	}
	if TestMap.ExpectedValueUpgrade != "" {
		expectedValuesUpgrade := strings.Split(TestMap.ExpectedValueUpgrade, ",")
		if len(cmds) != len(expectedValuesUpgrade) {
			shared.LogLevel("error", "mismatched length commands x expected values upgrade: %s x %s", cmds, expectedValuesUpgrade)
			os.Exit(1)
		}
	}

	if ServiceFlag.InstallMode.String() != "" && TestMap.ExpectedValueUpgrade == "" {
		shared.LogLevel("error", "using upgrade, please provide the expected value after upgrade")
		os.Exit(1)
	}
}

func ValidateVersionFormat() {
	rancherFlags := []string{
		ServiceFlag.RancherConfig.CertManagerVersion,
		ServiceFlag.RancherConfig.RancherHelmVersion,
		ServiceFlag.RancherConfig.RancherImageVersion,
	}

	re := regexp.MustCompile(`^v\d+\.\d+\.\d+$`)
	for _, v := range rancherFlags {
		if !re.MatchString(v) {
			shared.LogLevel("error", "invalid format: %s, expected format: v.xx.xx.xx", v)
			os.Exit(1)
		}
	}
}
