package qase

import (
	"strings"
	"testing"

	"github.com/onsi/ginkgo/v2/types"

	"github.com/rancher/distros-test-framework/internal/provisioning/driver"
)

func testCase(name, status string) TestCase {
	return TestCase{Name: name, Status: status, StackTrace: Failures{Message: "msg " + name}, Elapsed: 1, IsSpec: true}
}

func hook(nodeType types.NodeType, state types.SpecState, msg string) types.SpecReport {
	return types.SpecReport{LeafNodeType: nodeType, LeafNodeText: "", State: state, Failure: types.Failure{Message: msg}}
}

func it(text string, state types.SpecState, msg string) types.SpecReport {
	return types.SpecReport{
		LeafNodeType: types.NodeTypeIt, LeafNodeText: text, State: state, Failure: types.Failure{Message: msg},
	}
}

// Report → converter → result, the path the Jenkins jobs really take.
func TestSpecReportPathKeepsSkipReasonAndIgnoresHooks(t *testing.T) {
	const reason = "skipping k3s-specific test"
	report := &types.Report{SuiteSucceeded: true, SpecReports: types.SpecReports{
		hook(types.NodeTypeBeforeSuite, types.SpecStatePassed, ""),
		it("[k3s/10053] Validates Ingress after Pod Restart", types.SpecStateSkipped, reason),
		hook(types.NodeTypeAfterSuite, types.SpecStatePassed, ""),
	}}
	tcs, ok := specReportToTestCase(report)
	req := parseResults(newCluster(), tcs, ok, "summary", &createResultRequest{})
	if req.status != skipStatus {
		t.Fatalf("green hooks around a skipped It must be %s, got %s", skipStatus, req.status)
	}
	if !strings.Contains(*req.comment.Get(), reason) {
		t.Fatalf("the Skip() reason must survive the conversion:\n%s", *req.comment.Get())
	}
	if strings.Contains(*req.comment.Get(), "Failed sub-tests") {
		t.Fatalf("a skip is not a failure:\n%s", *req.comment.Get())
	}

	report.SpecReports = append(types.SpecReports{it("real", types.SpecStatePassed, "")}, report.SpecReports...)
	tcs, ok = specReportToTestCase(report)
	if got := parseResults(newCluster(), tcs, ok, "summary", &createResultRequest{}).status; got != passStatus {
		t.Fatalf("one passed It plus a skipped one must be %s, got %s", passStatus, got)
	}
}

func TestSpecReportPathHookFailureFails(t *testing.T) {
	report := &types.Report{SuiteSucceeded: false, SpecReports: types.SpecReports{
		hook(types.NodeTypeBeforeSuite, types.SpecStateFailed, "cluster did not come up"),
		it("never ran", types.SpecStateSkipped, ""),
	}}
	tcs, ok := specReportToTestCase(report)
	req := parseResults(newCluster(), tcs, ok, "summary", &createResultRequest{})
	comment := *req.comment.Get()
	if req.status != failStatus || !strings.Contains(comment, "cluster did not come up") {
		t.Fatalf("a failed BeforeSuite must fail the result with its message: %s\n%s", req.status, comment)
	}
	// The hook is named by its type, and the Its it kept from running are not "skipped".
	if !strings.Contains(comment, "Name: BeforeSuite") ||
		!strings.Contains(comment, "Specs not run (BeforeSuite failed):") ||
		strings.Contains(comment, "Skipped sub-tests (not failures)") {
		t.Fatalf("hook name or not-run heading:\n%s", comment)
	}
}

func newCluster() *driver.Cluster {
	c := &driver.Cluster{}
	c.Config.Product = "rke2"
	c.Config.Version = "v1.34.12-rc2+rke2r1"

	return c
}

// rke2_dual_stack: 7 passed + the k3s-only spec skipped must be a passed result, not a failed one.
func TestParseResultsSkippedSpecIsNotAFailure(t *testing.T) {
	cases := []TestCase{
		testCase("a", passStatus), testCase("b", passStatus), testCase("c", passStatus), testCase("d", passStatus),
		testCase("e", passStatus), testCase("f", passStatus), testCase("g", passStatus),
		testCase("[k3s/10053] Validates Ingress after Pod Restart", skipStatus),
	}
	req := parseResults(newCluster(), cases, true, "summary", &createResultRequest{})
	if req.status != passStatus {
		t.Fatalf("expected %s, got %s", passStatus, req.status)
	}
	comment := req.comment.Get()
	if strings.Contains(*comment, "Failed sub-tests") {
		t.Fatalf("skipped spec must not be listed as failed:\n%s", *comment)
	}
	if !strings.Contains(*comment, "Skipped sub-tests (not failures)") || !strings.Contains(*comment, "[k3s/10053]") {
		t.Fatalf("skipped spec must be listed as skipped:\n%s", *comment)
	}
}

