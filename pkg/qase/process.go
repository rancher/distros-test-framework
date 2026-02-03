package qase

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/rancher/distros-test-framework/shared"
)

// Output map test results from "Output" field from log file.
type Output struct {
	State    string  `json:"state"`
	Name     string  `json:"name"`
	Type     string  `json:"type"`
	Time     float64 `json:"time"`
	ErrorLog string
}

// goTestData to store all fields from log file.
type goTestData struct {
	Time    time.Time
	Action  string
	Package string
	Test    string
	Output  string
	Elapsed float64
}

// processedTestdata to store formatted data.
type processedTestdata struct {
	totalTestTime    string
	testDate         string
	failedTests      int
	passedTests      int
	skippedTests     int
	testSummary      []testOverview
	testSuiteSummary []testSuiteDetails
}

type testSuiteDetails struct {
	planID        int64
	testSuiteName string
	elapsedTime   float64
	status        string
	failedTests   int
	passedTests   int
	skippedTests  int
}

type testDetails struct {
	caseID         int64
	testSuiteName  string
	testCaseName   string
	elapsedTime    float64
	status         string
	errorLog       string
	failureDetails *FailureDetails
}

// FailureDetails contains structured information about a test failure.
type FailureDetails struct {
	ErrorType       string  `json:"error_type"`
	FailedCommand   string  `json:"failed_command,omitempty"`
	TimeoutDuration string  `json:"timeout_duration,omitempty"`
	ErrorMessage    string  `json:"error_message"`
	StackTrace      string  `json:"stack_trace,omitempty"`
	Duration        float64 `json:"duration_seconds"`
	TestSuite       string  `json:"test_suite"`
	TestCase        string  `json:"test_case"`
}

// testOverview to store a summary of the run.
type testOverview struct {
	testSuiteName string
	testCases     []testDetails
}

type status struct {
	passed, failed, skipped int
}

const (
	nanosToMinutes   = 1000 * 1000 * 1000 * 60
	secondsPerMinute = 60
)

// Pre-compiled regexes to avoid repeated compilation in loops.
var (
	ansiEscapeRegex = regexp.MustCompile(`\x1b\[[0-9;]*[mK]`)
	timeoutRegex    = regexp.MustCompile(`Timed out after ([0-9.]+s)`)
	failedCmdRegex  = regexp.MustCompile(`failed cmd:\s*(.+)`)
)

// processTestData reads the log file and processes the data updating the test details and test suite details.
// ci-arch parameter is optional - if empty, defaults to "amd64" for backwards compatibility.
//
//nolint:funlen // complex parsing logic better keep one function.
func processTestData(fileName, product, ciArch string) (*processedTestdata, error) {
	if ciArch == "" {
		ciArch = "amd64"
	}

	remainingData = ""
	data, readErr := parseLogsFromFile(fileName)
	if readErr != nil {
		return nil, fmt.Errorf("error reading log file: %w", readErr)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("no data found in the log file %s", fileName)
	}

	fullLog, err := readFullLogFile(fileName)
	if err != nil {
		return nil, fmt.Errorf("failed to read full log file: %w", err)
	}

	var (
		allTests      []testDetails
		allSuites     []testSuiteDetails
		suiteStatus   status
		testSuiteName string
	)

	suitesWithEmbeddedJSON := make(map[string]bool)
	processedSuites := make(map[string]bool)

	for _, row := range data {
		if row.Test != "" {
			testSuiteName = row.Test
			if err := updateTestDetails(&allTests, &row, &suiteStatus, fullLog, suitesWithEmbeddedJSON); err != nil {
				return nil, fmt.Errorf("error updating test details: %w", err)
			}
			// Handle test completion from Action field.
			if isCompletionAction(row.Action) {
				state := actionToState(row.Action)

				// For tests without embedded JSON (docker tests), add test details.
				if !suitesWithEmbeddedJSON[row.Test] {
					var errorLog string
					var failureDetails *FailureDetails
					if state == failStatus {
						errorLog = extractErrorLogFromContent(fullLog, row.Test)
						failureDetails = extractFailureDetails(fullLog, row.Test, row.Test, row.Elapsed)
					}
					td := testDetails{
						testSuiteName: row.Test,
						// use test name as case name for k3s docker tests.
						testCaseName:   row.Test,
						elapsedTime:    row.Elapsed,
						status:         state,
						errorLog:       errorLog,
						failureDetails: failureDetails,
					}
					allTests = append(allTests, td)
					switch state {
					case failStatus:
						suiteStatus.failed++
					case passStatus:
						suiteStatus.passed++
					case skipStatus:
						suiteStatus.skipped++
					}
				} else if state == failStatus && suiteStatus.failed == 0 {
					// For tests WITH embedded JSON that failed overall but had no individual failures,
					// add the suite-level failure (e.g., test crashed before/after individual tests).
					errorLog := extractErrorLogFromContent(fullLog, row.Test)
					failureDetails := extractFailureDetails(fullLog, row.Test, row.Test, row.Elapsed)
					td := testDetails{
						testSuiteName:  row.Test,
						testCaseName:   row.Test,
						elapsedTime:    row.Elapsed,
						status:         failStatus,
						errorLog:       errorLog,
						failureDetails: failureDetails,
					}
					allTests = append(allTests, td)
					suiteStatus.failed++
				}

				updateTestSuiteDetails(&allSuites, testSuiteName, &row, &suiteStatus)
				processedSuites[testSuiteName] = true
				suiteStatus = status{}
			}
		} else if isCompletionAction(row.Action) && !processedSuites[testSuiteName] {
			// process package-level completion if we haven't already processed the test suite.
			updateTestSuiteDetails(&allSuites, testSuiteName, &row, &suiteStatus)
			// reset counters for next suite.
			suiteStatus = status{}
		}
	}

	return processData(data, allTests, allSuites, product, ciArch), nil
}

