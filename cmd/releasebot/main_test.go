package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rancher/distros-test-framework/internal/pkg/releasebot"
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
	gh := &releasebot.GitHub{BaseURL: srv.URL, Token: "tok", HTTP: srv.Client()}

	plan := &releasebot.Plan{
		RequestID: "rb-1",
		Workflows: []releasebot.WorkflowDispatch{{Repo: "o/r", Workflow: "wf.yaml", Ref: "main"}},
		Jobs: []releasebot.JenkinsJob{{
			Name: "smoke", Product: "rke2", Version: "v1.37.1-rc1+rke2r1", Controller: "mower", Path: "p",
			Params: map[string]string{"QASE_RUN_ID": "{{QASE_RUN_ID}}"},
		}},
	}
	o := &options{poll: time.Millisecond, qaseTimeout: time.Second}

	err := execute(context.Background(), o, gh, &releasebot.Matrix{}, plan)
	if err == nil || !strings.Contains(err.Error(), "QASE_AUTOMATION_TOKEN") {
		t.Fatalf("expected a Qase token error, got %v", err)
	}
	if dispatches != 0 {
		t.Fatalf("%d workflow dispatches happened before the Qase check", dispatches)
	}

	// Without jobs that need a run id (-skip-jobs), no token is required and workflows dispatch.
	o.skipJobs = true
	if err = execute(context.Background(), o, gh, &releasebot.Matrix{}, plan); err != nil || dispatches != 1 {
		t.Fatalf("skip-jobs: err=%v dispatches=%d", err, dispatches)
	}
}
