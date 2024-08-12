package customflag

import (
	"os"
	"strings"
)

// validateExpectedValues validates if the expected value was sent and if the expected value after upgrade was sent too.
// It returns the expected values for the test and the expected values after upgrade if exists.
func validateExpectedValues(testArgs string) (expectedValues, expectedValuesUpgrade []string) {

	cleanedTestArgs := strings.Join(strings.Fields(testArgs), " ")
	sliceArgs := strings.Split(cleanedTestArgs, " ")

	keyValueResult := make(map[string][]string)

	for _, arg := range sliceArgs {
		if strings.Contains(arg, "=") {

			kvPair := strings.SplitN(arg, "=", 2)

			key := strings.TrimSpace(kvPair[0])
			values := strings.Split(kvPair[1], ",")
			keyValueResult[key] = values
		}
	}

	expected := keyValueResult["-expectedValue"]
	valuesUpgrade := keyValueResult["-expectedValueUpgrade"]

	if len(valuesUpgrade) == 0 {
		valuesUpgrade = []string{}

		return expected, valuesUpgrade
	}

	return expected, valuesUpgrade
}

// validateUpgrade validates if the upgrade flag was sent and if the expected value after upgrade was sent too.
func validateUpgrade(testArgs string) {
	if strings.Contains(testArgs, "-installVersionOrCommit") && !strings.Contains(testArgs, "-expectedValueUpgrade") ||
		!strings.Contains(testArgs, "-installVersionOrCommit") && strings.Contains(testArgs, "-expectedValueUpgrade") {
		log.Errorf("using upgrade, please provide the expected value after upgrade and the install version or commit")
		os.Exit(1)
	}
}

// validateTestTag validates the test tag that was sent on TEST_ARGS.
func validateTestTag(testArgs string) string {
	args := strings.Split(testArgs, " ")

	expectedTags := map[string]bool{
		"components":  true,
		"versionbump": true,
		"cilium":      true,
		"multus":      true,
		"flannel":     true,
	}

	for _, arg := range args {
		if strings.HasPrefix(arg, "-tags=") {
			tags := strings.Split(strings.TrimPrefix(arg, "-tags="), ",")
			for _, tag := range tags {
				tag = strings.TrimSpace(tag)
				if expectedTags[tag] {
					return tag
				}
			}
		}
	}

	return ""
}
