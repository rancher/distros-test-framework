package qase

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	qaseclient "github.com/qase-tms/qase-go/qase-api-client"
)

// newNullableInt64 is a aux function to create a new NullableInt64 to retun a pointer to the value.
func newNullableInt64(value int64) qaseclient.NullableInt64 {
	ptr := qaseclient.NewNullableInt64(&value)
	return *ptr
}

// newNullableString is a aux function to create a new NullableString to return a pointer to the value.
func newNullableString(value string) qaseclient.NullableString {
	ptr := qaseclient.NewNullableString(&value)
	return *ptr
}

// newNullString is a aux function to create a new NullableString to return a pointer to nil.
func newNullString() qaseclient.NullableString {
	ptr := qaseclient.NewNullableString(nil)
	return *ptr
}

// makeClickableLinks is a aux function to make the code location a clickable link to GitHub.
func makeClickableLinks(input string) string {
	lines := strings.Split(input, "\n")
	var updatedLines []string

	removeContainerPath := regexp.MustCompile(`\s*\[0-9a-fA-F]+`)

	for _, line := range lines {
		if !strings.Contains(line, "rancher/distros-test-framework/") {
			continue
		}

		codeLink := strings.Replace(line, "/go/src/", "", 1)
		codeLink = removeContainerPath.ReplaceAllString(codeLink, "")

		// Modify to create a GitHub clickable link.
		codeLink = strings.Replace(codeLink, "distros-test-framework/", "distros-test-framework/blob/main/", 1)
		codeLink = strings.Replace(codeLink, ":", "#", 1)
		codeLink = "https://" + codeLink
		updatedLines = append(updatedLines, codeLink)
	}

	return strings.Join(updatedLines, "\n")
}

func newString(s string) *string {
	return &s
}

func newBool(b bool) *bool {
	return &b
}

func newInt64(i int64) *int64 {
	return &i
}

func normalizeSuiteName(name string) string {
	name = strings.TrimPrefix(name, "Test_")
	name = strings.TrimPrefix(name, "E2E")

	return strings.ToLower(name)
}

func isValidTestState(state string) bool {
	return state == "failed" || state == "passed" || state == "skipped"
}

func isCompletionAction(action string) bool {
	return action == "fail" || action == "pass" || action == "skip"
}

func readFullLogFile(fileName string) (string, error) {
	logs, err := os.ReadFile(fileName)
	if err != nil {
		return "", fmt.Errorf("error reading file: %w", err)
	}

	return string(logs), nil
}

func readLogsFromFile(fileName string) ([]goTestData, error) {
	file, err := os.Open(fileName)
	if err != nil {
		return nil, fmt.Errorf("error opening file: %w", err)
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	rowData := make([]goTestData, 0)

	for decoder.More() {
		var row goTestData
		if err := decoder.Decode(&row); err != nil {
			return nil, fmt.Errorf("error decoding json: %w", err)
		}
		rowData = append(rowData, row)
	}

	return rowData, nil
}

func formatTotalTime(start, end time.Time) string {
	duration := end.Sub(start)
	if duration.Seconds() < secondsPerMinute {
		return fmt.Sprintf("%.2f s", duration.Seconds())
	}

	minutes := int(duration.Minutes())
	seconds := int(duration.Seconds()) % secondsPerMinute

	return fmt.Sprintf("%dm:%ds", minutes, seconds)
}
