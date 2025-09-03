package customflag

import (
	"os"
	"strings"
)

func validateFromJenkins(argsFromJenkins string) (command, testTag string, expectedValues, expectedUpgrades []string) {
	command = extractCmds(argsFromJenkins)
	testTag = validateTestTagFromJenkins(argsFromJenkins)
	if command == "" && testTag == "versionbump" {
		log.Error("cmd was not sent for versionbump test tag versionbump")
		os.Exit(1)
	} else if testTag != "versionbump" && command != "" {
		log.Errorf("cmd can not be sent for this test tag %s", testTag)
		os.Exit(1)
	}

	if !strings.Contains(argsFromJenkins, "expectedValue") {
		log.Errorf("expected value was not sent in %s", argsFromJenkins)
		os.Exit(1)
	}

	validateUpgradeFromJenkins(argsFromJenkins)

	expectedValues, expectedUpgrades = extractExpectedValues(argsFromJenkins)

	return command, testTag, expectedValues, expectedUpgrades
}

// extractExpectedValues validates if the expected value was sent and if the expected value after upgrade was sent too.
// It returns the expected values for the test and the expected values after upgrade even if empty.
func extractExpectedValues(testArgs string) (expectedValues, valuesUpgrades []string) {
	fields := strings.Fields(testArgs)
	keyValueResult := make(map[string][]string)

	for _, arg := range fields {
		if strings.Contains(arg, "=") {
			kvPair := strings.SplitN(arg, "=", 2)
			k := strings.TrimSpace(kvPair[0])
			v := strings.Split(kvPair[1], ",")

			keyValueResult[k] = v
		}
	}

	expectedValues = keyValueResult["-expectedValue"]
	valuesUpgrades = keyValueResult["-expectedValueUpgrade"]

	return expectedValues, valuesUpgrades
}

func extractCmds(testArgs string) string {
	log.Info("testArgs: ", testArgs)
	cmdStart := strings.Index(testArgs, "-cmd=")
	log.Info("cmdStart: ", cmdStart)
	cmdEnd := strings.Index(testArgs, "-expectedValue=")
	log.Info("cmdEnd: ", cmdEnd)
	if cmdStart == -1 || cmdEnd == -1 {
		log.Debugf("cmd or expected value not found in test args %v", testArgs)
		return ""
	}

	cmd := strings.TrimSpace(testArgs[cmdStart:cmdEnd])
	cmd = strings.TrimSpace(cmd[strings.Index(cmd, "=")+1:])

	return cmd
}

// validateUpgradeFromJenkins validates if the upgrade flag was sent and...
// if the expected value after upgrade was sent too inside the testArgs.
func validateUpgradeFromJenkins(testArgs string) {
	if strings.Contains(testArgs, "-installVersionOrCommit") && !strings.Contains(testArgs, "-expectedValueUpgrade") ||
		!strings.Contains(testArgs, "-installVersionOrCommit") && strings.Contains(testArgs, "-expectedValueUpgrade") {
		log.Error("using upgrade, please provide the expected value after upgrade and the install version or commit")
		os.Exit(1)
	}
}

// validateTestTagFromJenkins validates the test tag that was sent on TEST_ARGS from Jenkins.
func validateTestTagFromJenkins(testArgs string) string {
	args := strings.Split(testArgs, " ")
	expectedTags := map[string]bool{
		"components":  true,
		"versionbump": true,
		"cilium":      true,
		"multus":      true,
		"flannel":     true,
		"canal":       true,
		"calico":      true,
	}

	if !strings.HasPrefix(testArgs, "-tags=") {
		log.Errorf("test tag was not sent: %s", testArgs)
		os.Exit(1)
	}

	for _, arg := range args {
		tags := strings.Split(strings.TrimPrefix(arg, "-tags="), ",")
		for _, tag := range tags {
			tag = strings.TrimSpace(tag)
			if expectedTags[tag] {
				return tag
			}
		}
	}

	return ""
}
