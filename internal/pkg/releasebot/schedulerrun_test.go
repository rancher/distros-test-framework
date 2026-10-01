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
			{job: JenkinsJob{Product: "k3s", Version: "v1", Phase: 4}, abandoned: true},
		}},
		out: []Outcome{
			{Job: JenkinsJob{Product: "rke2", Version: "v2", Phase: 1}, Result: resultSuccess},
			{Job: JenkinsJob{Product: "k3s", Version: "v1", Phase: 3}, Result: "FAILURE"},
		},
	}
	s := &Scheduler{}
	s.publish(st)
	want := "rke2 v2: phase 2; 0 running, 2 waiting, 1 passed, 0 failed or not run\n" +
		"rke2 v1: phase 1; 1 running, 0 waiting, 0 passed, 0 failed or not run\n" +
		"k3s v1: done; 0 running, 0 waiting, 0 passed, 1 failed or not run"
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
	return "", fmt.Errorf("%w: EOF", ErrTriggerUnknown)
}

// A slot held by another run's unknown build makes this run report the controller instead of
// waiting forever; once that run is unblocked, its slots are free again for new runs.
func TestAbandonedSlotsAcrossRuns(t *testing.T) {
	capacity := &Capacity{}
	limits := map[string]Limits{"mower": {MaxConcurrent: 1}}
	sched := func(owner string, b Builder) *Scheduler {
		return &Scheduler{
			Builders: map[string]Builder{"mower": b}, Limits: limits, Poll: time.Millisecond,
			Capacity: capacity, Owner: owner,
		}
	}

	out := sched("A", &unknownBuilder{}).Run(testContext(t), runOf("v1", 1))
	if !errors.Is(out[0].Err, ErrTriggerUnknown) || capacity.Abandoned("mower") != 1 {
		t.Fatalf("run A: %+v, abandoned %d", out, capacity.Abandoned("mower"))
	}

	done := make(chan []Outcome, 1)
	go func() {
		done <- sched("B", &fakeBuilder{polls: map[string]int{}}).Run(testContext(t), runOf("v2", 2))
	}()
	select {
	case out = <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("run B waited forever for a slot held by run A's unknown build")
	}
	for _, o := range out {
		if o.Err == nil || !strings.Contains(o.Err.Error(), "unreachable") {
			t.Fatalf("run B: %+v", o)
		}
	}

	if freed := capacity.ReleaseAbandoned("B"); freed != 0 {
		t.Fatalf("run B held no unknown builds, freed %d", freed)
	}
	if freed := capacity.ReleaseAbandoned("A"); freed != 1 || capacity.InUse("mower") != 0 {
		t.Fatalf("freed %d, in use %d", freed, capacity.InUse("mower"))
	}
	for _, o := range sched("C", &fakeBuilder{polls: map[string]int{}}).Run(testContext(t), runOf("v3", 2)) {
		if o.Result != resultSuccess {
			t.Fatalf("run C after unblock: %+v", o)
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
		return "", fmt.Errorf("%w: sent with a canceled context", ErrTriggerUnknown)
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
