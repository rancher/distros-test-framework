package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/rancher/distros-test-framework/internal/pkg/releasebot"
)

// A run that ended but could not read its consoles keeps its final progress: a new run refuses (it
// would drop those MDs), and -resume writes them once jenkins answers, then forgets the run.
func TestBumpReportIsWrittenAgainWithResume(t *testing.T) {
	var down atomic.Bool
	down.Store(true)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case down.Load():
			http.Error(w, "down", http.StatusInternalServerError)
		case strings.HasSuffix(r.URL.Path, "/consoleText"):
			_, _ = w.Write([]byte("rke2 version v1.37.2-rc1+rke2r1 (1234abcd)\n" +
				"Command to Execute: sudo grep -h chart-url /x.yaml\nExpected Value: rke2-x-1.0.tgz\n" +
				"Matched with result:\n  chart-url: rke2-x-1.0.tgz\n\n" +
				"Command to Execute: sudo grep -h chart-url /c.yaml\nExpected Value: rke2-canal-2.0.tgz\n" +
				"Matched with result:\n  chart-url: rke2-canal-2.0.tgz\n\n"))
		default:
			_, _ = w.Write([]byte(`{"actions":[{"parameters":[{"name":"NODE_OS","value":"sles15"}]}]}`))
		}
	}))
	defer srv.Close()

	o, cfg, run, runFile := bumpReportFixture(t, srv.URL)

	app := newApp(o)
	terminal, _, terminalErr := bumpTerminal(o, app, cfg)
	if terminalErr != nil {
		t.Fatal(terminalErr)
	}
	if err := reportBumps(t.Context(), o, app, terminal, run, runFile, nil); err == nil ||
		!strings.Contains(err.Error(), "-resume") {
		t.Fatalf("a failed report: %v", err)
	}
	if _, err := os.Stat(runFile); err != nil {
		t.Fatalf("the ended run was not kept: %v", err)
	}
	if err := executeBumps(t.Context(), o, cfg, "main"); err == nil || !strings.Contains(err.Error(), "without its MDs") {
		t.Fatalf("a new run next to unwritten MDs: %v", err)
	}

	down.Store(false)
	o.resume = true
	if err := executeBumps(t.Context(), o, cfg, "main"); err != nil {
		t.Fatal(err)
	}
	// Both jobs of the branch (the canal install and the other) report their own issue.
	md, readErr := os.ReadFile(filepath.Join(o.bumpsDir, "rke2-bumps-release-2026-11.md"))
	if readErr != nil || !strings.Contains(string(md), "rke2-x-1.0.tgz") ||
		!strings.Contains(string(md), "rke2-canal-2.0.tgz") {
		t.Fatalf("MD %q, %v", md, readErr)
	}
	if _, err := os.Stat(runFile); !os.IsNotExist(err) {
		t.Fatalf("the reported run was kept: %v", err)
	}
}

// bumpReportFixture is a bump run that ended with one passed RKE2 1.37 job on the jenkins at url.
func bumpReportFixture(
	t *testing.T, url string,
) (o *options, cfg *releasebot.BumpConfig, run *releasebot.BumpRun, runFile string) {
	t.Helper()
	dir := t.TempDir()
	matrix := filepath.Join(dir, "matrix.yaml")
	raw := []byte("controllers:\n  mower:\n    url: " + url + "\n    maxConcurrent: 6\n")
	if err := os.WriteFile(matrix, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("JENKINS_MOWER_AUTH", "u:t")
	o = &options{matrixPath: matrix, stateFile: filepath.Join(dir, "state.json"), bumpsDir: filepath.Join(dir, "md"),
		skipWorkflows: true}
	cfg = &releasebot.BumpConfig{Controller: "mower", PrefixBase: "fmor", Jobs: map[string]string{"rke2": "r/v"}}

	target := &releasebot.BumpTarget{Product: "rke2", Branch: "release-1.37", Version: "v1.37.2-rc1+rke2r1",
		Checks: []releasebot.BumpCheck{{What: "rke2-x chart 1.0", Cmd: "sudo grep -h chart-url /x.yaml",
			Expected: "rke2-x-1.0.tgz", Issue: 2}}}
	canal := &releasebot.BumpTarget{Product: "rke2", Branch: "release-1.37", Version: "v1.37.2-rc1+rke2r1",
		JobPath: "r/c", JobName: "canal", Checks: []releasebot.BumpCheck{{What: "rke2-canal chart 2.0",
			Cmd: "sudo grep -h chart-url /c.yaml", Expected: "rke2-canal-2.0.tgz", Issue: 4}}}
	backport := &releasebot.BumpIssue{Product: "rke2", Repo: "rancher/rke2", Number: 2, Branch: "release-1.37"}
	canalBackport := &releasebot.BumpIssue{Product: "rke2", Repo: "rancher/rke2", Number: 4, Branch: "release-1.37"}
	run = &releasebot.BumpRun{
		Plan: &releasebot.BumpPlan{Month: "2026-11", Targets: []*releasebot.BumpTarget{target, canal},
			Groups: []*releasebot.BumpGroup{
				{Product: "rke2", Main: releasebot.BumpIssue{Repo: "rancher/rke2", Number: 1, Title: "Bump x"},
					Backports: []*releasebot.BumpIssue{backport}},
				{Product: "rke2", Main: releasebot.BumpIssue{Repo: "rancher/rke2", Number: 3, Title: "Update Canal"},
					Backports: []*releasebot.BumpIssue{canalBackport}},
			}},
		Final: &releasebot.Progress{Jobs: []releasebot.JobRecord{
			{Job: releasebot.JenkinsJob{Name: "rke2-bumps-137"}, State: "finished", Result: "SUCCESS",
				BuildURL: url + "/job/r/job/v/7/"},
			{Job: releasebot.JenkinsJob{Name: "rke2-bumps-137-canal"}, State: "finished", Result: "SUCCESS",
				BuildURL: url + "/job/r/job/c/8/"},
		}},
	}
	runFile = filepath.Join(dir, "bumps-run-plan.json")
	if err := releasebot.SaveBumpRun(runFile, run); err != nil {
		t.Fatal(err)
	}

	return o, cfg, run, runFile
}
