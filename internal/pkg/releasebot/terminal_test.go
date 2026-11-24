package releasebot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeJenkins serves buildWithParameters, queue items and build status over HTTP; a held job keeps
// building, and results set a job's final result (default SUCCESS).
type fakeJenkins struct {
	srv *httptest.Server

	mu       sync.Mutex
	triggers map[string]int
	queue    []string // queue item n is queue[n-1]
	held     map[string]bool
	results  map[string]string
}

func newFakeJenkins(t *testing.T) *fakeJenkins {
	t.Helper()
	f := &fakeJenkins{triggers: map[string]int{}, held: map[string]bool{}, results: map[string]string{}}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)

	return f
}

func (f *fakeJenkins) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	switch {
	case len(parts) == 3 && parts[0] == "job" && parts[2] == "buildWithParameters":
		f.triggers[parts[1]]++
		f.queue = append(f.queue, parts[1])
		w.Header().Set("Location", fmt.Sprintf("%s/queue/item/%d/", f.srv.URL, len(f.queue)))
		w.WriteHeader(http.StatusCreated)
	case len(parts) == 5 && parts[0] == "queue" && parts[3] == "api":
		n, _ := strconv.Atoi(parts[2])
		_, _ = fmt.Fprintf(w, `{"executable":{"url":%q}}`, fmt.Sprintf("%s/job/%s/%d/", f.srv.URL, f.queue[n-1], n))
	case len(parts) == 5 && parts[0] == "job" && parts[3] == "api":
		result := f.results[parts[1]]
		if result == "" {
			result = resultSuccess
		}
		if f.held[parts[1]] {
			_, _ = io.WriteString(w, `{"building":true,"result":null}`)
			return
		}
		_, _ = fmt.Fprintf(w, `{"building":false,"result":%q}`, result)
	default:
		http.NotFound(w, r) // crumbIssuer included: no CSRF protection here
	}
}

func (f *fakeJenkins) count(job string) int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.triggers[job]
}

func (f *fakeJenkins) set(job string, held bool, result string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.held[job], f.results[job] = held, result
}

// terminalPlan: smoke, then conf once smoke passed, on the fake controller (also in the matrix file a
// resume reads again).
func terminalPlan(t *testing.T, f *fakeJenkins) (*App, *prepared) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "matrix.yaml")
	body := "controllers:\n  mower: {url: " + f.srv.URL + ", maxConcurrent: 2}\njobs:\n" +
		"  - {name: smoke, product: rke2, controller: mower, path: smoke, priority: 1, code: s}\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	app := New(&Config{
		MatrixPath:  path,
		Poll:        time.Millisecond,
		JenkinsAuth: func(string) (string, string, bool) { return "u", "t", true },
	})
	plan := &Plan{RequestID: "rb-cli", Jobs: []JenkinsJob{
		{Name: "smoke", Product: "rke2", Version: "v1", Controller: "mower", Path: "smoke", Phase: 1},
		{
			Name: "conf", Product: "rke2", Version: "v1", Controller: "mower", Path: "conf", Phase: 2,
			DependsOn: []string{"smoke"},
		},
	}}
	matrix := &Matrix{Controller: map[string]Limits{"mower": {URL: f.srv.URL, MaxConcurrent: 2}}}

	return app, &prepared{Plan: plan, Matrix: matrix, GitHub: &gitHub{}}
}

// syncBuffer is an io.Writer safe for the run's goroutines.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.b.String()
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); !cond(); time.Sleep(time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
	}
}

