package customflag

import (
	"os"
	"regexp"
	"strings"

	"github.com/rancher/distros-test-framework/config"
	"github.com/rancher/distros-test-framework/shared"
)

// ValidateTemplateFlags validates version bump template flags that were set on environment variables at .env file.
func ValidateTemplateFlags() {
	if err := config.SetEnv(shared.BasePath() + "/config/.env"); err != nil {
		os.Exit(1)
	}

	testTag := os.Getenv("TEST_TAG")
	expectedValue := os.Getenv("EXPECTED_VALUE")

	// validate if expected value was sent because it is required for all tests.
	if expectedValue == "" {
		shared.LogLevel("error", "expected value was not sent")
		os.Exit(1)
	}

	// validate if flag for install version or commit was sent we should have the expected value after upgrade.
	instalVersionOrCommit := os.Getenv("INSTALL_VERSION_OR_COMMIT")
	valuesUpgrade := os.Getenv("VALUE_UPGRADED")
	if instalVersionOrCommit != "" && valuesUpgrade == "" {
		shared.LogLevel("error", "using upgrade, please provide the expected value after upgrade")
		os.Exit(1)
	}

	expected := strings.Split(expectedValue, ",")
	expectedUpgrade := strings.Split(valuesUpgrade, ",")

	switch testTag {
	case "versionbump":
		validateVersionBumpTest(expected, expectedUpgrade, valuesUpgrade)
	case "cilium":
		validateCiliumTest(expected, expectedUpgrade, valuesUpgrade)
	case "multus":
		validateMultusTest(expected, expectedUpgrade, valuesUpgrade)
	case "components":
		validateComponentsTest(expected, expectedUpgrade, valuesUpgrade)
	default:
		shared.LogLevel("error", "test tag not found")
	}

}

func validateVersionBumpTest(expectedValue, expectedUpgrade []string, valuesUpgrade string) {
	cmd := os.Getenv("CMD")
	if cmd == "" {
		shared.LogLevel("error", "cmd was not sent")
		os.Exit(1)
	}

	cmdLenght := strings.Split(cmd, ",")
	if len(cmdLenght) != len(expectedValue) {
		shared.LogLevel("error", "mismatched length commands: %d x expected values: %d",
			len(cmdLenght), len(expectedValue))
		os.Exit(1)
	}

	if valuesUpgrade != "" {
		if len(expectedUpgrade) != len(expectedValue) {
			shared.LogLevel("error", "mismatched length commands: %d x expected values upgrade: %d",
				len(cmdLenght), len(expectedValue))
			os.Exit(1)
		}
	}
}

func validateCiliumTest(expectedValue, valuesUpgrade []string, upgrade string) {
	ciliumCmdsLenght := 2

	if len(expectedValue) != ciliumCmdsLenght {
		shared.LogLevel("error", "mismatched length commands: %d x expected values: %d",
			ciliumCmdsLenght, len(expectedValue))
		os.Exit(1)
	}

	if upgrade != "" {
		if len(valuesUpgrade) != ciliumCmdsLenght {
			shared.LogLevel("error", "mismatched length commands: %d x expected values upgrade: %d",
				ciliumCmdsLenght, len(valuesUpgrade))
			os.Exit(1)
		}
	}
}

func validateMultusTest(expectedValue, valuesUpgrade []string, upgrade string) {
	multusCmdsLenght := 4

	if len(expectedValue) != multusCmdsLenght {
		shared.LogLevel("error", "mismatched length commands: %d x expected values: %d",
			multusCmdsLenght, len(expectedValue))
		os.Exit(1)
	}

	if upgrade != "" {
		if len(valuesUpgrade) != multusCmdsLenght {
			shared.LogLevel("error", "mismatched length commands: %d x expected values upgrade: %d",
				multusCmdsLenght, len(valuesUpgrade))
			os.Exit(1)
		}
	}
}

func validateComponentsTest(expectedValue, valuesUpgrade []string, upgrade string) {
	product := os.Getenv("ENV_PRODUCT")
	k3scomponentsCmdsLenght := 10
	rke2componentsCmdsLenght := 8

	switch product {
	case "k3s":
		if len(expectedValue) != k3scomponentsCmdsLenght {
			shared.LogLevel("error", "mismatched length commands: %d x expected values: %d",
				k3scomponentsCmdsLenght, len(expectedValue))
			os.Exit(1)
		}

		if upgrade != "" {
			if len(valuesUpgrade) != k3scomponentsCmdsLenght {
				shared.LogLevel("error", "mismatched length commands: %d x expected values upgrade: %d",
					k3scomponentsCmdsLenght, len(valuesUpgrade))
				os.Exit(1)
			}
		}
	case "rke2":
		if len(expectedValue) != rke2componentsCmdsLenght {
			shared.LogLevel("error", "mismatched length commands: %d x expected values: %d",
				rke2componentsCmdsLenght, len(expectedValue))
			os.Exit(1)
		}

		if upgrade != "" {
			if len(valuesUpgrade) != rke2componentsCmdsLenght {
				shared.LogLevel("error", "mismatched length commands: %d x expected values upgrade: %d",
					rke2componentsCmdsLenght, len(valuesUpgrade))
				os.Exit(1)
			}
		}
	}
}

func ValidateVersionFormat() {
	if err := config.SetEnv(shared.BasePath() + "/config/.env"); err != nil {
		os.Exit(1)
	}

	re := regexp.MustCompile(`^v\d+\.\d+\.\d+$`)
	envVars := []string{"CERTMANAGERVERSION", "CHARTSVERSION", "CHARTSREPOURL"}

	for _, v := range envVars {
		value := os.Getenv(v)
		if value == "" {
			continue
		}
		if !re.MatchString(value) {
			shared.LogLevel("error", "invalid format: %s, expected format: v.xx.xx.xx", value)
			os.Exit(1)
		}
	}
}

func ValidateTemplateTcs() {
	if err := config.SetEnv(shared.BasePath() + "/config/.env"); err != nil {
		os.Exit(1)
	}

	validTestCases := map[string]struct{}{
		"TestDaemonset":                    {},
		"TestIngress":                      {},
		"TestDnsAccess":                    {},
		"TestServiceClusterIP":             {},
		"TestServiceNodePort":              {},
		"TestLocalPathProvisionerStorage":  {},
		"TestServiceLoadBalancer":          {},
		"TestInternodeConnectivityMixedOS": {},
		"TestSonobuoyMixedOS":              {},
		"TestSelinuxEnabled":               {},
		"TestSelinux":                      {},
		"TestSelinuxSpcT":                  {},
		"TestUninstallPolicy":              {},
		"TestSelinuxContext":               {},
		"TestIngressRoute":                 {},
		"TestCertRotate":                   {},
		"TestSecretsEncryption":            {},
		"TestRestartService":               {},
		"TestClusterReset":                 {},
	}

	tcs := os.Getenv("TEST_CASE")
	if tcs != "" {
		testCases := strings.Split(tcs, ",")

		for _, tc := range testCases {
			tc = strings.TrimSpace(tc)
			if _, exists := validTestCases[tc]; !exists {
				shared.LogLevel("error", "test case %s not found", tcs)
				os.Exit(1)
			}
		}
	}
}
