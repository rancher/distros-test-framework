package qase

import (
	"context"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	qaseclient "github.com/qase-tms/qase-go/qase-api-client"

	"github.com/rancher/distros-test-framework/shared"
)

const (
	failStatus = "failed"
	passStatus = "passed"
	skipStatus = "skipped"
)

var (
	qaseRunID = os.Getenv("QASE_RUN_ID")
	caseID    = os.Getenv("QASE_TEST_CASE_ID")
	projectID = "K3SRKE2"
)

// TestCase is the struct used for sending the appropriate API call to Qase.
type TestCase struct {
	Name       string
	Status     string
	StackTrace Failures
	Elapsed    int64
	CaseID     int64
}

// Failures contains detailed information about a test failure.
type Failures struct {
	Message        string
	Location       string
	CodeLocation   string
	FullStackTrace string
}

// createResultRequest is the struct used to create a test result in Qase.
type createResultRequest struct {
	projectID string
	status    string
	runID     int32
	caseID    *int64
	time      qaseclient.NullableInt64
	comment   qaseclient.NullableString
}

func (c Client) ReportRun(fileName, product string) error {
	pd, processTestDataErr := processTestData(fileName, product)
	if processTestDataErr != nil {
		return fmt.Errorf("error processing test data: %w", processTestDataErr)
	}

	id, createErr := c.createRun(pd, "e2e", product)
	if createErr != nil {
		return fmt.Errorf("error creating run: %w", createErr)
	}

	// Checking if the runID is within the int32 bounds to avoid integer overflow.
	if *id < int64(math.MinInt32) || *id > int64(math.MaxInt32) {
		return fmt.Errorf("runID %d out of int32 bounds", *id)
	}
	runID := int32(*id)

	tcs := testSuiteDetailsToTestCase(pd.testSummary)

	resultReq := parseBulkResults(tcs, runID)
	if err := c.createBulkTestResult(resultReq); err != nil {
		return fmt.Errorf("error creating bulk test result: %w", err)
	}

	if completeErr := c.completeRun(runID); completeErr != nil {
		return fmt.Errorf("error completing run: %w", completeErr)
	}

	return nil
}

// SpecReportTestResults receives the report from ginkgo and sends the test results to Qase.
func (c Client) SpecReportTestResults(ctx context.Context, report *Report, version string) {
	shared.LogLevel("info", "Start publishing test results to Qase\n")

	runID, tcID, err := validateQaseIDs()
	if err != nil {
		shared.LogLevel("error", "failed to validate Qase IDs: %w\n", err)
	}

	req := createResultRequest{
		projectID: projectID,
		status:    "",
		runID:     runID,
		caseID:    tcID,
		time:      newNullableInt64(int64(report.RunTime.Seconds())),
		comment:   newNullString(),
	}

	tcs, _ := specReportToTestCase(report)
	request := parseResults(tcs, version, &req)

	if err := c.createTestResult(ctx, request); err != nil {
		shared.LogLevel("error", "failed to create test result: %w\n", err)
	}
}

// validateQaseIDs validates the Qase Run ID and Test Case ID and returns as int32 and int64 respectively.
func validateQaseIDs() (runID int32, tcID *int64, err error) {
	if projectID == "" {
		shared.LogLevel("error", "QASE_PROJECT_ID is not set")
	}

	if qaseRunID == "" {
		shared.LogLevel("error", "QASE_RUN_ID is not set")
	}

	id, err := strconv.ParseInt(qaseRunID, 10, 32)
	if err != nil {
		return 0, nil, fmt.Errorf("invalid QASE_RUN_ID: %w", err)
	}

	caseIDInt, err := strconv.ParseInt(caseID, 10, 64)
	if err != nil {
		return 0, nil, fmt.Errorf("invalid QASE_TEST_CASE_ID: %w", err)
	}

	return int32(id), &caseIDInt, nil
}

