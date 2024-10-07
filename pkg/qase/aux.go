package qase

import (
	"regexp"
	"strings"

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

	extraAddressRegex := regexp.MustCompile(`\s*\+0x[0-9a-fA-F]+`)

	for _, line := range lines {
		if strings.Contains(line, "rancher/distros-test-framework/") {
			codeLink := strings.Replace(line, "/go/src/", "", 1)
			codeLink = extraAddressRegex.ReplaceAllString(codeLink, "")
			// Modify to create a GitHub clickable link
			codeLink = strings.Replace(codeLink, "distros-test-framework/", "distros-test-framework/blob/main/", 1)
			codeLink = strings.Replace(codeLink, ":", "#", 1)
			codeLink = "https://" + codeLink
			updatedLines = append(updatedLines, codeLink)
		}
	}

	return strings.Join(updatedLines, "\n")
}
