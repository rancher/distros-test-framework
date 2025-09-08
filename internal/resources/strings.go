package resources

import (
	"fmt"
	"slices"
	"strings"
)

// CleanString removes spaces and new lines from a string.
func CleanString(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(s), "\n", ""), " ", "")
}

// CleanSliceStrings removes spaces and new lines from a slice of strings.
func CleanSliceStrings(stringsSlice []string) []string {
	for i, str := range stringsSlice {
		stringsSlice[i] = CleanString(str)
	}

	return stringsSlice
}

// SliceContainsString verify if a string is found in the list of strings.
func SliceContainsString(list []string, a string) bool {
	for _, b := range list {
		if strings.Contains(a, b) {
			return true
		}
	}

	return false
}

// CountOfStringInSlice Used to count the pods using prefix passed in the list of pods.
func CountOfStringInSlice(str string, pods []Pod) int {
	var count int
	for i := range pods {
		if strings.Contains(pods[i].Name, str) {
			count++
		}
	}

	return count
}

// MatchWithPath verify expected files found in the actual file list.
func MatchWithPath(actualFileList, expectedFileList []string) error {
	for i := 0; i < len(expectedFileList); i++ {
		if !slices.Contains(actualFileList, expectedFileList[i]) {
			return ReturnLogError("FAIL: Expected file: %s NOT found in actual list",
				expectedFileList[i])
		}
		LogLevel("info", "PASS: Expected file %s found", expectedFileList[i])
	}

	for i := 0; i < len(actualFileList); i++ {
		if !slices.Contains(expectedFileList, actualFileList[i]) {
			LogLevel("info", "Actual file %s found as well which was not in the expected list",
				actualFileList[i])
		}
	}

	return nil
}

// EncloseSqBraces encloses a string in square braces.
func EncloseSqBraces(ip string) string {
	return "[" + ip + "]"
}

// LogGrepOutput
// Grep for a particular text/string (content) in a file (filename) on a node with 'ip' and log the same.
// Ex: Log content:'denied' calls in filename:'/var/log/audit/audit.log' file.
func LogGrepOutput(filename, content, ip string) {
	cmd := fmt.Sprintf("sudo cat %s | grep %s", filename, content)
	grepData, grepErr := RunCommandOnNode(cmd, ip)
	if grepErr != nil {
		LogLevel("error", "error getting grep %s log for %s calls", filename, content)
	}
	if grepData != "" {
		LogLevel("debug", "grep for %s in file %s output:\n %s", content, filename, grepData)
	}
}

func NormalizeString(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\\n", "\n")

	return s
}
