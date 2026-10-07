package releasebot

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Jobs that use {{QASE_RUN_ID}} need the Qase client; without a token the CLI must stop before
// dispatching any workflow, so nothing is half-started.
func TestExecuteChecksQaseBeforeDispatch(t *testing.T) {
	t.Setenv("QASE_AUTOMATION_TOKEN", "")

	dispatches := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		dispatches++
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	gh := &gitHub{BaseURL: srv.URL, Token: "tok", HTTP: srv.Client()}

	plan := &Plan{
		RequestID: "rb-1",
		Workflows: []workflowDispatch{{Repo: "o/r", Workflow: "wf.yaml", Ref: "main"}},
		Jobs: []JenkinsJob{{
			Name: "smoke", Product: "rke2", Version: "v1.37.1-rc1+rke2r1", Controller: "mower", Path: "p",
			Params: map[string]string{"QASE_RUN_ID": "{{QASE_RUN_ID}}"},
		}},
	}
	p := &prepared{Plan: plan, Matrix: &Matrix{}, GitHub: gh}

	err := New(&Config{Poll: time.Millisecond, QaseTimeout: time.Second}).Execute(context.Background(), p, nil)
	if err == nil || !strings.Contains(err.Error(), "QASE_AUTOMATION_TOKEN") {
		t.Fatalf("expected a Qase token error, got %v", err)
	}
	if dispatches != 0 {
		t.Fatalf("%d workflow dispatches happened before the Qase check", dispatches)
	}

	// Without jobs that need a run id (-skip-jobs), no token is required and workflows dispatch.
	skipJobs := New(&Config{Poll: time.Millisecond, QaseTimeout: time.Second, SkipJobs: true})
	if err = skipJobs.Execute(context.Background(), p, nil); err != nil || dispatches != 1 {
		t.Fatalf("skip-jobs: err=%v dispatches=%d", err, dispatches)
	}
}

// fakeDispatches answers each workflow dispatch with the next status, calling onDispatch first.
func fakeDispatches(t *testing.T, statuses []int, onDispatch func()) (gh *gitHub, dispatches *int) {
	t.Helper()
	n := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		status := statuses[n]
		n++
		onDispatch()
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)

	return &gitHub{BaseURL: srv.URL, Token: "tok", HTTP: srv.Client()}, &n
}

// After the first dispatch may have started a workflow, any failure is a partial run; only a
// first dispatch rejected (4xx) or never sent (no token), or a failure before it, proves nothing started.
func TestExecuteMarksPartialRuns(t *testing.T) {
	wfs := []workflowDispatch{
		{Repo: "o/r", Workflow: "a.yaml", Ref: "qa-infra-RC-1"},
		{Repo: "o/r", Workflow: "b.yaml", Ref: "qa-infra-RC-1"},
	}
	jobs := []JenkinsJob{{Name: "smoke", Controller: "mower", Path: "p"}}
	matrix := &Matrix{Controller: map[string]Limits{"mower": {URL: "http://jenkins"}}}

	cases := []struct {
		name       string
		statuses   []int // per dispatch
		cancel     bool  // cancel ctx once the first dispatch is accepted
		jobs       bool
		noToken    bool
		partial    bool
		dispatches int
	}{
		{name: "first dispatch rejected", statuses: []int{422}, partial: false, dispatches: 1},
		{name: "first dispatch outcome unknown", statuses: []int{502}, partial: true, dispatches: 1},
		{name: "second dispatch rejected", statuses: []int{204, 422}, partial: true, dispatches: 2},
		{name: "canceled after first dispatch", statuses: []int{204, 204}, cancel: true, partial: true, dispatches: 1},
		{name: "jenkins credentials missing", jobs: true, partial: false, dispatches: 0},
		{name: "github token missing", noToken: true, partial: false, dispatches: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			onDispatch := func() {}
			if tc.cancel {
				onDispatch = cancel
			}
			gh, dispatches := fakeDispatches(t, tc.statuses, onDispatch)
			if tc.noToken {
				gh.Token = ""
			}

			plan := &Plan{RequestID: "rb-1", Workflows: wfs}
			if tc.jobs {
				plan.Jobs = jobs
			}
			app := New(&Config{Poll: time.Millisecond})
			err := app.Execute(ctx, &prepared{Plan: plan, Matrix: matrix, GitHub: gh}, nil)
			if err == nil {
				t.Fatal("expected an error")
			}
			if got := errors.Is(err, errPartialRun); got != tc.partial {
				t.Fatalf("partial = %v, want %v (err: %v)", got, tc.partial, err)
			}
			if *dispatches != tc.dispatches {
				t.Fatalf("dispatches = %d, want %d", *dispatches, tc.dispatches)
			}
		})
	}
}