// actionToState converts go test JSON Action to state string.
func actionToState(action string) string {
	switch action {
	case "pass":
		return passStatus
	case "fail":
		return failStatus
	case "skip":
		return skipStatus
	default:
		return ""
	}
}

// updateTestDetails gets the data from "Output" field and updates the test details.
func updateTestDetails(
	allTests *[]testDetails,
	row *goTestData,
	s *status,
	fullLog string,
	suitesWithEmbeddedJSON map[string]bool,
) error {
	outputRes := extractTestOutput(row.Output)

	// add this suite as having embedded JSON if we found results.
	if len(outputRes) > 0 {
		suitesWithEmbeddedJSON[row.Test] = true
	}

	for _, out := range outputRes {
		errorLog := out.ErrorLog
		var failureDetails *FailureDetails

		if out.State == "failed" {
			errorLog = extractErrorLogFromContent(fullLog, row.Test)
			failureDetails = extractFailureDetails(fullLog, row.Test, out.Name, out.Time/nanosToMinutes)
		}

		td := testDetails{
			testSuiteName:  row.Test,
			testCaseName:   out.Name,
			elapsedTime:    out.Time / nanosToMinutes,
			status:         out.State,
			errorLog:       errorLog,
			failureDetails: failureDetails,
		}

		*allTests = append(*allTests, td)

		switch out.State {
		case failStatus:
			s.failed++
		case passStatus:
			s.passed++
		case skipStatus:
			s.skipped++
		}
	}

	return nil
}

// remainingData is used to store the remaining log content after extracting the test output.
// this is being used when we have a test output that spans multiple lines but for the same test.
var remainingData string

