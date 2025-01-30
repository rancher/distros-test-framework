package qase

import (
	"context"
	"fmt"
	"os"
	"strings"

	qaseclient "github.com/qase-tms/qase-go/qase-api-client"

	"github.com/rancher/distros-test-framework/shared"
)

func (c Client) createRun(pd *processedTestdata, titleName, product string) (*int64, error) {
	description, planID := buildRun(pd)

	// create the test run title.
	runTitle := titleName + " " + product + " test run - " + pd.testDate
	createRunReq := c.QaseAPI.RunsAPI.CreateRun(c.Ctx, projectID).RunCreate(qaseclient.RunCreate{
		Title:           runTitle,
		Description:     newString(description),
		IncludeAllCases: newBool(false),
		PlanId:          newInt64(planID),
		IsAutotest:      newBool(true),
	})

	res, httpRes, err := createRunReq.Execute()
	if err != nil {
		if httpRes != nil {
			return nil, fmt.Errorf("failed to create test run: %w, response: %v", err, httpRes.Body)
		}
		return nil, fmt.Errorf("failed to create test run: %w", err)
	}

	return res.Result.Id, nil
}

func buildRun(pd *processedTestdata) (desc string, planID int64) {
	var suiteSummaries []string
	for _, suite := range pd.testSuiteSummary {
		summary := fmt.Sprintf(
			"Suite: %s\n Elapsed time: %.2f min\n  Status: %s\n  Failed: %d, Passed: %d, Skipped: %d\n",
			suite.testSuiteName,
			suite.elapsedTime,
			suite.status,
			suite.failedTests,
			suite.passedTests,
			suite.skippedTests,
		)
		// If the suite has failures, add details of failed test cases.
		if suite.failedTests > 0 {
			var failedTestNames string
			for _, overview := range pd.testSummary {
				if overview.testSuiteName == suite.testSuiteName {
					for _, td := range overview.testCases {
						if strings.EqualFold(td.status, failStatus) {
							failedTestNames += fmt.Sprintf("    - %s\n", td.testCaseName)
						}
					}

					break
				}
			}
			if failedTestNames != "" {
				summary += "  Failed Test Cases:\n" + failedTestNames
			}
		}
		suiteSummaries = append(suiteSummaries, summary)
	}
	// Join all suite summaries.
	testSuiteSummary := strings.Join(suiteSummaries, "\n")
	// Build test run description.
	description := fmt.Sprintf(
		"Total Test time: %s\nTest Date: %s\nFAILED: %d\nPASSED: %d\nSKIPPED: %d\n\n"+
			"Test Suite Summary:\n%s\nGH Actions: %s\n",
		pd.totalTestTime,
		pd.testDate,
		pd.failedTests,
		pd.passedTests,
		pd.skippedTests,
		testSuiteSummary,
		os.Getenv("COMMENT_LINK"),
	)

	// Update the planID according to the product that has uptated the the planID field on testSuiteSummary/testSuiteDetails.
	var id int64
	for _, suite := range pd.testSuiteSummary {
		id = suite.planID
		break
	}

	return description, id
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

func (c Client) createBulkTestResult(reqs []createResultRequest) error {
	if len(reqs) == 0 {
		return nil
	}

	results := make([]qaseclient.ResultCreate, len(reqs))
	for i, r := range reqs {
		results[i] = qaseclient.ResultCreate{
			CaseId:  r.caseID,
			Status:  r.status,
			Comment: r.comment,
			Time:    r.time,
		}
	}

	bulkRequest := qaseclient.ResultcreateBulk{
		Results: results,
	}

	create := c.QaseAPI.ResultsAPI.CreateResultBulk(c.Ctx, projectID, reqs[0].runID)
	res, httpRes, err := create.ResultcreateBulk(bulkRequest).Execute()
	if err != nil {
		return fmt.Errorf("failed to create test result: %w, response: %v", err, httpRes)
	}

	shared.LogLevel("debug", "Test result created: %v\n", &res)

	return nil
}
