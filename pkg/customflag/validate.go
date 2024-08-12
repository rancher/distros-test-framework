package customflag

import (
	"os"
	"regexp"
	"strings"

	"github.com/rancher/distros-test-framework/pkg/logger"
)

var log = logger.AddLogger()

// ValidateTemplateFlags validates version bump template flags that were set on environment variables at .env file.
func ValidateTemplateFlags() {
	var (
		expected        []string
		expectedUpgrade []string
		testTag         string
	)

	cmd := os.Getenv("CMD")
	testArgs := os.Getenv("TEST_ARGS")
	expectedValue := os.Getenv("EXPECTED_VALUE")

	// validate if expected value was sent because it is required for all tests.
	if expectedValue == "" && !strings.Contains(testArgs, "expectedValue") {
		log.Errorf("expected value was not sent")
		os.Exit(1)
	}

	// checking if the testArgs was sent, if so it means we are getting vars from Jenkins not from .env file.
	if testArgs != "" {
		validateUpgrade(testArgs)
		testTag = validateTestTag(testArgs)
		expected, expectedUpgrade = validateExpectedValues(testArgs)
	} else {
		installVersionOrCommit := os.Getenv("INSTALL_VERSION_OR_COMMIT")
		valuesUpgrade := os.Getenv("VALUE_UPGRADED")
		testTag = os.Getenv("TEST_TAG")

		expected = strings.Split(expectedValue, ",")
		expectedUpgrade = strings.Split(valuesUpgrade, ",")

		// validate if flag for install version or commit was sent we should have the expected value after upgrade.
		if (installVersionOrCommit != "" && valuesUpgrade == "") || (installVersionOrCommit == "" && valuesUpgrade != "") {
			log.Errorf("using upgrade, please provide the expected value after upgrade and the install version or commit")
			os.Exit(1)
		}
	}

	switch testTag {
	case "versionbump":
		validateVersionBumpTest(expected, expectedUpgrade, cmd, testArgs)
	case "cilium":
		validateCiliumTest(expected, expectedUpgrade, cmd, testArgs)
	case "multus":
		validateMultusTest(expected, expectedUpgrade, cmd, testArgs)
	case "components":
		validateComponentsTest(expected, expectedUpgrade, cmd, testArgs)
	case "flannel":
		validateFlannelTest(cmd, testArgs)
	default:
		log.Errorf("test tag not found")
	}
}

func validateVersionBumpTest(expectedValue, expectedUpgrade []string, cmd, testArgs string) {
	if cmd == "" && !strings.Contains(testArgs, "cmd") {
		log.Errorf("cmd was not sent")
		os.Exit(1)
	}

	cmdLenght := strings.Split(cmd, ",")
	if len(cmdLenght) != len(expectedValue) {
		log.Errorf("mismatched length commands: %d x expected values: %d", len(cmdLenght), len(expectedValue))
		os.Exit(1)
	}

	if len(expectedUpgrade) > 0 && len(expectedUpgrade) != len(expectedValue) {
		log.Errorf("mismatched length commands: %d x expected values upgrade: %d",
			len(expectedUpgrade), len(expectedValue))
		os.Exit(1)
	}
}

func validateFlannelTest(cmd, testArgs string) {
	if cmd != "" || strings.Contains(testArgs, "cmd") {
		log.Errorf("cmd can not be sent for flannel tests as it is already defined in the test file")
		os.Exit(1)
	}
}

func validateCiliumTest(expectedValue, valuesUpgrade []string, cmd, testArgs string) {
	if cmd != "" || strings.Contains(testArgs, "cmd") {
		log.Errorf("cmd can not be sent for cilium tests as it is already defined in the test file")
		os.Exit(1)
	}

	ciliumCmdsLength := 2
	if len(expectedValue) != ciliumCmdsLength {
		log.Errorf("mismatched length commands: %d x expected values: %d", ciliumCmdsLength, len(expectedValue))
		os.Exit(1)
	}

	if len(valuesUpgrade) > 0 && len(valuesUpgrade) != ciliumCmdsLength {
		log.Errorf("mismatched length commands: %d x expected values upgrade: %d",
			ciliumCmdsLength, len(valuesUpgrade))
		os.Exit(1)
	}
}

func validateMultusTest(expectedValue, valuesUpgrade []string, cmd, testArgs string) {
	if cmd != "" || strings.Contains(testArgs, "cmd") {
		log.Errorf("cmd can not be sent for multus tests as it is already defined in the test file")
		os.Exit(1)
	}

	multusCmdsLength := 4
	if len(expectedValue) != multusCmdsLength {
		log.Errorf("mismatched length commands: %d x expected values: %d", multusCmdsLength, len(expectedValue))
		os.Exit(1)
	}

	if len(valuesUpgrade) > 0 && len(valuesUpgrade) != multusCmdsLength {
		log.Errorf("mismatched length commands: %d x expected values upgrade: %d",
			multusCmdsLength, len(valuesUpgrade))
		os.Exit(1)
	}
}

func validateComponentsTest(expectedValue, valuesUpgrade []string, cmd, cmdTestArgs string) {
	if cmd != "" || strings.Contains(cmdTestArgs, "cmd") {
		log.Errorf("cmd can not be sent for components tests as it is already defined in the test file")
		os.Exit(1)
	}

	k3scomponentsCmdsLength := 9
	rke2componentsCmdsLength := 8

	product := os.Getenv("ENV_PRODUCT")
	switch product {
	case "k3s":
		if len(expectedValue) != k3scomponentsCmdsLength {
			log.Errorf("mismatched length commands: %d x expected values: %d", k3scomponentsCmdsLength, len(expectedValue))
			os.Exit(1)
		}

		if len(valuesUpgrade) > 0 && len(valuesUpgrade) != k3scomponentsCmdsLength {
			log.Errorf("mismatched length commands: %d x expected values upgrade: %d",
				k3scomponentsCmdsLength, len(valuesUpgrade))
			os.Exit(1)
		}

	case "rke2":
		if len(expectedValue) != rke2componentsCmdsLength {
			log.Errorf("mismatched length commands: %d x expected values: %d", rke2componentsCmdsLength, len(expectedValue))
			os.Exit(1)
		}

		if len(valuesUpgrade) > 0 && len(valuesUpgrade) != rke2componentsCmdsLength {
			log.Errorf("mismatched length commands: %d x expected values upgrade: %d",
				rke2componentsCmdsLength, len(valuesUpgrade))
			os.Exit(1)
		}
	}
}

func ValidateVersionFormat() {
	re := regexp.MustCompile(`^v\d+\.\d+\.\d+$`)
	envVars := []string{"CERT_MANAGER_VERSION", "CHARTS_VERSION", "CHARTS_REPO_URL"}

	for _, v := range envVars {
		value := os.Getenv(v)
		if value == "" {
			continue
		}
		if !re.MatchString(value) {
			log.Errorf("invalid format: %s, expected format: v.xx.xx", value)
			os.Exit(1)
		}
	}
}

func ValidateTemplateTcs() {
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
				log.Errorf("test case %s not found", tc)
				os.Exit(1)
			}
		}
	}
}