// extractTestOutput processes raw test output data by removing any ANSI escape sequences,
// storing incomplete input and extracting JSON blocks representing individual test results,
// taking into account the possible remaining data from previous calls.
// It returns a slice of Output structs representing the test results.
//
//nolint:funlen // urhg, yeah it's long lets keep as is.
func extractTestOutput(res string) []Output {
	res = ansiEscapeRegex.ReplaceAllString(res, "")

	// append the cleaned input to the global "remainingData".
	remainingData += res

	var currentOutput *Output
	var results []Output

	i := 0
	for {
		// Find next opening brace.
		nextBraceIdx := strings.Index(remainingData[i:], "{")
		if nextBraceIdx == -1 {
			// If no more supposedly JSON stuff...
			// If a current test output is marked as failed, append the remaining text to its error log and break.
			if currentOutput != nil && currentOutput.State == "failed" {
				currentOutput.ErrorLog += remainingData[i:]
			}
			break
		}

		// Move to the next brace.
		nextBraceIdx += i

		// If there's any data between the current position and the next '{', and if the current test
		// has failed status, add this data to its error log.
		if nextBraceIdx > i && currentOutput != nil && currentOutput.State == "failed" {
			currentOutput.ErrorLog += remainingData[i:nextBraceIdx]
		}

		// Now set boundaries of the JSON by finding match closing brace.
		start := nextBraceIdx
		braceCount := 0
		found := false
		end := start

		// Now we go over the data start from the found '{'.
		for j := start; j < len(remainingData); j++ {
			if remainingData[j] == '{' {
				braceCount++
			} else if remainingData[j] == '}' {
				braceCount--
				// When the braces are correct "{...}"  we found the end. We break the loop.
				if braceCount == 0 {
					end = j
					found = true
					break
				}
			}
		}

		// if we didn't find the end, we break the loop and continue with the next iteration.
		if !found {
			break
		}

		// Ok now we get the JSON string from the boundaries we set above.
		// Like starting in the first field and ending in the last.
		// {"state":"passed","name":"TestK3s","type":"k3s test","time":0.000000}
		jsonStr := remainingData[start : end+1]

		var testOutput Output
		if err := json.Unmarshal([]byte(jsonStr), &testOutput); err == nil {
			if (strings.Contains(testOutput.Type, "k3s test") || strings.Contains(testOutput.Type, "rke2 test")) &&
				isValidTestState(testOutput.State) {
				// Update the current test output.
				if currentOutput != nil {
					results = append(results, *currentOutput)
				}
				// Start new test output.
				currentOutput = &testOutput

				// if JSON block does not represent a test output and a test is active but failed state,
				// append this block to its error log.
			} else if currentOutput != nil && currentOutput.State == "failed" {
				currentOutput.ErrorLog += jsonStr
			}
		} else {
			shared.LogLevel("warn", "error unmarshalling json: %v", err)
			// IF we can't unmarshal the JSON block, instead of exit or error out
			// Instead of exiting, we add this data to the error log if the state is failed,
			// because we might want extra logs for failed tests.
			if currentOutput != nil && currentOutput.State == "failed" {
				currentOutput.ErrorLog += jsonStr
			}
		}
		// Go to the next character after the closing brace.
		i = end + 1
	}

	// Any active test output is added to the results slice.
	if currentOutput != nil {
		results = append(results, *currentOutput)
	}

	// Remove the already processed portion from 'remainingData'
	// so that only unprocessed data remains to be processed in the next call.
	if i < len(remainingData) {
		remainingData = remainingData[i:]
	} else {
		remainingData = ""
	}

	return results
}

// extractFailureDetails extracts structured failure information from the log content.
//
//nolint:funlen // again, yes.
func extractFailureDetails(content, testSuite, testCase string, duration float64) *FailureDetails {
	details := &FailureDetails{
		TestSuite: testSuite,
		TestCase:  testCase,
		Duration:  duration,
		ErrorType: "Unknown",
	}

	lines := strings.Split(content, "\n")
	var outputLines []string
	inFailureSection := false
	summarizingLineCount := 0

	for _, line := range lines {
		var data struct {
			Test   string `json:"Test"`
			Output string `json:"Output"`
		}

		if err := json.Unmarshal([]byte(line), &data); err != nil {
			continue
		}

		if data.Test != testSuite {
			continue
		}

		// clean the output for ANSI codes.
		cleanOutput := ansiEscapeRegex.ReplaceAllString(data.Output, "")
		cleanOutput = strings.TrimRight(cleanOutput, "\n\r")

		if cleanOutput == "" {
			continue
		}

		if strings.Contains(cleanOutput, "[FAILED]") {
			inFailureSection = true
		}

		if inFailureSection {
			outputLines = append(outputLines, cleanOutput)

			// After "Summarizing" line, continue for a few more lines to capture the summary.
			if strings.Contains(cleanOutput, "Summarizing") {
				summarizingLineCount = 1
			} else if summarizingLineCount > 0 {
				summarizingLineCount++
				// 10 more lines after "Summarizing" to get the full summary.
				if summarizingLineCount > 10 {
					break
				}
			}
		}

		if strings.Contains(cleanOutput, "Timed out after") {
			timeoutMatch := timeoutRegex.FindStringSubmatch(cleanOutput)
			if len(timeoutMatch) > 1 {
				details.TimeoutDuration = timeoutMatch[1]
				details.ErrorType = "Timeout"
			}
		}

		if strings.Contains(cleanOutput, "failed cmd:") {
			cmdMatch := failedCmdRegex.FindStringSubmatch(cleanOutput)
			if len(cmdMatch) > 1 {
				details.FailedCommand = strings.TrimSpace(cmdMatch[1])
			}
		}

		switch {
		case strings.Contains(cleanOutput, "Unexpected error"):
			details.ErrorType = "UnexpectedError"
		case strings.Contains(cleanOutput, "exit status") && details.ErrorType == "Unknown":
			details.ErrorType = "ExitError"
		case strings.Contains(cleanOutput, "connection refused"):
			details.ErrorType = "ConnectionRefused"
		case strings.Contains(cleanOutput, "failed to run") && details.ErrorType == "Unknown":
			details.ErrorType = "CommandFailed"
		}

		if details.FailedCommand == "" && strings.Contains(cleanOutput, "failed to run") {
			details.FailedCommand = cleanOutput
		}
	}

	if len(outputLines) > 0 {
		details.ErrorMessage = strings.Join(outputLines, "\n")
	}

	// truncate if it is now too long.
	if len(details.ErrorMessage) > 6000 {
		details.ErrorMessage = details.ErrorMessage[:6000] + "\n... (truncated)"
	}

	return details
}

