package releasebot

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// attemptBuilder finishes each build on its second poll; a job fails (with result) until
// passFrom[path] attempts, so reruns can succeed.
type attemptBuilder struct {
	mu       sync.Mutex
	polls    map[string]int
	passFrom map[string]int // path -> first attempt (0-based) that passes; missing = always passes
	result   string         // failure result (default FAILURE)
	triggers []string
}

func (b *attemptBuilder) Trigger(_ context.Context, j *JenkinsJob) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.triggers = append(b.triggers, fmt.Sprintf("%s#%d", j.Name, j.Attempt))

	return fmt.Sprintf("q/%s/%s/%d", j.Path, j.Version, j.Attempt), nil
}

func (*attemptBuilder) BuildFromQueue(_ context.Context, q string) (string, error) {
	return "b/" + q, nil
}

func (b *attemptBuilder) Finished(_ context.Context, u string) (done bool, result string, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.polls[u]++
	if b.polls[u] < 2 {
		return false, "", nil
	}
	var path, version string
	var attempt int
	if _, err = fmt.Sscanf(strings.ReplaceAll(strings.TrimPrefix(u, "b/q/"), "/", " "), "%s %s %d",
		&path, &version, &attempt); err != nil {
		return true, "", err
	}
	if pass, ok := b.passFrom[path]; ok && attempt < pass {
		if b.result != "" {
			return true, b.result, nil
		}

		return true, "FAILURE", nil
	}

	return true, resultSuccess, nil
}

func (b *attemptBuilder) triggered() []string {
	b.mu.Lock()
	defer b.mu.Unlock()

	return append([]string{}, b.triggers...)
}

// smokeThenConf: conf waits for smoke, like a phase gate.
func smokeThenConf() []JenkinsJob {
	return []JenkinsJob{
		{Name: "smoke", Product: "rke2", Version: "v1", Controller: "mower", Path: "smoke", Phase: 1},
		{
			Name: "conf", Product: "rke2", Version: "v1", Controller: "mower", Path: "conf", Phase: 2,
			DependsOn: []string{"smoke"},
		},
	}
}

type triageHarness struct {
	s        *Scheduler
	b        *attemptBuilder
	commands chan Command
	mu       sync.Mutex
	help     []string
}

func newTriageHarness(passFrom map[string]int, triage Triager) *triageHarness {
	h := &triageHarness{b: &attemptBuilder{polls: map[string]int{}, passFrom: passFrom}, commands: make(chan Command, 1)}
	h.s = &Scheduler{
		Builders: map[string]Builder{"mower": h.b},
		Limits:   map[string]Limits{"mower": {MaxConcurrent: 2}},
		Poll:     time.Millisecond,
		Triage:   triage,
		Commands: h.commands,
		Help: func(m string) {
			h.mu.Lock()
			h.help = append(h.help, m)
			h.mu.Unlock()
		},
	}

	return h
}

func (h *triageHarness) waitHelp(t *testing.T, n int) string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		h.mu.Lock()
		got := append([]string{}, h.help...)
		h.mu.Unlock()
		if len(got) >= n {
			return got[n-1]
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("no help request #%d", n)

	return ""
}

func (h *triageHarness) send(t *testing.T, c Command) string {
	t.Helper()
	reply := make(chan string, 1)
	c.Reply = reply
	h.commands <- c
	select {
	case r := <-reply:
		return r
	case <-time.After(5 * time.Second):
		t.Fatal("no reply to command")
	}

	return ""
}

func rerunTriage(context.Context, *Outcome) Decision {
	return Decision{Rerun: true, Summary: "flaky provisioning"}
}

func runAsync(s *Scheduler, jobs []JenkinsJob) <-chan []Outcome {
	done := make(chan []Outcome, 1)
	go func() { done <- s.Run(context.Background(), jobs) }()

	return done
}

