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
	caseID        int64
	testSuiteName string
	testCaseName  string
	elapsedTime   float64
	status        string
	errorLog      string
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

// processTestData reads the log file and processes the data updating the test details and test suite details.
func processTestData(fileName, product string) (*processedTestdata, error) {
	data, readErr := readLogsFromFile(fileName)
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

	for _, row := range data {
		if row.Test != "" {
			testSuiteName = row.Test
			if err := updateTestDetails(&allTests, &row, &suiteStatus, fullLog); err != nil {
				return nil, fmt.Errorf("error updating test details: %w", err)
			}
		} else if isCompletionAction(row.Action) {
			updateTestSuiteDetails(&allSuites, testSuiteName, &row, &suiteStatus)
			// Reseting counters for next suite.
			suiteStatus = status{}
		}
	}

	return processData(data, allTests, allSuites, product), nil
}

// updateTestDetails  gets the data from "Output" field and updates the test details.
func updateTestDetails(
	allTests *[]testDetails,
	row *goTestData,
	s *status,
	fullLog string,
) error {
	outputRes := extractTestOutput(row.Output)

	for _, out := range outputRes {
		errorLog := out.ErrorLog
		if out.State == "failed" {
			errorLog = extractErrorLogFromContent(fullLog, row.Test)
		}

		td := testDetails{
			testSuiteName: row.Test,
			testCaseName:  out.Name,
			elapsedTime:   out.Time / nanosToMinutes,
			status:        out.State,
			errorLog:      errorLog,
		}

		// Append to the slice, preserving insertion order
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
	// Remove ANSI escape sequences.
	ansiEscape := regexp.MustCompile(`\x1b\[[0-9;]*[mK]`)
	res = ansiEscape.ReplaceAllString(res, "")

	// Append the cleaned input to the global "remainingData".
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

func extractErrorLogFromContent(content, testSuite string) string {
	var builder strings.Builder
	lines := strings.Split(content, "\n")
	suiteName := strings.ToLower(testSuite)

	for i, line := range lines {
		if strings.Contains(strings.ToLower(line), suiteName) {
			// here we are just adding next 5 lines after the testSuite name to the error log.
			for j := i; j < i+6 && j < len(lines); j++ {
				_, _ = builder.WriteString(lines[j])
				_, _ = builder.WriteString("\n")
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
	*allSuites = append(*allSuites, suite)
}

// processData formats the data into a processedTestdata struct updating it.
// so it can be used to parse and report the data to Qase.
func processData(
	data []goTestData,
	allTests []testDetails,
	allSuites []testSuiteDetails,
	product string,
) *processedTestdata {
	testSummary := make([]testOverview, 0, len(allSuites))
	totalStats := status{}

	for i, suite := range allSuites {
		if product == "rke2" {
			allSuites[i].planID = 16
		} else {
			allSuites[i].planID = 17
		}

		var testCases []testDetails
		for _, td := range allTests {
			if td.testSuiteName == suite.testSuiteName {
				if cID, err := extractIDs(td.testSuiteName, product); err != nil {
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
		testSuiteSummary: allSuites,
	}
}

func extractIDs(suiteName, product string) (int64, error) {
	var tcID int64
	name := normalizeSuiteName(suiteName)

	if product == "rke2" {
		rke2Tcs := map[string]int64{
			"ciliumnokp":           220,
			"dnscache":             221,
			"dualstack":            222,
			"mixedosvalidation":    223,
			"mixedosbgpvalidation": 224,
			"multus":               225,
			"secretsencryption":    226,
			"splitserver":          227,
			"upgradevalidation":    228,
			"clustervalidation":    229,
			"kinevalidation":       230,
		}

		for keyword, id := range rke2Tcs {
			if strings.Contains(name, keyword) {
				tcID = id
				break
			}
		}
	} else {
		// TODO: Add k3s test cases
		k3sTcs := map[string]int64{
			"dualstack":         231,
			"clustervalidation": 232,
			"secretsencryption": 233,
			"splitserver":       234,
			"startup":           235,
			"externalip":        236,
			"rotateca":          237,
			"snapshotrestore":   238,
		}
		for keyword, id := range k3sTcs {
			if strings.Contains(name, keyword) {
				tcID = id
				break
			}
		}
	}

	if tcID == 0 {
		return 0, fmt.Errorf("no matching test case found for suite: %s", suiteName)
	}

	return tcID, nil
}
