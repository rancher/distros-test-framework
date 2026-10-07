package releasebot

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func testContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)

	return ctx
}

func receive[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for test goroutine")
	}

	var zero T

	return zero
}

func waitGroup(t *testing.T, wg *sync.WaitGroup) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	receive(t, done)
}

func testMatrix(t *testing.T) *Matrix {
	t.Helper()
	path := filepath.Join(t.TempDir(), "m.yaml")
	body := `
controllers:
  mower: {url: https://mower.example, maxConcurrent: 2}
defaultParams:
  INSTALL_VERSION: "{{VERSION}}"
  HOSTNAME_PREFIX: "{{PREFIX}}"
jobs:
  - {name: r-slow, product: rke2, controller: mower, path: a/rke2_slow, priority: 3, code: rs}
  - {name: r-smoke, product: rke2, controller: mower, path: a/rke2_smoke, priority: 1, code: rm}
  - name: k-smoke
    product: k3s
    controller: mower
    path: a/k3s_smoke
    priority: 1
    code: k
    params: {HOSTNAME_PREFIX: "{{PREFIX}}x"}
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	m, err := loadMatrix(path)
	if err != nil {
		t.Fatal(err)
	}

	return m
}

// fakeBuilder starts every queued item on the next poll and finishes it on the one after.
type fakeBuilder struct {
	mu      sync.Mutex
	polls   map[string]int
	running int
	peak    int
	fail    map[string]bool
}

func (f *fakeBuilder) Trigger(_ context.Context, j *JenkinsJob) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail[j.Path] {
		return "", errors.New("boom")
	}
	f.running++
	if f.running > f.peak {
		f.peak = f.running
	}

	return "q/" + j.Path + "/" + j.Version, nil
}

func (*fakeBuilder) BuildFromQueue(_ context.Context, q string) (string, error) {
	return "b/" + q, nil
}

func (f *fakeBuilder) Finished(_ context.Context, b string) (done bool, result string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.polls[b]++
	if f.polls[b] < 2 {
		return false, "", nil
	}
	f.running--

	return true, "SUCCESS", nil
}

var testNow = time.Date(2026, time.September, 28, 12, 0, 0, 0, time.UTC)

// scriptBuilder answers queue/build queries from per-job scripts and tracks real concurrency:
// a build counts as running from Trigger until Finished reports it done.
type scriptBuilder struct {
	mu       sync.Mutex
	queue    map[string][]error // per path: errors returned by BuildFromQueue before it succeeds
	finished map[string][]error // per path: errors returned by Finished before it reports done
	lost     map[string]bool    // per path: jenkins accepts the trigger but the response is lost
	results  map[string]string  // per path: final result (default SUCCESS)
	order    []string           // paths in trigger order
	running  int
	peak     int
	triggers int
}

func (b *scriptBuilder) Trigger(_ context.Context, j *JenkinsJob) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.triggers++
	b.running++
	b.peak = max(b.peak, b.running)
	b.order = append(b.order, j.Path)
	if b.lost[j.Path] {
		return "", fmt.Errorf("trigger %s: %w: EOF", j.Path, errTriggerUnknown)
	}

	return j.Path, nil
}

func (b *scriptBuilder) BuildFromQueue(_ context.Context, path string) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if errs := b.queue[path]; len(errs) > 0 {
		b.queue[path] = errs[1:]
		if errors.Is(errs[0], errQueueCanceled) {
			b.running--
		}

		return "", errs[0]
	}

	return "build/" + path, nil
}

func (b *scriptBuilder) Finished(_ context.Context, u string) (done bool, result string, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	path := strings.TrimPrefix(u, "build/")
	if errs := b.finished[path]; len(errs) > 0 {
		b.finished[path] = errs[1:]
		if errs[0] != nil {
			return false, "", errs[0]
		}

		return false, "", nil // still building
	}
	b.running--
	if r, ok := b.results[path]; ok {
		return true, r, nil
	}

	return true, "SUCCESS", nil
}

func outcomesByPath(out []Outcome) map[string]Outcome {
	m := map[string]Outcome{}
	for i := range out {
		m[out[i].Job.Path+" "+out[i].Job.Version] = out[i]
	}

	return m
}

func runJobs(t *testing.T, b *scriptBuilder, limit int, jobs []JenkinsJob) []Outcome {
	t.Helper()
	s := &Scheduler{
		Builders:      map[string]Builder{"mower": b},
		Limits:        map[string]Limits{"mower": {MaxConcurrent: limit}},
		Poll:          time.Millisecond,
		MaxPollErrors: 3,
		// Unknown builds are not checked again here, and the run ends shortly after losing them.
		WatchPoll: time.Hour, WatchLimit: 20 * time.Millisecond,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out := s.Run(ctx, jobs)
	if ctx.Err() != nil {
		t.Fatal("scheduler did not finish on its own")
	}
	if len(out) != len(jobs) {
		t.Fatalf("got %d outcomes for %d jobs: %+v", len(out), len(jobs), out)
	}

	return out
}