// A CLI run cut mid-way (Ctrl-C, a crash, a reboot) stays saved; started again with -resume it follows
// the running build instead of triggering it again, runs the rest and removes its state at the end.
func TestTerminalRunResumesAfterInterrupt(t *testing.T) {
	f := newFakeJenkins(t)
	f.set("smoke", true, "")
	app, p := terminalPlan(t, f)
	state := filepath.Join(t.TempDir(), "cli-run.json")

	first := &TerminalRun{StatePath: state, Out: &syncBuffer{}}
	ctx, interrupt := context.WithCancel(testContext(t))
	done := make(chan error, 1)
	go func() { done <- first.Execute(ctx, app, p, "v1 smoke", nil) }()
	waitFor(t, "smoke to run with its state saved", func() bool {
		saved, _ := first.Saved()
		return saved != nil && len(saved.Progress.Jobs) == 2 && saved.Progress.Jobs[0].State == jobRunning
	})
	interrupt()
	_ = receive(t, done) // the interrupt's error; what counts is what stays saved

	out := first.Out.(*syncBuffer).String()
	saved, err := first.Saved()
	if err != nil || saved == nil || saved.Request != "v1 smoke" || !strings.Contains(out, "Interrupted") {
		t.Fatalf("interrupted run not kept: saved %+v err %v\n%s", saved, err, out)
	}

	f.set("smoke", false, "")
	second := &TerminalRun{StatePath: state, Out: &syncBuffer{}}
	app2, _ := terminalPlan(t, f)
	if resumeErr := second.Resume(testContext(t), app2, saved, nil); resumeErr != nil {
		t.Fatalf("resume: %v\n%s", resumeErr, second.Out.(*syncBuffer).String())
	}
	if f.count("smoke") != 1 || f.count("conf") != 1 {
		t.Fatalf("triggers smoke %d conf %d, want 1 each", f.count("smoke"), f.count("conf"))
	}
	if _, statErr := os.Stat(state); !os.IsNotExist(statErr) {
		t.Fatalf("the finished run's state is still there: %v", statErr)
	}
	if !strings.Contains(second.Out.(*syncBuffer).String(), "Release plan finished.") {
		t.Fatalf("output:\n%s", second.Out.(*syncBuffer).String())
	}
}

// A failed job waits for the person at the terminal: status answers, skip lets the next phase start.
func TestTerminalRunTakesCommands(t *testing.T) {
	f := newFakeJenkins(t)
	f.set("smoke", false, "FAILURE")
	app, p := terminalPlan(t, f)
	in, typed := io.Pipe()
	defer typed.Close()
	out := &syncBuffer{}
	term := &TerminalRun{StatePath: filepath.Join(t.TempDir(), "cli-run.json"), In: in, Out: out, User: "captain"}

	done := make(chan error, 1)
	go func() { done <- term.Execute(testContext(t), app, p, "v1", nil) }()
	waitFor(t, "the help message", func() bool { return strings.Contains(out.String(), "NEEDS HELP: Needs help: smoke v1") })

	_, _ = io.WriteString(typed, "status\nskip smoke v1\n")
	if err := receive(t, done); err != nil {
		t.Fatalf("run: %v\n%s", err, out.String())
	}

	text := out.String()
	if !strings.Contains(text, "waiting") || !strings.Contains(text, "captain") || f.count("conf") != 1 {
		t.Fatalf("status, skip or the next phase missing (conf triggered %d):\n%s", f.count("conf"), text)
	}
}

