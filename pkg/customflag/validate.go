package customflag

import (
	"os"
	"regexp"
	"slices"
	"strings"

	"github.com/rancher/distros-test-framework/pkg/logger"
)

var log = logger.AddLogger()

// ValidateTemplateFlags validates version bump template flags that were set on environment variables at .env file.
func ValidateTemplateFlags() {
	var (
		expectedValues   []string
		expectedUpgrades []string
		testTag          string
		cmd              string
	)

	argsFromJenkins := os.Getenv("TEST_ARGS")
	if argsFromJenkins != "" {
		cmd, testTag, expectedValues, expectedUpgrades = validateFromJenkins(argsFromJenkins)
	} else {
		cmd, testTag, expectedValues, expectedUpgrades = validateFromLocal()
	}

	switch testTag {
	case "versionbump":
		validateVersionBumpTest(expectedValues, expectedUpgrades, cmd)
	case "cilium":
		validateCiliumTest(expectedValues, expectedUpgrades)
	case "multus":
		validateMultusTest(expectedValues, expectedUpgrades)
	case "components":
		validateComponentsTest(expectedValues, expectedUpgrades)
	case "flannel":
		validateSingleCNITest(expectedValues, expectedUpgrades)
	case "calico":
		validateSingleCNITest(expectedValues, expectedUpgrades)
	case "canal":
		validateCanalTest(expectedValues, expectedUpgrades)
	default:
		log.Errorf("test tag not found")
	}
}

func validateFromLocal() (cmd, testTag string, expectedValues, expectedUpgrades []string) {
	testTag = validateTestTagFromLocal()
	cmd = os.Getenv("CMD")
	if cmd == "" && testTag == "versionbump" {
		log.Error("cmd was not sent for versionbump test tag")
		os.Exit(1)
	} else if testTag != "versionbump" && cmd != "" {
		log.Errorf("cmd can not be sent for this test tag: %s", testTag)
		os.Exit(1)
	}

	expectedValue := os.Getenv("EXPECTED_VALUE")
	if expectedValue == "" {
		log.Error("expected value was not sent")
		os.Exit(1)
	}

	installVersionOrCommit := os.Getenv("INSTALL_VERSION_OR_COMMIT")
	valuesUpgrade := os.Getenv("VALUE_UPGRADED")
	if valuesUpgrade != "" {
		expectedUpgrades = strings.Split(valuesUpgrade, ",")
	}

	validateUpgradeFromLocal(installVersionOrCommit, valuesUpgrade)

	expectedValues = strings.Split(expectedValue, ",")

	return cmd, testTag, expectedValues, expectedUpgrades
}

// validateUpgradeFromLocal validates if the upgrade flag was sent and...
// if the expected value after upgrade was sent too inside the environment variables.
func validateUpgradeFromLocal(installVersionOrCommit, valuesUpgrade string) {
	if (installVersionOrCommit != "" && valuesUpgrade == "") || (installVersionOrCommit == "" && valuesUpgrade != "") {
		log.Error("using upgrade, please provide the expected value after upgrade and the install version or commit")
		os.Exit(1)
	}
}

func validateTestTagFromLocal() string {
	testTag := os.Getenv("TEST_TAG")
	if testTag == "" {
		log.Error("test tag was not sent")
		os.Exit(1)
	}

	testTags := []string{"calico", "canal", "cilium", "flannel", "multus", "components", "versionbump"}
	if !slices.Contains(testTags, testTag) {
		log.Errorf("test tag not found in: %s", testTag)
		os.Exit(1)
	}

	return testTag
}

func validateVersionBumpTest(expectedValue, expectedUpgrade []string, cmd string) {
	cmds := strings.Split(cmd, ",")

	if len(cmds) != len(expectedValue) {
		log.Errorf("mismatched length commands: %d x expected values: %d", len(cmds), len(expectedValue))
		os.Exit(1)
	}

	if expectedUpgrade != nil && len(expectedUpgrade) != len(expectedValue) {
		log.Errorf("mismatched length commands: %d x expected values upgrade: %d", len(cmds), len(expectedUpgrade))
		os.Exit(1)
	}
}

