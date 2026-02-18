package qase

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
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
	Name           string
	Status         string
	StackTrace     Failures
	Elapsed        int64
	CaseID         int64
	FailureDetails *FailureDetails
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

func (c Client) ReportE2ETestRun(fileName, product, ciArch string) (int32, error) {
	pd, processTestDataErr := processTestData(fileName, product, ciArch)
	if processTestDataErr != nil {
		return 0, fmt.Errorf("error processing test data: %w", processTestDataErr)
	}

	id, createErr := c.createRun(pd, "e2e", product, ciArch)
	if createErr != nil {
		return 0, fmt.Errorf("error creating run: %w", createErr)
	}

	// Checking if the runID is within the int32 bounds to avoid integer overflow.
	if *id < int64(math.MinInt32) || *id > int64(math.MaxInt32) {
		return 0, fmt.Errorf("runID %d out of int32 bounds", *id)
	}
	runID := int32(*id)

	tcs := testSuiteDetailsToTestCase(pd.testSummary)

	resultReq := parseBulkResults(tcs, runID)
	if err := c.createBulkTestResult(resultReq); err != nil {
		return 0, fmt.Errorf("error creating bulk test result: %w", err)
	}

	if completeErr := c.completeRun(runID); completeErr != nil {
		return 0, fmt.Errorf("error completing run: %w", completeErr)
	}

	return runID, nil
}

// SpecReportTestResults receives the report from ginkgo and sends the test results to Qase.
func (c Client) SpecReportTestResults(ctx context.Context, cluster *shared.Cluster, report *Report, reportSummary string) {
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
	request := parseResults(cluster, tcs, reportSummary, &req)

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
func parseResults(
	cluster *shared.Cluster,
	testCases []TestCase,
	reportSummary string,
	req *createResultRequest,
) *createResultRequest {
	testResSummary := tcResultSummary(cluster, reportSummary)
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
					"Location: \n%s\n\nCodeLocation: \n%s\n\nFullStackTrace: \n%s\n\n", cluster.Config.Version,
				tc.Name, tc.Status, tc.StackTrace.Message, stacTraceLocation,
				codeLocationLink, updatedFullStackTrace,
			)
		}
		testResSummary += fmt.Sprintf("\n"+"\n"+"Failed sub-tests:\n%s"+"\n", comments)
		req.comment = newNullableString(testResSummary)
	} else {
		req.status = passStatus
		req.comment = newNullableString(fmt.Sprintf("Version Tested: %s\n", cluster.Config.Version))
		req.comment = newNullableString(testResSummary)
	}

	return req
}