func extractErrorLogFromContent(content, testSuite string) string {
	content = ansiEscapeRegex.ReplaceAllString(content, "")

	var builder strings.Builder
	lines := strings.Split(content, "\n")
	suiteName := strings.ToLower(testSuite)

	inFailureBlock := false
	lineCount := 0
	maxLines := 50

	for _, line := range lines {
		lineLower := strings.ToLower(line)

		if strings.Contains(lineLower, suiteName) &&
			(strings.Contains(lineLower, "failed") || strings.Contains(lineLower, "fail")) {
			inFailureBlock = true
		}

		if inFailureBlock {
			trimmedLine := strings.TrimSpace(line)
			if trimmedLine == "" || strings.HasPrefix(trimmedLine, "{\"Time\"") {
				continue
			}

			builder.WriteString(line)
			builder.WriteString("\n")
			lineCount++

			if lineCount >= maxLines || strings.Contains(line, "--- PASS:") ||
				(strings.Contains(line, "--- FAIL:") && lineCount > 5) {
				break
			}
		}
	}

	return builder.String()
}

func updateTestSuiteDetails(
	allSuites *[]testSuiteDetails,
	suiteName string,
	row *goTestData,
	s *status,
) {
	suite := testSuiteDetails{
		testSuiteName: suiteName,
		elapsedTime:   row.Elapsed / secondsPerMinute,
		status:        row.Action,
		failedTests:   s.failed,
		passedTests:   s.passed,
		skippedTests:  s.skipped,
	}

	for i, existing := range *allSuites {
		if existing.testSuiteName == suiteName {
			(*allSuites)[i] = suite
			return
		}
	}

	*allSuites = append(*allSuites, suite)
}

// processData formats the data into a processedTestdata struct updating it.
// so it can be used to parse and report the data to Qase.
func processData(
	data []goTestData,
	allTests []testDetails,
	allSuites []testSuiteDetails,
	product string,
	ciArch string,
) *processedTestdata {
	testSummary := make([]testOverview, 0, len(allSuites))
	filteredSuites := make([]testSuiteDetails, 0, len(allSuites))
	totalStats := status{}

	for _, suite := range allSuites {
		// filter k3s tests based on architecture.
		if product == "k3s" && !shouldIncludeTest(suite.testSuiteName, ciArch) {
			continue
		}

		if product == "rke2" {
			suite.planID = 16
		} else {
			suite.planID = 17
		}

		var testCases []testDetails
		for _, td := range allTests {
			if td.testSuiteName == suite.testSuiteName {
				if cID, err := extractIDs(td.testSuiteName, product, ciArch); err != nil {
					shared.LogLevel("error", "error extracting caseID for suite %s: %v", td.testSuiteName, err)
				} else {
					td.caseID = cID
				}
				testCases = append(testCases, td)
			}
		}

		testSummary = append(testSummary, testOverview{
			testSuiteName: suite.testSuiteName,
			testCases:     testCases,
		})

		filteredSuites = append(filteredSuites, suite)

		totalStats.failed += suite.failedTests
		totalStats.passed += suite.passedTests
		totalStats.skipped += suite.skippedTests
	}

	return &processedTestdata{
		totalTestTime:    formatTotalTime(data[0].Time, data[len(data)-1].Time),
		testDate:         data[0].Time.Format(time.RFC850),
		failedTests:      totalStats.failed,
		passedTests:      totalStats.passed,
		skippedTests:     totalStats.skipped,
		testSummary:      testSummary,
		testSuiteSummary: filteredSuites,
	}
}

