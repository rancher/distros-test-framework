package releasebot

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

func runOf(version string, n int) []JenkinsJob {
	jobs := make([]JenkinsJob, 0, n)
	for i := range n {
		name := fmt.Sprintf("j%d", i)
		jobs = append(jobs, JenkinsJob{Name: name, Controller: "mower", Path: name, Product: "rke2", Version: version})
	}

	return jobs
}

// Two runs sharing one Capacity never exceed the controller's limit together.
func TestSchedulersShareCapacity(t *testing.T) {
	fb := &fakeBuilder{polls: map[string]int{}}
	capacity := &Capacity{}
	newSched := func() *Scheduler {
		return &Scheduler{
			Builders: map[string]Builder{"mower": fb},
			Limits:   map[string]Limits{"mower": {MaxConcurrent: 3}},
			Poll:     time.Millisecond,
			Capacity: capacity,
		}
	}

	var wg sync.WaitGroup
	for _, v := range []string{"v1", "v2"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for _, o := range newSched().Run(testContext(t), runOf(v, 6)) {
				if o.Result != resultSuccess {
					t.Errorf("%s %s: %+v", o.Job.Name, v, o)
				}
			}
		}()
	}
	waitGroup(t, &wg)
	if fb.peak > 3 || capacity.InUse("mower") != 0 {
		t.Fatalf("peak %d (limit 3), slots still in use %d", fb.peak, capacity.InUse("mower"))
	}
}

// stopBuilder keeps builds running until released, so a stop can arrive mid-run.
type stopBuilder struct {
	fakeBuilder
	release chan struct{}
}

func (b *stopBuilder) Finished(ctx context.Context, u string) (done bool, result string, err error) {
	select {
	case <-b.release:
		return b.fakeBuilder.Finished(ctx, u)
	default:
		return false, "", nil
	}
}

// After Stop, pending jobs are reported as stopped while running builds are followed to the end.
func TestSchedulerStopDrains(t *testing.T) {
	sb := &stopBuilder{fakeBuilder: fakeBuilder{polls: map[string]int{}}, release: make(chan struct{})}
	stop := make(chan struct{})
	s := &Scheduler{
		Builders: map[string]Builder{"mower": sb},
		Limits:   map[string]Limits{"mower": {MaxConcurrent: 2}},
		Poll:     time.Millisecond,
		Stop:     stop,
	}

	done := make(chan []Outcome, 1)
	go func() { done <- s.Run(testContext(t), runOf("v1", 5)) }()
	for deadline := time.Now().Add(5 * time.Second); !strings.Contains(s.Status(), "2 running"); {
		if time.Now().After(deadline) {
			t.Fatalf("builds did not start: %q", s.Status())
		}
		time.Sleep(time.Millisecond)
	}
	if got := s.Status(); got != "rke2 v1: in progress; 2 running, 3 waiting, 0 passed, 0 failed or not run" {
		t.Fatalf("status = %q", got)
	}
	close(stop)
	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(s.Status(), "3 failed or not run") {
		if time.Now().After(deadline) {
			t.Fatalf("pending jobs not reported as stopped: %q", s.Status())
		}
		time.Sleep(time.Millisecond)
	}
	close(sb.release)

	var passed, stopped int
	for _, o := range receive(t, done) {
		switch {
		case o.Result == resultSuccess:
			passed++
		case errors.Is(o.Err, ErrStopped):
			stopped++
		default:
			t.Fatalf("unexpected outcome %+v", o)
		}
	}
	if passed != 2 || stopped != 3 {
		t.Fatalf("passed %d, stopped %d; want 2 and 3", passed, stopped)
	}
}

// Status tells per RC which phase it is in and how many jobs are where.
func TestSchedulerStatusPerRC(t *testing.T) {
	st := &runState{
		pending: []JenkinsJob{{Product: "rke2", Version: "v2", Phase: 2}, {Product: "rke2", Version: "v2", Phase: 3}},
		active: map[string][]*running{"mower": {
			{job: JenkinsJob{Product: "rke2", Version: "v1", Phase: 1}},
			{job: JenkinsJob{Name: "up", Product: "k3s", Version: "v1", Phase: 4}, abandoned: true},
		}},
		out: []Outcome{
			{Job: JenkinsJob{Product: "rke2", Version: "v2", Phase: 1}, Result: resultSuccess},
			{Job: JenkinsJob{Name: "gap", Product: "k3s", Version: "v1", Phase: 3}, Result: "FAILURE"},
		},
	}
	s := &Scheduler{}
	s.publish(st)
	want := "rke2 v2: phase 2; 0 running, 2 waiting, 1 passed, 0 failed or not run\n" +
		"rke2 v1: phase 1; 1 running, 0 waiting, 0 passed, 0 failed or not run\n" +
		"k3s v1: done; 0 running, 0 waiting, 1 in unknown state (watched), 0 passed, 1 failed or not run"
	if got := s.Status(); got != want {
		t.Fatalf("status:\n%s\nwant\n%s", got, want)
	}
}

