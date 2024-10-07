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

	if testTag != "versionbump" && testTag != "cilium" && testTag != "multus" &&
		testTag != "components" && testTag != "flannel" && testTag != "calico" &&
		testTag != "canal" {
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
	ciliumCmdsLength := 2
	if len(expectedValue) != ciliumCmdsLength {
		log.Errorf("mismatched length commands: %d x expected values: %d", ciliumCmdsLength, len(expectedValue))
		os.Exit(1)
	}

	if valuesUpgrade != nil && len(valuesUpgrade) != ciliumCmdsLength {
		log.Errorf("mismatched length commands: %d x expected values upgrade: %d",
			ciliumCmdsLength, len(valuesUpgrade))
		os.Exit(1)
	}
}

func validateCanalTest(expectedValue, valuesUpgrade []string) {
	// calico + flannel
	canalCmdsLength := 2
	if len(expectedValue) != canalCmdsLength {
		log.Errorf("mismatched length commands: %d x expected values: %d", canalCmdsLength, len(expectedValue))
		os.Exit(1)
	}

	if valuesUpgrade != nil && len(valuesUpgrade) != canalCmdsLength {
		log.Errorf("mismatched length commands: %d x expected values upgrade: %d",
			canalCmdsLength, len(valuesUpgrade))
		os.Exit(1)
	}
}

func validateSingleCNITest(expectedValue, valuesUpgrade []string) {
	cmdsLength := 1
	if len(expectedValue) != cmdsLength {
		log.Errorf("mismatched length commands: %d x expected values: %d", cmdsLength, len(expectedValue))
		os.Exit(1)
	}

	if valuesUpgrade != nil && len(valuesUpgrade) != cmdsLength {
		log.Errorf("mismatched length commands: %d x expected values upgrade: %d",
			cmdsLength, len(valuesUpgrade))
		os.Exit(1)
	}
}

func validateMultusTest(expectedValue, valuesUpgrade []string) {
	multusCmdsLength := 4
	if len(expectedValue) != multusCmdsLength {
		log.Errorf("mismatched length commands: %d x expected values: %d", multusCmdsLength, len(expectedValue))
		os.Exit(1)
	}

	if valuesUpgrade != nil && len(valuesUpgrade) != multusCmdsLength {
		log.Errorf("mismatched length commands: %d x expected values upgrade: %d",
			multusCmdsLength, len(valuesUpgrade))
		os.Exit(1)
	}
}

func validateComponentsTest(expectedValue, valuesUpgrade []string) {
	k3scomponentsCmdsLength := 10
	rke2componentsCmdsLength := 7

	product := os.Getenv("ENV_PRODUCT")
	switch product {
	case "k3s":
		if len(expectedValue) != k3scomponentsCmdsLength {
			log.Errorf("mismatched length commands: %d x expected values: %d", k3scomponentsCmdsLength, len(expectedValue))
			os.Exit(1)
		}

		if valuesUpgrade != nil && len(valuesUpgrade) != k3scomponentsCmdsLength {
			log.Errorf("mismatched length commands: %d x expected values upgrade: %d",
				k3scomponentsCmdsLength, len(valuesUpgrade))
			os.Exit(1)
		}

	case "rke2":
		if len(expectedValue) != rke2componentsCmdsLength {
			log.Errorf("mismatched length commands: %d x expected values: %d", rke2componentsCmdsLength, len(expectedValue))
			os.Exit(1)
		}

		if valuesUpgrade != nil && len(valuesUpgrade) != rke2componentsCmdsLength {
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