// A plan with a required job that cannot run is refused before anything is dispatched.
func TestExecuteRefusesUnrunnablePlan(t *testing.T) {
	dispatches := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		dispatches++
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	gh := &gitHub{BaseURL: srv.URL, Token: "tok", HTTP: srv.Client()}
	plan := &Plan{
		RequestID: "rb-1",
		Workflows: []workflowDispatch{{Repo: "o/r", Workflow: "wf.yaml", Ref: "qa-infra-RC-1"}},
		Jobs:      []JenkinsJob{{Name: "arm", Controller: "mower", Path: "p/arm"}},
		Blocked:   []SkippedJob{{Path: "p/tar", Version: "v1", Reason: "job has no parameter INSTALL_VERSION"}},
	}

	p := &prepared{Plan: plan, Matrix: &Matrix{}, GitHub: gh}
	err := New(&Config{Poll: time.Millisecond}).Execute(context.Background(), p, nil)
	if err == nil || !strings.Contains(err.Error(), "not running") || dispatches != 0 {
		t.Fatalf("err=%v dispatches=%d", err, dispatches)
	}
}

// The Slack summary has no DRY-RUN/EXECUTE label: the message around it says whether it started.
func TestPlanSummaryHasNoMode(t *testing.T) {
	sum := planSummary(&Plan{RequestID: "rb-1"})
	labeled := strings.Contains(sum, "DRY-RUN") || strings.Contains(sum, "EXECUTE")
	if labeled || !strings.Contains(sum, "Release plan, request rb-1") {
		t.Fatalf("summary: %q", sum)
	}
}

// flakyBuilder fails the "smoke" path and passes every other job; builds finish on the first poll.
type flakyBuilder struct{}

func (flakyBuilder) Trigger(_ context.Context, j *JenkinsJob) (string, error) {
	return "q/" + j.Path, nil
}

func (flakyBuilder) BuildFromQueue(_ context.Context, q string) (string, error) { return "b/" + q, nil }

func (flakyBuilder) Finished(_ context.Context, b string) (done bool, result string, err error) {
	if b == "b/q/smoke" {
		return true, "FAILURE", nil
	}

	return true, "SUCCESS", nil
}

// A job a person skips counts as passed in the run's result, so the run does not end as failed
// (which would block new plans).
func TestRunJobsSkippedIsNotAFailure(t *testing.T) {
	commands := make(chan Command, 1)
	h := &hooks{Notify: func(string, string, ...any) {}, Triage: askTriager, Commands: commands}
	h.Help = func(string) {
		reply := make(chan string, 1)
		commands <- Command{Action: CommandSkip, Job: "smoke", By: "U1", Reply: reply}
		<-reply
	}
	matrix := &Matrix{Controller: map[string]Limits{"mower": {URL: "u", MaxConcurrent: 2}}}
	jobs := []JenkinsJob{
		{Name: "smoke", Product: "rke2", Version: "v1", Controller: "mower", Path: "smoke", Phase: 1},
		{
			Name: "conf", Product: "rke2", Version: "v1", Controller: "mower", Path: "conf", Phase: 2,
			DependsOn: []string{"smoke"},
		},
	}
	builders := map[string]Builder{"mower": flakyBuilder{}}

	app := New(&Config{Poll: time.Millisecond})
	if err := app.runJobs(context.Background(), h, &Progress{}, matrix, builders, jobs, nil); err != nil {
		t.Fatalf("runJobs = %v, want nil after a skip", err)
	}
}

// A dispatch that never left the bot says so: nothing can have started, so the run is not partial.
func TestDispatchNotSent(t *testing.T) {
	w := workflowDispatch{Repo: "o/r", Workflow: "wf.yaml", Ref: "main"}
	if err := (&gitHub{Token: ""}).Dispatch(context.Background(), w); !errors.Is(err, errDispatchNotSent) {
		t.Fatalf("no token: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := newGitHub("tok").Dispatch(ctx, w); !errors.Is(err, errDispatchNotSent) {
		t.Fatalf("canceled: %v", err)
	}
	srv := httptest.NewServer(http.NotFoundHandler())
	addr := srv.URL
	srv.Close()
	gh := &gitHub{BaseURL: addr, Token: "tok", HTTP: http.DefaultClient}
	if err := gh.Dispatch(context.Background(), w); !errors.Is(err, errDispatchNotSent) {
		t.Fatalf("connection refused: %v", err)
	}
}