func waitOut(t *testing.T, done <-chan []Outcome) map[string]Outcome {
	t.Helper()
	select {
	case out := <-done:
		m := map[string]Outcome{}
		for i := range out {
			m[out[i].Job.Name] = out[i]
		}

		return m
	case <-time.After(5 * time.Second):
		t.Fatal("run did not end")
	}

	return nil
}

// A failure triage calls transient is rerun once; the rerun passes and the next phase runs.
func TestTriageRerunsOnce(t *testing.T) {
	h := newTriageHarness(map[string]int{"smoke": 1}, rerunTriage)
	out := waitOut(t, runAsync(h.s, smokeThenConf()))
	if out["smoke"].Result != resultSuccess || out["conf"].Result != resultSuccess {
		t.Fatalf("outcomes %+v", out)
	}
	if got := strings.Join(h.b.triggered(), " "); got != "smoke#0 smoke#1 conf#0" {
		t.Fatalf("triggers %q", got)
	}
}

// No second automatic rerun and none for an aborted build: the job waits for a person, and triage
// is not even run there, since its answer could not change anything.
func TestTriageFixedLimits(t *testing.T) {
	for name, tc := range map[string]*struct {
		passFrom int
		result   string
		want     string
		triggers string
		triaged  int32
	}{
		"second failure": {
			passFrom: 9, want: "no automatic rerun is possible (it was already rerun once)",
			triggers: "smoke#0 smoke#1", triaged: 1,
		},
		"aborted": {
			passFrom: 9, result: "ABORTED", want: "no automatic rerun is possible (the build was aborted)",
			triggers: "smoke#0", triaged: 0,
		},
	} {
		t.Run(name, func(t *testing.T) {
			var calls atomic.Int32
			counting := func(ctx context.Context, o *Outcome) Decision {
				calls.Add(1)
				return rerunTriage(ctx, o)
			}
			h := newTriageHarness(map[string]int{"smoke": tc.passFrom}, counting)
			h.b.result = tc.result
			done := runAsync(h.s, smokeThenConf())
			if msg := h.waitHelp(t, 1); !strings.Contains(msg, tc.want) || !strings.Contains(msg, "retry smoke v1") {
				t.Fatalf("help: %q", msg)
			}
			if got := strings.Join(h.b.triggered(), " "); got != tc.triggers {
				t.Fatalf("triggers %q, want %q (conf must wait)", got, tc.triggers)
			}
			if n := calls.Load(); n != tc.triaged {
				t.Fatalf("triaged %d times, want %d", n, tc.triaged)
			}
			r := h.send(t, Command{Action: CommandSkip, Job: "smoke", By: "U1"})
			if !strings.Contains(r, "Skipped smoke v1") {
				t.Fatalf("skip reply %q", r)
			}
			waitOut(t, done)
		})
	}
}

// A job waiting for help holds its dependents; `retry` runs it again and the run carries on.
func TestHeldJobRetry(t *testing.T) {
	h := newTriageHarness(map[string]int{"smoke": 1}, askTriager)
	done := runAsync(h.s, smokeThenConf())
	msg := h.waitHelp(t, 1)
	if !strings.Contains(msg, "Needs help: smoke v1 ended with FAILURE (b/q/smoke/v1/0)") ||
		!strings.Contains(msg, "automatic triage is not set up yet") {
		t.Fatalf("help: %q", msg)
	}
	if st := h.s.Status(); !strings.Contains(st, "1 need help") {
		t.Fatalf("status %q", st)
	}

	if r := h.send(t, Command{Action: CommandRetry, Job: "nope", By: "U1"}); !strings.Contains(r, "Waiting: smoke v1") {
		t.Fatalf("unknown job reply %q", r)
	}
	r := h.send(t, Command{Action: CommandRetry, Job: "smoke", Version: "v1", By: "U1"})
	if r != "Retrying smoke v1, asked by <@U1>." {
		t.Fatalf("retry reply %q", r)
	}
	out := waitOut(t, done)
	if out["smoke"].Result != resultSuccess || out["conf"].Result != resultSuccess {
		t.Fatalf("outcomes %+v", out)
	}
}