// specReportToTestCase receives the test results report from ginkgo and unpacks them into a slice of TestCase type.
//
// returns the slice of TestCase and a boolean indicating if the suite succeeded.
func specReportToTestCase(report *Report) ([]TestCase, bool) {
	var tcs []TestCase

	for i := range report.SpecReports {
		r := &report.SpecReports[i]
		var f Failures
		if r.State.String() != passStatus {
			f = Failures{
				Message:        r.Failure.Message,
				Location:       r.Failure.Location.String(),
				CodeLocation:   r.Failure.FailureNodeLocation.String(),
				FullStackTrace: r.Failure.Location.FullStackTrace,
			}
		}

		tcs = append(tcs, TestCase{
			Name:       r.LeafNodeText,
			Status:     r.State.String(),
			StackTrace: f,
			Elapsed:    int64(r.RunTime.Seconds()),
		})
	}

	return tcs, report.SuiteSucceeded
}

// parseResults receives the test results and parses the results into the createResultRequest.
func parseResults(testCases []TestCase, version string, req *createResultRequest) *createResultRequest {
	var failedSubTests []TestCase

	for _, tc := range testCases {
		if tc.Status != passStatus {
			failedSubTests = append(failedSubTests, tc)
		}
	}

	if len(failedSubTests) > 0 {
		req.status = failStatus
		var comments string
		for _, tc := range failedSubTests {
			updatedFullStackTrace := makeClickableLinks(tc.StackTrace.FullStackTrace)
			codeLocationLink := makeClickableLinks(tc.StackTrace.CodeLocation)
			stacTraceLocation := makeClickableLinks(tc.StackTrace.Location)

			comments += fmt.Sprintf(
				"Failed test:\nVersion Tested: %s\nName: %s\nStatus: %s\nMessage: %s\n"+
					"Location: \n%s\n\nCodeLocation: \n%s\n\nFullStackTrace: \n%s\n\n", version,
				tc.Name, tc.Status, tc.StackTrace.Message, stacTraceLocation,
				codeLocationLink, updatedFullStackTrace,
			)
		}
		req.comment = newNullableString(comments)
	} else {
		req.status = passStatus
		req.comment = newNullableString(fmt.Sprintf("Version Tested: %s\n", version))
	}

	return req
}

func testSuiteDetailsToTestCase(testOverview []testOverview) []TestCase {
	var testCases []TestCase

	for _, overview := range testOverview {
		for _, td := range overview.testCases {
			var stackTrace Failures
			if td.status == failStatus {
				stackTrace = Failures{
					Message: td.errorLog,
				}
			}

			tc := TestCase{
				Name:       td.testSuiteName + " " + td.testCaseName,
				Status:     td.status,
				StackTrace: stackTrace,
				Elapsed:    int64(td.elapsedTime),
				CaseID:     td.caseID,
			}

			testCases = append(testCases, tc)
		}
	}

	return testCases
}

func parseBulkResults(testCases []TestCase, runID int32) []createResultRequest {
	caseGroups := make(map[int64][]TestCase)
	for _, tc := range testCases {
		if tc.CaseID <= 0 {
			continue
		}
		caseGroups[tc.CaseID] = append(caseGroups[tc.CaseID], tc)
	}

	var reqs []createResultRequest

	for cid, group := range caseGroups {
		// here we default finalStatus to passStatus and update it to failStatus if any of the sub-tests fail.
		finalStatus := passStatus
		var totalElapsed int64
		var commentBuilder strings.Builder

		commentBuilder.WriteString("Version Tested: Latest master commit ,see link above on description!") // nolint:revive // it is builder string only.

		for _, tc := range group {
			totalElapsed += tc.Elapsed

			if tc.Status == failStatus {
				finalStatus = failStatus
				commentBuilder.WriteString(fmt.Sprintf( // nolint:revive // it is builder string only.
					"\nFailed sub-test: %s\nMessage: %s\n\n",
					tc.Name, tc.StackTrace.Message,
				))
			} else {
				commentBuilder.WriteString(fmt.Sprintf( // nolint:revive // it is builder string only.
					"Passed sub-test: %s\n",
					tc.Name,
				))
			}
		}

		req := createResultRequest{
			projectID: projectID,
			runID:     runID,
			caseID:    newInt64(cid),
			time:      newNullableInt64(totalElapsed),
			status:    finalStatus,
			comment:   newNullableString(commentBuilder.String()),
		}

		reqs = append(reqs, req)
	}

	return reqs
}