func TestParseResultsStatuses(t *testing.T) {
	cases := []struct {
		name           string
		specs          []TestCase
		suiteSucceeded bool
		want           string
		wantInComment  string
	}{
		{name: "all passed", specs: []TestCase{testCase("a", passStatus)}, suiteSucceeded: true, want: passStatus},
		{
			name: "one failed", specs: []TestCase{testCase("a", passStatus), testCase("b", failStatus)},
			suiteSucceeded: true,
			want:           failStatus, wantInComment: "Failed sub-tests",
		},
		{
			name: "panicked counts as failure", specs: []TestCase{testCase("a", types.SpecStatePanicked.String())},
			suiteSucceeded: true, want: failStatus,
		},
		{
			name: "timedout counts as failure", specs: []TestCase{testCase("a", types.SpecStateTimedout.String())},
			suiteSucceeded: true, want: failStatus,
		},
		{
			name: "everything skipped is skipped", specs: []TestCase{testCase("a", skipStatus), testCase("b", "pending")},
			suiteSucceeded: true, want: skipStatus,
		},
		{
			name: "suite failure with green specs (BeforeSuite/AfterSuite)", specs: []TestCase{testCase("a", passStatus)},
			suiteSucceeded: false, want: failStatus, wantInComment: "Suite failed outside its specs",
		},
		{name: "no specs at all and suite ok", specs: nil, suiteSucceeded: true, want: passStatus},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := parseResults(newCluster(), c.specs, c.suiteSucceeded, "summary", &createResultRequest{})
			if req.status != c.want {
				t.Fatalf("expected %s, got %s", c.want, req.status)
			}
			if c.wantInComment != "" && !strings.Contains(*req.comment.Get(), c.wantInComment) {
				t.Fatalf("comment must contain %q:\n%s", c.wantInComment, *req.comment.Get())
			}
		})
	}
}

func TestParseBulkResultsSkipped(t *testing.T) {
	withCase := func(id int64, t TestCase) TestCase {
		t.CaseID = id

		return t
	}
	reqs := parseBulkResults([]TestCase{
		withCase(1, testCase("only skipped", skipStatus)),
		withCase(2, testCase("mixed pass", passStatus)), withCase(2, testCase("mixed skip", skipStatus)),
		withCase(3, testCase("ok", passStatus)), withCase(3, testCase("boom", failStatus)),
	}, 7)

	got := map[int64]createResultRequest{}
	for _, r := range reqs {
		got[*r.caseID] = r
	}
	if got[1].status != skipStatus || strings.Contains(*got[1].comment.Get(), "Passed sub-test") {
		t.Fatalf("all-skipped case must be skipped without 'Passed sub-test': %s %s", got[1].status, *got[1].comment.Get())
	}
	if got[2].status != passStatus || !strings.Contains(*got[2].comment.Get(), "Skipped sub-test: mixed skip") {
		t.Fatalf("mixed case must pass and list the skip: %s %s", got[2].status, *got[2].comment.Get())
	}
	if got[3].status != failStatus || !strings.Contains(*got[3].comment.Get(), "FAILED Sub-test:** boom") {
		t.Fatalf("failing case must fail: %s %s", got[3].status, *got[3].comment.Get())
	}
}

// Skip()s stay "skipped" when the suite fails for another reason: an It failed, or AfterSuite
// failed after the specs ran. Only a failed BeforeSuite means the specs did not run.
func TestSkipsStaySkipsWhenSuiteFailsElsewhere(t *testing.T) {
	for name, reports := range map[string]types.SpecReports{
		"an It failed": {
			hook(types.NodeTypeBeforeSuite, types.SpecStatePassed, ""),
			it("broken", types.SpecStateFailed, "boom"),
			it("k3s only", types.SpecStateSkipped, "skipping k3s-specific test"),
		},
		"AfterSuite failed": {
			hook(types.NodeTypeBeforeSuite, types.SpecStatePassed, ""),
			it("fine", types.SpecStatePassed, ""),
			it("k3s only", types.SpecStateSkipped, "skipping k3s-specific test"),
			hook(types.NodeTypeAfterSuite, types.SpecStateFailed, "cleanup failed"),
		},
	} {
		tcs, _ := specReportToTestCase(&types.Report{SuiteSucceeded: false, SpecReports: reports})
		comment := *parseResults(newCluster(), tcs, false, "summary", &createResultRequest{}).comment.Get()
		if !strings.Contains(comment, "Skipped sub-tests (not failures)") || strings.Contains(comment, "Specs not run") {
			t.Fatalf("%s:\n%s", name, comment)
		}
	}
}

// A panicked spec from the parsed log reaches Qase with its error log as the stack trace.
func TestSuiteDetailsKeepPanickedStackTrace(t *testing.T) {
	failure := &FailureDetails{}
	tcs := testSuiteDetailsToTestCase([]testOverview{{testCases: []testDetails{
		{testCaseName: "crashed", status: "panicked", errorLog: "panic: nil map", failureDetails: failure},
	}}})
	if len(tcs) != 1 || tcs[0].StackTrace.Message != "panic: nil map" || tcs[0].FailureDetails != failure {
		t.Fatalf("test cases %+v", tcs)
	}
}