// `skip` counts the job as passed: the next phase starts, and the outcome says SKIPPED.
func TestHeldJobSkip(t *testing.T) {
	h := newTriageHarness(map[string]int{"smoke": 9}, askTriager)
	done := runAsync(h.s, smokeThenConf())
	h.waitHelp(t, 1)
	h.send(t, Command{Action: CommandSkip, Job: "smoke", By: "U1"})
	out := waitOut(t, done)
	if out["smoke"].Result != "SKIPPED" || out["conf"].Result != resultSuccess {
		t.Fatalf("outcomes %+v", out)
	}
}

// An unknown action (a new verb, a typo) changes nothing: the job keeps waiting for help.
func TestHeldJobUnknownAction(t *testing.T) {
	h := newTriageHarness(map[string]int{"smoke": 9}, askTriager)
	done := runAsync(h.s, smokeThenConf())
	h.waitHelp(t, 1)
	if r := h.send(t, Command{Action: "skpi", Job: "smoke", By: "U1"}); !strings.Contains(r, `Unknown action "skpi"`) {
		t.Fatalf("reply %q", r)
	}
	if st := h.s.Status(); !strings.Contains(st, "1 need help") {
		t.Fatalf("status %q", st)
	}
	h.send(t, Command{Action: CommandSkip, Job: "smoke", By: "U1"})
	waitOut(t, done)
}

// A command without a buffered Reply cannot block the scheduler: the reply is dropped and the
// run keeps answering.
func TestCommandWithoutReplyDoesNotBlock(t *testing.T) {
	h := newTriageHarness(map[string]int{"smoke": 9}, askTriager)
	done := runAsync(h.s, smokeThenConf())
	h.waitHelp(t, 1)
	h.commands <- Command{Action: CommandTriage, Job: "nope", By: "U1"}
	h.commands <- Command{Action: CommandTriage, Job: "nope", By: "U1", Reply: make(chan string)}
	if r := h.send(t, Command{Action: CommandSkip, Job: "smoke", By: "U1"}); !strings.Contains(r, "Skipped smoke") {
		t.Fatalf("reply %q", r)
	}
	waitOut(t, done)
}

// With two waiting jobs of the same name, the RC must be given.
func TestHeldJobAmbiguous(t *testing.T) {
	h := newTriageHarness(map[string]int{"smoke": 9}, askTriager)
	jobs := []JenkinsJob{
		{Name: "smoke", Product: "rke2", Version: "v1", Controller: "mower", Path: "smoke"},
		{Name: "smoke", Product: "rke2", Version: "v2", Controller: "mower", Path: "smoke"},
	}
	done := runAsync(h.s, jobs)
	h.waitHelp(t, 2)
	if r := h.send(t, Command{Action: CommandSkip, Job: "smoke", By: "U1"}); !strings.Contains(r, "add the RC") {
		t.Fatalf("reply %q", r)
	}
	h.send(t, Command{Action: CommandSkip, Job: "smoke", Version: "v1", By: "U1"})
	h.send(t, Command{Action: CommandSkip, Job: "smoke", Version: "v2", By: "U1"})
	waitOut(t, done)
}

// `stop` finishes jobs waiting for help, so the run ends; later commands are refused.
func TestStopEndsHeldJobs(t *testing.T) {
	h := newTriageHarness(map[string]int{"smoke": 9}, askTriager)
	stop := make(chan struct{})
	h.s.Stop = stop
	done := runAsync(h.s, smokeThenConf())
	h.waitHelp(t, 1)
	close(stop)
	out := waitOut(t, done)
	if out["smoke"].Result != "FAILURE" || out["conf"].Err == nil {
		t.Fatalf("outcomes %+v", out)
	}
}

// Without a commands channel nobody can answer: a failure fails the job instead of waiting.
func TestNoCommandsNeverWaits(t *testing.T) {
	h := newTriageHarness(map[string]int{"smoke": 9}, askTriager)
	h.s.Commands = nil
	out := waitOut(t, runAsync(h.s, smokeThenConf()))
	if out["smoke"].Result != "FAILURE" || out["conf"].Err == nil {
		t.Fatalf("outcomes %+v", out)
	}
}

