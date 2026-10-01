package qase

import (
	"testing"
	"time"
)

func TestRKE2AdditionalSuitesProduceQaseResults(t *testing.T) {
	tests := []struct {
		suite  string
		caseID int64
		status string
	}{
		{"Test_E2EClusterLoadBalancer", 369, passStatus},
		{"Test_E2ENightlyCNI", 370, failStatus},
		{"Test_E2EKubeVIP", 371, passStatus},
	}

	for _, tt := range tests {
		t.Run(tt.suite, func(t *testing.T) {
			start := time.Date(2026, time.September, 15, 3, 0, 0, 0, time.UTC)
			data := []goTestData{{Time: start}, {Time: start.Add(time.Minute)}}
			allTests := []testDetails{{
				testSuiteName: tt.suite,
				testCaseName:  "Validates cluster behavior",
				status:        tt.status,
			}}
			allSuites := []testSuiteDetails{{testSuiteName: tt.suite}}
			pd := processData(data, allTests, allSuites, "rke2", "amd64")
			results := parseBulkResults(testSuiteDetailsToTestCase(pd.testSummary), 123)

			if len(results) != 1 {
				t.Fatalf("suite %s produced %d Qase results, want 1", tt.suite, len(results))
			}
			result := results[0]
			if result.caseID == nil {
				t.Fatal("Qase result has no case ID")
			}
			if *result.caseID != tt.caseID {
				t.Errorf("case ID = %d, want %d", *result.caseID, tt.caseID)
			}
			if result.status != tt.status {
				t.Errorf("status = %q, want %q", result.status, tt.status)
			}
			if result.runID != 123 {
				t.Errorf("run ID = %d, want 123", result.runID)
			}
		})
	}
}

// A panicked or timed-out spec in the embedded results is kept and counted as failed, like the
// bulk results do, instead of being dropped as an unknown state.
func TestUpdateTestDetailsCountsEveryFailureState(t *testing.T) {
	for _, state := range []string{"failed", "panicked", "timedout"} {
		remainingData = ""
		var all []testDetails
		var s status
		out := `{"state":"` + state + `","name":"spec","type":"rke2 test","time":1}` + "\n"
		row := &goTestData{Test: "Test_E2E", Output: out}
		if err := updateTestDetails(&all, row, &s, "", map[string]bool{}); err != nil {
			t.Fatal(err)
		}
		if s.failed != 1 || len(all) != 1 || all[0].status != state {
			t.Fatalf("%s: failed %d, details %+v", state, s.failed, all)
		}
	}
	remainingData = ""
}

// Failure details of a panicked spec reach the reporters like those of a failed one.
func TestFailedTestDetailsIncludePanicked(t *testing.T) {
	failure := &FailureDetails{}
	pd := &processedTestdata{testSummary: []testOverview{{testCases: []testDetails{
		{status: "panicked", failureDetails: failure}, {status: passStatus, failureDetails: &FailureDetails{}},
	}}}}
	if got := pd.GetFailedTestDetails(); len(got) != 1 || got[0] != failure {
		t.Fatalf("failed details %v", got)
	}
}