func validateCiliumTest(expectedValue, valuesUpgrade []string) {
	cmdCount := 2
	if len(expectedValue) != cmdCount {
		log.Errorf("mismatched length commands: %d x expected values: %d", cmdCount, len(expectedValue))
		os.Exit(1)
	}

	if valuesUpgrade != nil && len(valuesUpgrade) != cmdCount {
		log.Errorf("mismatched length commands: %d x expected values upgrade: %d",
			cmdCount, len(valuesUpgrade))
		os.Exit(1)
	}
}

func validateCanalTest(expectedValue, valuesUpgrade []string) {
	// calico + flannel
	cmdCount := 2
	if len(expectedValue) != cmdCount {
		log.Errorf("mismatched length commands: %d x expected values: %d", cmdCount, len(expectedValue))
		os.Exit(1)
	}

	if valuesUpgrade != nil && len(valuesUpgrade) != cmdCount {
		log.Errorf("mismatched length commands: %d x expected values upgrade: %d",
			cmdCount, len(valuesUpgrade))
		os.Exit(1)
	}
}

func validateSingleCNITest(expectedValue, valuesUpgrade []string) {
	cmdCount := 1
	if len(expectedValue) != cmdCount {
		log.Errorf("mismatched length commands: %d x expected values: %d", cmdCount, len(expectedValue))
		os.Exit(1)
	}

	if valuesUpgrade != nil && len(valuesUpgrade) != cmdCount {
		log.Errorf("mismatched length commands: %d x expected values upgrade: %d",
			cmdCount, len(valuesUpgrade))
		os.Exit(1)
	}
}

func validateMultusTest(expectedValue, valuesUpgrade []string) {
	cmdCount := 4
	if len(expectedValue) != cmdCount {
		log.Errorf("mismatched length commands: %d x expected values: %d", cmdCount, len(expectedValue))
		os.Exit(1)
	}

	if valuesUpgrade != nil && len(valuesUpgrade) != cmdCount {
		log.Errorf("mismatched length commands: %d x expected values upgrade: %d",
			cmdCount, len(valuesUpgrade))
		os.Exit(1)
	}
}

func validateComponentsTest(expectedValue, valuesUpgrade []string) {
	k3sComponentsCmdsCount := 10
	rke2ComponentsCmdsCount := 7

	product := os.Getenv("ENV_PRODUCT")
	switch product {
	case "k3s":
		if len(expectedValue) != k3sComponentsCmdsCount {
			log.Errorf("mismatched length commands: %d x expected values: %d", k3sComponentsCmdsCount, len(expectedValue))
			os.Exit(1)
		}

		if valuesUpgrade != nil && len(valuesUpgrade) != k3sComponentsCmdsCount {
			log.Errorf("mismatched length commands: %d x expected values upgrade: %d",
				k3sComponentsCmdsCount, len(valuesUpgrade))
			os.Exit(1)
		}

	case "rke2":
		if len(expectedValue) != rke2ComponentsCmdsCount {
			log.Errorf("mismatched length commands: %d x expected values: %d", rke2ComponentsCmdsCount, len(expectedValue))
			os.Exit(1)
		}

		if valuesUpgrade != nil && len(valuesUpgrade) != rke2ComponentsCmdsCount {
			log.Errorf("mismatched length commands: %d x expected values upgrade: %d",
				rke2ComponentsCmdsCount, len(valuesUpgrade))
			os.Exit(1)
		}
	}
}

func ValidateVersionFormat() {
	re := regexp.MustCompile(`^v\d+\.\d+\.\d+$`)
	envVars := []string{"CERT_MANAGER_VERSION", "CHARTS_VERSION"}

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
