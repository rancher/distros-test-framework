package qase

import (
	"context"
	"fmt"
	"os"
	"strconv"

	. "github.com/onsi/ginkgo/v2"
	qaseclient "github.com/qase-tms/qase-go/qase-api-client"

	"github.com/rancher/distros-test-framework/shared"
)

const (
	failStatus = "failed"
	passStatus = "passed"
)

var (
	qaseRunID = os.Getenv("QASE_RUN_ID")
	caseID    = os.Getenv("QASE_TEST_CASE_ID")
	projectID = os.Getenv("QASE_PROJECT_ID")
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

// ReportTestResults receives the report from ginkgo and sends the test results to Qase.
func (c Client) ReportTestResults(ctx context.Context, report *Report, version string) {
	shared.LogLevel("info", "Start publishing test results to Qase\n")

	runID, tcID := validateQaseIDs()

	req := createResultRequest{
		projectID: projectID,
		status:    "",
		runID:     int32(runID),
		caseID:    tcID,
		time:      newNullableInt64(int64(report.RunTime.Seconds())),
		comment:   newNullString(),
	}

	tcs, _ := reportToTestCase(report)

	request := parseResults(tcs, version, &req)

	if err := c.createTestResult(ctx, request); err != nil {
		shared.LogLevel("error", "failed to create test result: %w\n", err)
	}
}

// validateQaseIDs validates the Qase Run ID and Test Case ID and returns parsed as int and int64 respectively.
func validateQaseIDs() (runID int, tcID *int64) {
	if projectID == "" {
		shared.LogLevel("error", "QASE_PROJECT_ID is not set")
	}

	if qaseRunID == "" {
		shared.LogLevel("error", "QASE_RUN_ID is not set")
	}

	runID, err := strconv.Atoi(qaseRunID)
	if err != nil {
		shared.LogLevel("error", "invalid QASE_RUN_ID: %w\n", err)
	}

	caseIDInt, err := strconv.ParseInt(caseID, 10, 64)
	if err != nil {
		shared.LogLevel("error", "invalid QASE_TEST_CASE_ID: %w\n", err)
	}
	tcID = &caseIDInt

	return runID, tcID
}

// reportToTestCase receives the test results report and unpacks them into a slice of TestCase type.
//
// returns the slice of TestCase and a boolean indicating if the suite succeeded.
func reportToTestCase(report *Report) ([]TestCase, bool) {
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

// createTestResult receives the the createResultRequest and sends the request to create a test result in Qase.
func (c Client) createTestResult(ctx context.Context, req *createResultRequest) error {
	qaseRequest := qaseclient.ResultCreate{
		CaseId:  req.caseID,
		Status:  req.status,
		Time:    req.time,
		Comment: req.comment,
	}

	create := c.QaseAPI.ResultsAPI.CreateResult(ctx, req.projectID, req.runID).ResultCreate(qaseRequest)
	res, httpRes, err := create.Execute()
	if err != nil {
		return fmt.Errorf("failed to create test result: %w, response: %v", err, httpRes)
	}

	shared.LogLevel("info", "Test result created: %v\n", &res.Status)

	return nil
}