// The saved run is the file's own format; another version is refused, not misread.
func TestTerminalRunRefusesOtherFormats(t *testing.T) {
	state := filepath.Join(t.TempDir(), "cli-run.json")
	raw, _ := json.Marshal(SavedTerminalRun{Version: stateVersion + 1})
	if err := os.WriteFile(state, raw, 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := (&TerminalRun{StatePath: state}).Saved(); err == nil || !strings.Contains(err.Error(), "format") {
		t.Fatalf("a newer format was read: %v", err)
	}
}

// A stop typed before the scheduler knows of it (workflows and Qase come first) survives the run's next
// save and an interrupt, so -resume never starts the jobs of a stopped run.
func TestTerminalRunKeepsATypedStop(t *testing.T) {
	in, typed := io.Pipe()
	defer typed.Close()
	term := &TerminalRun{StatePath: filepath.Join(t.TempDir(), "cli-run.json"), In: in, Out: &syncBuffer{}}
	ctx, interrupt := context.WithCancel(testContext(t))
	saved := &SavedTerminalRun{Version: stateVersion, Request: "v1"}

	done := make(chan error, 1)
	go func() {
		done <- term.drive(ctx, saved, func(rc *RunControl) error {
			<-rc.Stop
			if err := rc.Save(Progress{RequestID: "rb-1", Stage: StageDispatched}); err != nil {
				return err
			}
			interrupt()

			return context.Canceled
		})
	}()
	_, _ = io.WriteString(typed, "stop\n")
	_ = receive(t, done)

	got, err := term.Saved()
	if err != nil || got == nil || !got.Progress.Stopped || got.Progress.Stage != StageDispatched {
		t.Fatalf("the typed stop was lost: %+v, %v", got, err)
	}
}

// One long triage (a full one, a person's `triage`) holds the slot; a quick one behind it waits only so
// long, then asks the person, as the broker does after -triage-wait.
func TestLocalTriageWaitsForTheSlotOnlySoLong(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	defer close(release)
	lt := &LocalTriage{Wait: 20 * time.Millisecond, Triage: func(context.Context, *TriageRequest) (*Verdict, error) {
		close(started)
		<-release

		return &Verdict{Action: "ask", Bucket: "INFRA", Confidence: 50, Summary: "s"}, nil
	}}
	go lt.Ask(testContext(t), quickRequest())
	receive(t, started)

	answered := make(chan Decision, 1)
	go func() {
		d, ok := lt.Ask(testContext(t), quickRequest())
		if ok {
			t.Error("a triage that never ran reported ok")
		}
		answered <- d
	}()
	if d := receive(t, answered); !strings.Contains(d.Summary, "another triage held it for 20ms") {
		t.Fatalf("decision %+v", d)
	}
}

// Workflows that may have started without being followed keep the run saved, so a new execution cannot
// dispatch them again; a run whose jobs all ended (one failed) is over and its state goes.
func TestTerminalRunKeepsUncertainOutcomes(t *testing.T) {
	wfs := []workflowDispatch{{Repo: "o/r", Workflow: "a.yaml", Ref: "main"}, {Repo: "o/r", Workflow: "b.yaml", Ref: "main"}}
	gh, _ := fakeDispatches(t, []int{204, 422}, func() {})
	p := &prepared{Plan: &Plan{RequestID: "rb-wf", Workflows: wfs}, Matrix: &Matrix{}, GitHub: gh}
	state := filepath.Join(t.TempDir(), "cli-run.json")
	out := &syncBuffer{}
	term := &TerminalRun{StatePath: state, Out: out}

	err := term.Execute(testContext(t), New(&Config{Poll: time.Millisecond}), p, "v1", nil)
	if saved, _ := term.Saved(); !errors.Is(err, errPartialRun) || saved == nil ||
		saved.Progress.Stage != StageDispatching || !strings.Contains(out.String(), "may have been dispatched") {
		t.Fatalf("a dispatch that failed part-way was not kept: err %v, saved %+v\n%s", err, saved, out.String())
	}

	// A resume finds it cut while dispatching: still unknown what started, so it stays.
	saved, _ := term.Saved()
	if resumeErr := term.Resume(testContext(t), New(&Config{}), saved, nil); !errors.Is(resumeErr, errNotResumable) {
		t.Fatalf("resume of a dispatching run: %v", resumeErr)
	}
	if again, _ := term.Saved(); again == nil {
		t.Fatal("the not-resumable run's state was removed")
	}

	f := newFakeJenkins(t)
	f.set("smoke", false, "FAILURE")
	app, jobs := terminalPlan(t, f)
	ended := &TerminalRun{StatePath: filepath.Join(t.TempDir(), "cli-run.json"), Out: &syncBuffer{}}
	if runErr := ended.Execute(testContext(t), app, jobs, "v1", nil); runErr == nil {
		t.Fatal("a failed job did not fail the run")
	}
	if left, _ := ended.Saved(); left != nil {
		t.Fatalf("a run whose jobs all ended stayed saved: %+v", left)
	}
}
