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