// GetFailedTestDetails returns a slice of FailureDetails for all failed tests.
func (pd *processedTestdata) GetFailedTestDetails() []*FailureDetails {
	var failures []*FailureDetails
	for _, overview := range pd.testSummary {
		for _, tc := range overview.testCases {
			if tc.status == failStatus && tc.failureDetails != nil {
				failures = append(failures, tc.failureDetails)
			}
		}
	}

	return failures
}

//nolint:funlen // test case ID mapping with many entries
func extractIDs(suiteName, product, arch string) (int64, error) {
	var tcID int64
	name := normalizeSuiteName(suiteName)

	if product == "rke2" {
		rke2Tcs := map[string]int64{
			"ciliumnokp":            220,
			"calico_ebpf":           284,
			"mixedosvalidation":     223,
			"mixedosbgpvalidation":  224,
			"secretsencryption_old": 296,
			"multus":                225,
			"secretsencryption":     226,
			"splitserver":           227,
			"upgradevalidation":     228,
			"clustervalidation":     229,
			"kinevalidation":        230,
			"ciliumwireguard":       295,
		}

		for keyword, id := range rke2Tcs {
			if strings.Contains(name, keyword) {
				tcID = id
				break
			}
		}
	} else {
		// docker tests for amd64.
		k3sDockerAmd64Tcs := map[string]int64{
			"dockerdualstack":              231,
			"dockerrotateca":               237,
			"dockerautoimport":             304,
			"dockerbasic":                  305,
			"dockerbootstraptoken":         306,
			"dockercacerts":                307,
			"dockerconformance":            308,
			"dockeretcd":                   309,
			"dockerhardened":               310,
			"dockerlazypull":               311,
			"dockersecretsencryption":      312,
			"dockerskew":                   313,
			"dockersnapshotrestore":        314,
			"dockersvcpoliciesandfirewall": 315,
			"dockertoken":                  316,
			"dockerupgrade":                317,
		}

		// docker tests for arm64 (only tests that run on arm64).
		k3sDockerArm64Tcs := map[string]int64{
			"dockerbasic":          320,
			"dockerbootstraptoken": 321,
			"dockercacerts":        322,
			"dockeretcd":           323,
			"dockerhardened":       324,
			"dockerlazypull":       325,
			"dockerskew":           326,
			"dockertoken":          327,
			"dockerupgrade":        328,
		}

		// vagrant e2e tests - arch independent.
		k3sVagrantTcs := map[string]int64{
			"clustervalidation": 232,
			"secretsencryption": 233,
			"splitserver":       234,
			"startup":           235,
			"externalip":        236,
			"wasm":              297,
			"btrfs":             298,
			"embeddedmirror":    299,
			"multus":            300,
			"privateregistry":   301,
			"rootless":          302,
			"s3":                303,
			"tailscale":         318,
			"dualstack":         319,
		}

		isDocker := strings.Contains(name, "docker")
		if isDocker {
			var dockerTcs map[string]int64
			if arch == "arm64" {
				dockerTcs = k3sDockerArm64Tcs
			} else {
				dockerTcs = k3sDockerAmd64Tcs
			}
			for keyword, id := range dockerTcs {
				if strings.Contains(name, keyword) {
					tcID = id
					break
				}
			}
		}

		if tcID == 0 {
			for keyword, id := range k3sVagrantTcs {
				if strings.Contains(name, keyword) {
					tcID = id
					break
				}
			}
		}
	}

	if tcID == 0 {
		return 0, fmt.Errorf("no matching test case found for suite: %s", suiteName)
	}

	return tcID, nil
}

// shouldIncludeTest determines if a test should be included based on architecture.
// For k3s e2e arm64, only include docker tests that run on arm64 (exclude vagrant and amd64-only docker tests).
// For k3s e2e amd64, include all tests (vagrant + all docker amd64 tests).
func shouldIncludeTest(suiteName, arch string) bool {
	name := normalizeSuiteName(suiteName)
	isDocker := strings.Contains(name, "docker")

	if arch == "arm64" {
		if !isDocker {
			return false
		}

		// docker tests excluded from arm64.
		arm64ExcludedTests := []string{
			"dockerautoimport",
			"dockerdualstack",
			"dockersecretsencryption",
			"dockersnapshotrestore",
			"dockersvcpoliciesandfirewall",
		}
		for _, excluded := range arm64ExcludedTests {
			if strings.Contains(name, excluded) {
				return false
			}
		}

		return true
	}

	return true
}