// Status lists RCs newest first by release: v1.36.10 before v1.36.9.
func TestSchedulerStatusOrdersByRelease(t *testing.T) {
	st := &runState{pending: []JenkinsJob{
		{Product: "rke2", Version: "v1.36.9-rc1+rke2r1", Phase: 1},
		{Product: "rke2", Version: "v1.36.10-rc1+rke2r1", Phase: 1},
	}}
	s := &Scheduler{}
	s.publish(st)
	if got := s.Status(); !strings.HasPrefix(got, "rke2 v1.36.10-rc1+rke2r1:") {
		t.Fatalf("status:\n%s", got)
	}
}

// slowTriggerBuilder blocks its first Trigger until released, so a stop can land mid-trigger.
type slowTriggerBuilder struct {
	fakeBuilder
	entered, release chan struct{}
	calls            int
}

func (b *slowTriggerBuilder) Trigger(ctx context.Context, j *JenkinsJob) (string, error) {
	b.mu.Lock()
	b.calls++
	first := b.calls == 1
	b.mu.Unlock()
	if first {
		close(b.entered)
		select {
		case <-b.release:
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}

	return b.fakeBuilder.Trigger(ctx, j)
}

// A stop sent while a trigger is in flight prevents the triggers that would have followed it.
func TestSchedulerStopBetweenTriggers(t *testing.T) {
	sb := &slowTriggerBuilder{
		fakeBuilder: fakeBuilder{polls: map[string]int{}},
		entered:     make(chan struct{}), release: make(chan struct{}),
	}
	stop := make(chan struct{})
	s := &Scheduler{
		Builders: map[string]Builder{"mower": sb},
		Limits:   map[string]Limits{"mower": {MaxConcurrent: 3}},
		Poll:     time.Millisecond,
		Stop:     stop,
	}
	done := make(chan []Outcome, 1)
	go func() { done <- s.Run(testContext(t), runOf("v1", 3)) }()
	receive(t, sb.entered)
	close(stop)
	close(sb.release)

	var passed, stopped int
	for _, o := range receive(t, done) {
		switch {
		case o.Result == resultSuccess:
			passed++
		case errors.Is(o.Err, ErrStopped):
			stopped++
		}
	}
	if sb.calls != 1 || passed != 1 || stopped != 2 {
		t.Fatalf("triggers %d, passed %d, stopped %d; want 1, 1, 2", sb.calls, passed, stopped)
	}
}

// unknownBuilder loses every trigger response: the build may exist, so the slot stays held.
type unknownBuilder struct{ fakeBuilder }

func (*unknownBuilder) Trigger(context.Context, *JenkinsJob) (string, error) {
	return "", fmt.Errorf("%w: EOF", errTriggerUnknown)
}

// A slot held by another run's unknown build makes this run report the controller instead of
// waiting forever; once the first run stops watching that build, its slot is free for new runs.
func TestAbandonedSlotsAcrossRuns(t *testing.T) {
	capacity := &Capacity{}
	limits := map[string]Limits{"mower": {MaxConcurrent: 1}}
	sched := func(owner string, b Builder) *Scheduler {
		return &Scheduler{
			Builders: map[string]Builder{"mower": b}, Limits: limits, Poll: time.Millisecond,
			Capacity: capacity, Owner: owner, WatchPoll: time.Millisecond, WatchLimit: 300 * time.Millisecond,
		}
	}

	runA := make(chan []Outcome, 1)
	go func() { runA <- sched("A", &unknownBuilder{}).Run(testContext(t), runOf("v1", 1)) }()
	for deadline := time.Now().Add(5 * time.Second); capacity.Abandoned("mower") != 1; {
		if time.Now().After(deadline) {
			t.Fatal("run A's lost trigger never held its slot")
		}
		time.Sleep(time.Millisecond)
	}

	for _, o := range sched("B", &fakeBuilder{polls: map[string]int{}}).Run(testContext(t), runOf("v2", 2)) {
		if o.Err == nil || !strings.Contains(o.Err.Error(), "unreachable") {
			t.Fatalf("run B: %+v", o)
		}
	}

	out := receive(t, runA)
	if !errors.Is(out[0].Err, errTriggerUnknown) || capacity.InUse("mower") != 0 || capacity.Abandoned("mower") != 0 {
		t.Fatalf("run A: %+v, in use %d, abandoned %d", out, capacity.InUse("mower"), capacity.Abandoned("mower"))
	}
	for _, o := range sched("C", &fakeBuilder{polls: map[string]int{}}).Run(testContext(t), runOf("v3", 2)) {
		if o.Result != resultSuccess {
			t.Fatalf("run C after A stopped watching: %+v", o)
		}
	}
}

// cancelingBuilder cancels the run from inside its first trigger, as a shutdown would mid-loop.
type cancelingBuilder struct {
	fakeBuilder
	cancel   context.CancelFunc
	triggers int
}

func (b *cancelingBuilder) Trigger(ctx context.Context, j *JenkinsJob) (string, error) {
	b.triggers++
	if ctx.Err() != nil {
		return "", fmt.Errorf("%w: sent with a canceled context", errTriggerUnknown)
	}
	b.cancel()

	return b.fakeBuilder.Trigger(ctx, j)
}

// A canceled run sends nothing more: the pending job ends with context.Canceled and only the
// build really started keeps its slot as unknown.
func TestCanceledRunTriggersNothing(t *testing.T) {
	ctx, cancel := context.WithCancel(testContext(t))
	b := &cancelingBuilder{fakeBuilder: fakeBuilder{polls: map[string]int{}}, cancel: cancel}
	s := &Scheduler{
		Builders: map[string]Builder{"mower": b},
		Limits:   map[string]Limits{"mower": {MaxConcurrent: 2}},
		Poll:     time.Millisecond,
		Owner:    "run",
	}
	out := s.Run(ctx, runOf("v1", 2))
	if b.triggers != 1 || s.Capacity.Abandoned("mower") != 1 {
		t.Fatalf("triggers %d, abandoned %d; want 1 and 1", b.triggers, s.Capacity.Abandoned("mower"))
	}
	for _, o := range out {
		if o.Job.Name == "j1" && !errors.Is(o.Err, context.Canceled) {
			t.Fatalf("pending job: %+v", o)
		}
	}
}

// ctxBuilder answers queries with the context's error once it is canceled, like the real client.
type ctxBuilder struct{ cancelingBuilder }

func (b *ctxBuilder) BuildFromQueue(ctx context.Context, q string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	return b.cancelingBuilder.BuildFromQueue(ctx, q)
}

// Canceling a run on purpose does not log the build queries that fail because of it.
func TestCanceledRunLogsNoQueryFailures(t *testing.T) {
	ctx, cancel := context.WithCancel(testContext(t))
	b := &ctxBuilder{cancelingBuilder{fakeBuilder: fakeBuilder{polls: map[string]int{}}, cancel: cancel}}
	var warns []string
	s := &Scheduler{
		Builders: map[string]Builder{"mower": b},
		Limits:   map[string]Limits{"mower": {MaxConcurrent: 1}},
		Poll:     time.Millisecond,
		Notify: func(level, format string, args ...any) {
			if level == "warn" {
				warns = append(warns, fmt.Sprintf(format, args...))
			}
		},
	}
	s.Run(ctx, runOf("v1", 1))
	for _, w := range warns {
		if strings.Contains(w, "query failed") || strings.Contains(w, "context canceled") {
			t.Fatalf("spurious warning: %q", w)
		}
	}
}

func TestSchedulerRespectsConcurrency(t *testing.T) {
	fb := &fakeBuilder{polls: map[string]int{}, fail: map[string]bool{"bad": true}}
	s := &Scheduler{
		Builders: map[string]Builder{"mower": fb},
		Limits:   map[string]Limits{"mower": {MaxConcurrent: 2}},
		Poll:     time.Millisecond,
	}

	var jobs []JenkinsJob
	for i := range 5 {
		name := fmt.Sprintf("j%d", i)
		jobs = append(jobs, JenkinsJob{Name: name, Controller: "mower", Path: name, Version: "v1"})
	}
	jobs = append(jobs,
		JenkinsJob{Name: "bad", Controller: "mower", Path: "bad"},
		JenkinsJob{Name: "x", Controller: "baler", Path: "x"})

	out := s.Run(context.Background(), jobs)
	if len(out) != len(jobs) {
		t.Fatalf("got %d outcomes, want %d", len(out), len(jobs))
	}
	if fb.peak > 2 {
		t.Fatalf("peak concurrency %d > 2", fb.peak)
	}

	errs := 0
	for _, o := range out {
		if o.Err != nil {
			errs++
		} else if o.Result != "SUCCESS" {
			t.Fatalf("unexpected result %+v", o)
		}
	}
	if errs != 2 {
		t.Fatalf("expected 2 errors (trigger failure, unknown controller), got %d", errs)
	}
}

func runScheduler(t *testing.T, b *scriptBuilder, limit int, paths ...string) (out []Outcome, logs []string) {
	t.Helper()
	var mu sync.Mutex
	s := &Scheduler{
		Builders:      map[string]Builder{"mower": b},
		Limits:        map[string]Limits{"mower": {MaxConcurrent: limit}},
		Poll:          time.Millisecond,
		MaxPollErrors: 3,
		// Unknown builds are not checked again here, and the run ends shortly after losing them.
		WatchPoll: time.Hour, WatchLimit: 20 * time.Millisecond,
		Notify: func(_, f string, a ...any) {
			mu.Lock()
			defer mu.Unlock()
			logs = append(logs, fmt.Sprintf(f, a...))
		},
	}
	jobs := make([]JenkinsJob, 0, len(paths))
	for _, p := range paths {
		jobs = append(jobs, JenkinsJob{Name: p, Controller: "mower", Path: p, Version: "v1"})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out = s.Run(ctx, jobs)
	if ctx.Err() != nil {
		t.Fatal("scheduler did not finish on its own")
	}

	return out, logs
}

// A transient queue error (HTTP 503) must not free the slot of a job jenkins accepted.
func TestSchedulerQueueErrorKeepsSlot(t *testing.T) {
	e503 := errors.New("GET queue: 503 Service Unavailable")
	b := &scriptBuilder{
		queue:    map[string][]error{"a": {e503, e503}},
		finished: map[string][]error{"a": {nil, nil}},
	}
	out, _ := runScheduler(t, b, 1, "a", "b")
	if b.peak > 1 {
		t.Fatalf("peak concurrency %d with limit 1", b.peak)
	}
	for _, o := range out {
		if o.Err != nil || o.Result != "SUCCESS" {
			t.Fatalf("transient errors must recover: %+v", o)
		}
	}
}

// Persistent query errors (403/404) are reported, bounded, and never treated as "running";
// the unknown build keeps its slot, so with limit 1 the controller is marked unreachable.
func TestSchedulerPersistentErrorsGiveUp(t *testing.T) {
	e403 := errors.New("GET build: 403 Forbidden")
	b := &scriptBuilder{finished: map[string][]error{"a": {e403, e403, e403, e403, e403, e403}}}
	out, logs := runScheduler(t, b, 1, "a", "b")

	if b.triggers != 1 {
		t.Fatalf("triggered %d jobs; the stuck slot must block the second", b.triggers)
	}
	byPath := map[string]Outcome{}
	for _, o := range out {
		byPath[o.Job.Path] = o
	}
	if e := byPath["a"].Err; e == nil || !errors.Is(e, e403) || !strings.Contains(e.Error(), "after 3 failed") {
		t.Fatalf("job a: %v", e)
	}
	if e := byPath["b"].Err; e == nil || !strings.Contains(e.Error(), "unreachable") {
		t.Fatalf("job b: %v", e)
	}
	failures := 0
	for _, l := range logs {
		if strings.Contains(l, "status query failed") {
			failures++
		}
	}
	if failures != 3 {
		t.Fatalf("expected 3 reported query failures, got %d: %v", failures, logs)
	}
}

// With spare capacity, one unknown build only takes its own slot: the others still run, and the
// unknown one gets its real result once jenkins answers for it again.
func TestSchedulerUnknownBuildReducesCapacity(t *testing.T) {
	e404 := errors.New("GET build: 404 Not Found")
	b := &scriptBuilder{finished: map[string][]error{"a": {e404, e404, e404}}}
	out, _ := runScheduler(t, b, 2, "a", "b", "c")
	ok := 0
	for _, o := range out {
		if o.Err == nil && o.Result == "SUCCESS" {
			ok++
		}
	}
	if ok != 3 || b.peak > 2 {
		t.Fatalf("want a, b and c to succeed within limit 2; ok=%d peak=%d out=%+v", ok, b.peak, out)
	}
}

// A canceled queue item never ran, so its slot is released right away.
func TestSchedulerCanceledQueueFreesSlot(t *testing.T) {
	b := &scriptBuilder{queue: map[string][]error{"a": {errQueueCanceled}}}
	out, _ := runScheduler(t, b, 1, "a", "b")
	for _, o := range out {
		switch o.Job.Path {
		case "a":
			if !errors.Is(o.Err, errQueueCanceled) {
				t.Fatalf("a: %+v", o)
			}
		case "b":
			if o.Result != "SUCCESS" {
				t.Fatalf("b: %+v", o)
			}
		}
	}
}

// jenkins accepted the POST but the response was lost; the slot must stay taken.
func TestSchedulerLostTriggerResponseKeepsSlot(t *testing.T) {
	b := &scriptBuilder{lost: map[string]bool{"a": true}}
	out, _ := runScheduler(t, b, 1, "a", "b")
	if b.triggers != 1 || b.peak > 1 {
		t.Fatalf("triggers=%d peak=%d: b must not start while a may be running", b.triggers, b.peak)
	}
	got := outcomesByPath(out)
	if !errors.Is(got["a v1"].Err, errTriggerUnknown) {
		t.Fatalf("a: %+v", got["a v1"])
	}
	if e := got["b v1"].Err; e == nil || !strings.Contains(e.Error(), "unreachable") {
		t.Fatalf("b: %+v", got["b v1"])
	}
}

// With limit 2, A unknown and B finished leaves one free slot: C must still run.
func TestSchedulerUnknownJobDoesNotBlockFreeCapacity(t *testing.T) {
	b := &scriptBuilder{lost: map[string]bool{"a": true}, finished: map[string][]error{"b": {nil}}}
	out, _ := runScheduler(t, b, 2, "a", "b", "c")
	got := outcomesByPath(out)
	if got["b v1"].Result != "SUCCESS" || got["c v1"].Result != "SUCCESS" {
		t.Fatalf("b/c must succeed: %+v", out)
	}
	if b.peak > 2 {
		t.Fatalf("peak %d > 2", b.peak)
	}
}

// The scheduler marks builds it lost track of, and unreconciled reports them.
func TestUnreconciledFromScheduler(t *testing.T) {
	b := &scriptBuilder{lost: map[string]bool{"a": true}}
	out, _ := runScheduler(t, b, 2, "a", "b")
	err := unreconciled(out)
	if !errors.Is(err, errUnreconciledBuilds) || !strings.Contains(err.Error(), "a v1") ||
		strings.Contains(err.Error(), "b v1") {
		t.Fatalf("unreconciled = %v", err)
	}

	e403 := errors.New("403")
	b = &scriptBuilder{finished: map[string][]error{"a": {e403, e403, e403, e403, e403, e403}}}
	out, _ = runScheduler(t, b, 2, "a")
	if err = unreconciled(out); !errors.Is(err, errUnreconciledBuilds) || !errors.Is(out[0].Err, errStateUnknown) {
		t.Fatalf("state unknown not reported: %v / %v", err, out[0].Err)
	}
	if unreconciled([]Outcome{{Result: "SUCCESS"}, {Err: errors.New("400 rejected")}}) != nil {
		t.Fatal("finished or rejected jobs are not unreconciled")
	}
}

// Stopping the bot mid-run reports the in-flight builds as unknown, not as a plain error.
func TestSchedulerCancelMarksInFlightUnknown(t *testing.T) {
	b := &scriptBuilder{finished: map[string][]error{"a": {nil, nil, nil, nil, nil, nil, nil, nil, nil, nil}}}
	s := &Scheduler{
		Builders: map[string]Builder{"mower": b}, Limits: map[string]Limits{"mower": {MaxConcurrent: 1}},
		Poll: time.Millisecond,
	}
	ctx, cancel := context.WithCancel(testContext(t))
	defer cancel()
	jobs := []JenkinsJob{
		{Name: "a", Controller: "mower", Path: "a", Version: "v1"},
		{Name: "b", Controller: "mower", Path: "b", Version: "v1"},
	}
	done := make(chan []Outcome, 1)
	go func() { done <- s.Run(ctx, jobs) }()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		b.mu.Lock()
		n := b.triggers
		b.mu.Unlock()
		if n > 0 {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatalf("scheduler never triggered a job: %v", ctx.Err())
		case <-ticker.C:
		}
	}
	cancel()
	out := receive(t, done)
	err := unreconciled(out)
	if !errors.Is(err, errUnreconciledBuilds) || !strings.Contains(err.Error(), "a v1") ||
		strings.Contains(err.Error(), "b v1") {
		t.Fatalf("in-flight job not reported, or never-triggered job reported: %v", err)
	}
}