func testSuiteDetailsToTestCase(testOverview []testOverview) []TestCase {
	var testCases []TestCase

	for _, overview := range testOverview {
		for _, td := range overview.testCases {
			var stackTrace Failures
			var failureDetails *FailureDetails

			if td.status == failStatus {
				stackTrace = Failures{
					Message: td.errorLog,
				}
				failureDetails = td.failureDetails
			}

			tc := TestCase{
				Name:           td.testSuiteName + " " + td.testCaseName,
				Status:         td.status,
				StackTrace:     stackTrace,
				Elapsed:        int64(td.elapsedTime),
				CaseID:         td.caseID,
				FailureDetails: failureDetails,
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
		commentBuilder.WriteString("Version Tested: Latest master commit, see link above on description!\n\n")

		for _, tc := range group {
			totalElapsed += tc.Elapsed

			if tc.Status == failStatus {
				finalStatus = failStatus
				commentBuilder.WriteString(fmt.Sprintf(
					"---\n**FAILED Sub-test:** %s\n\n",
					tc.Name,
				))

				if tc.FailureDetails != nil {
					commentBuilder.WriteString(formatFailureDetailsForQase(tc.FailureDetails))
				} else if tc.StackTrace.Message != "" {
					commentBuilder.WriteString(fmt.Sprintf("**Error Message:**\n```\n%s\n```\n\n", tc.StackTrace.Message))
				}
			} else {
				commentBuilder.WriteString(fmt.Sprintf(
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

// formatFailureDetailsForQase formats the FailureDetails struct into a readable string for Qase comments.
func formatFailureDetailsForQase(fd *FailureDetails) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("**Error Type:** %s\n", fd.ErrorType))
	sb.WriteString(fmt.Sprintf("**Duration:** %.2f seconds\n", fd.Duration))

	if fd.TimeoutDuration != "" {
		sb.WriteString(fmt.Sprintf("**Timeout:** %s\n", fd.TimeoutDuration))
	}

	if fd.FailedCommand != "" {
		sb.WriteString(fmt.Sprintf("\n**Failed Command:**\n```\n%s\n```\n", fd.FailedCommand))
	}

	if fd.ErrorMessage != "" {
		// truncate error message if too long for Qase.
		errMsg := fd.ErrorMessage
		if len(errMsg) > 3000 {
			errMsg = errMsg[:3000] + "\n... (truncated)"
		}
		sb.WriteString(fmt.Sprintf("\n**Error Output:**\n```\n%s\n```\n", errMsg))
	}

	if fd.StackTrace != "" {
		// truncate stack trace if too long...
		stackTrace := fd.StackTrace
		if len(stackTrace) > 1000 {
			stackTrace = stackTrace[:1000] + "\n... (truncated)"
		}
		sb.WriteString(fmt.Sprintf("\n**Stack Trace:**\n```\n%s\n```\n", stackTrace))
	}

	if jsonBytes, err := json.MarshalIndent(fd, "", "  "); err == nil {
		sb.WriteString("\n<details>\n<summary>Failure Details JSON</summary>\n\n```json\n")
		_, _ = sb.Write(jsonBytes)
		sb.WriteString("\n```\n</details>\n")
	}

	sb.WriteString("\n")

	return sb.String()
}

func tcResultSummary(c *shared.Cluster, reportSummary string) string {
	var reportSummaryBuilder strings.Builder
	reportSummaryBuilder.WriteString("**Summary Data**\n")
	reportSummaryBuilder.WriteString("\n")
	reportSummaryBuilder.WriteString(reportSummary)

	reportSummaryBuilder.WriteString(formatClusterConfig(c))

	reportSummaryBuilder.WriteString(formatAWSConfig(c))

	reportSummaryBuilder.WriteString(formatOptionalConfigs(c))

	return reportSummaryBuilder.String()
}

func formatClusterConfig(c *shared.Cluster) string {
	clusterInfo := []struct{ labelKey, value string }{
		{"Product", c.Config.Product},
		{"Product version", c.Config.Version},
		{"Channel", c.Config.Channel},
		{"Install mode", c.Config.InstallMode},
		{"Install method", c.Config.InstallMethod},
		{"Server flags", c.Config.ServerFlags},
		{"Datastore", c.Config.DataStore},
		{"Architecture", c.Config.Arch},
		{"Node OS", c.NodeOS},
	}

	return formatSection("Cluster Configuration", clusterInfo)
}

func formatAWSConfig(c *shared.Cluster) string {
	accessKey := os.Getenv("ACCESS_KEY_LOCAL")
	accessKeyName := filepath.Base(accessKey)

	awsInfo := []struct{ labelKey, value string }{
		{"Access key", accessKeyName},
		{"User", c.Aws.EC2.AwsUser},
		{"Key name", c.Aws.EC2.KeyName},
		{"Region", c.Aws.Region},
		{"Availability zone", c.Aws.AvailabilityZone},
		{"AMI", c.Aws.EC2.Ami},
		{"Instance class", c.Aws.EC2.InstanceClass},
		{"Volume size", c.Aws.EC2.VolumeSize},
		{"VPC ID", c.Aws.VPCID},
		{"Subnets", c.Aws.Subnets},
		{"Security group ID", c.Aws.SgId},
		{"Num servers", strconv.Itoa(c.NumServers)},
		{"Num agents", strconv.Itoa(c.NumAgents)},
	}

	return formatSection("AWS Configuration", awsInfo)
}

func formatOptionalConfigs(c *shared.Cluster) string {
	var sections []string

	// externalDB config.
	if c.Config.DataStore == "external" {
		extInfo := []struct{ labelKey, value string }{
			{"DB endpoint", c.Config.ExternalDbEndpoint},
			{"DB type", c.Config.ExternalDb},
			{"DB version", c.Config.ExternalDbVersion},
			{"DB group name", c.Config.ExternalDbGroupName},
			{"DB node type", c.Config.ExternalDbNodeType},
		}
		sections = append(sections, formatSection("External Database", extInfo))
	}

	// bastion config.
	if c.NumBastion > 0 {
		bastionInfo := []struct{ labelKey, value string }{
			{"Bastion Public IPv4", c.BastionConfig.PublicIPv4Addr},
			{"Bastion Public DNS", c.BastionConfig.PublicDNS},
		}
		sections = append(sections, formatSection("Bastion Host", bastionInfo))
	}

	return strings.Join(sections, "\n\n")
}