// holdBuilder keeps the "long" build running until released, so a run stays alive after stop.
type holdBuilder struct {
	*attemptBuilder
	release chan struct{}
}

func (b *holdBuilder) Finished(ctx context.Context, u string) (done bool, result string, err error) {
	if strings.Contains(u, "/long/") {
		select {
		case <-b.release:
			return true, resultSuccess, nil
		default:
			return false, "", nil
		}
	}

	return b.attemptBuilder.Finished(ctx, u)
}

// stop ends jobs still in triage at once, even with a triager that ignores cancellation (a stuck
// external call), and a verdict that arrives later is ignored while the run is still alive.
func TestStopEndsTriage(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	stuck := func(context.Context, *Outcome) Decision {
		close(entered)
		<-release

		return Decision{Rerun: true, Summary: "late"}
	}
	h := newTriageHarness(map[string]int{"smoke": 9}, stuck)
	hb := &holdBuilder{attemptBuilder: h.b, release: make(chan struct{})}
	h.s.Builders = map[string]Builder{"mower": hb}
	stop := make(chan struct{})
	h.s.Stop = stop
	long := JenkinsJob{Name: "long", Product: "rke2", Version: "v2", Controller: "mower", Path: "long"}
	jobs := append(smokeThenConf(), long)
	done := runAsync(h.s, jobs)
	<-entered
	close(stop)

	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(h.s.Status(), "rke2 v1: done") {
		if time.Now().After(deadline) {
			t.Fatalf("smoke still in triage after stop: %q", h.s.Status())
		}
		time.Sleep(time.Millisecond)
	}
	close(release) // the late verdict arrives while "long" keeps the run alive
	time.Sleep(20 * time.Millisecond)
	close(hb.release)

	var smoke int
	for _, o := range <-waitDone(t, done) {
		if o.Job.Name == "smoke" {
			smoke++
		}
	}
	if smoke != 1 || strings.Contains(strings.Join(h.b.triggered(), " "), "smoke#1") {
		t.Fatalf("smoke outcomes %d, triggers %v: the late verdict was acted on", smoke, h.b.triggered())
	}
}

func waitDone(t *testing.T, done <-chan []Outcome) <-chan []Outcome {
	t.Helper()
	out := make(chan []Outcome, 1)
	select {
	case o := <-done:
		out <- o
	case <-time.After(5 * time.Second):
		t.Fatal("run did not end")
	}

	return out
}

// A "Needs help" post that hangs (slow Slack) does not hold up the other jobs of the run; the run
// ends once the post returns (real posts are bounded by PostTimeout).
func TestBlockedHelpDoesNotStallScheduler(t *testing.T) {
	h := newTriageHarness(map[string]int{"bad": 9}, askTriager)
	h.s.Limits = map[string]Limits{"mower": {MaxConcurrent: 1}}
	hang := make(chan struct{})
	posted := make(chan string, 1)
	h.s.Help = func(msg string) {
		posted <- msg
		<-hang
	}
	jobs := []JenkinsJob{
		{Name: "bad", Product: "rke2", Version: "v1", Controller: "mower", Path: "bad"},
		{Name: "good", Product: "rke2", Version: "v2", Controller: "mower", Path: "good"},
	}
	done := runAsync(h.s, jobs)
	<-posted // the help post is now stuck

	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(h.s.Status(), "rke2 v2: done") {
		if time.Now().After(deadline) {
			t.Fatalf("the next job did not run while help was posting: %q, triggers %v", h.s.Status(), h.b.triggered())
		}
		time.Sleep(time.Millisecond)
	}
	h.send(t, Command{Action: CommandSkip, Job: "bad", By: "U1"})
	close(hang) // the slow post returns
	out := waitOut(t, done)
	if out["good"].Result != resultSuccess || out["bad"].Result != ResultSkipped {
		t.Fatalf("outcomes %+v", out)
	}
}
