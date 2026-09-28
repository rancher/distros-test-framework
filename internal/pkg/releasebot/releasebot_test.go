package releasebot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	qaseclient "github.com/qase-tms/qase-go/qase-api-client"

	"github.com/rancher/distros-test-framework/internal/pkg/qase"
)

func TestParseRequest(t *testing.T) {
	msg := "please test `v1.37.1-rc1+k3s1` v1.36.5-rc1%2Bk3s1 and " +
		"<https://github.com/rancher/rke2/releases/tag/v1.37.1-rc2+rke2r1|v1.37.1-rc2+rke2r1>" +
		" v1.37.1-rc1+k3s1 lts=v1.33.9-rc1+rke2r1,v1.32.12-rc1+rke2r1"

	got := ParseRequest(msg)
	want := Request{
		K3s:     []string{"v1.36.5-rc1+k3s1", "v1.37.1-rc1+k3s1"},
		RKE2:    []string{"v1.37.1-rc2+rke2r1"},
		RKE2LTS: []string{"v1.32.12-rc1+rke2r1", "v1.33.9-rc1+rke2r1"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}

	if !ParseRequest("hello bot").Empty() {
		t.Fatal("expected empty request")
	}
}

func TestBaseVersion(t *testing.T) {
	if got := BaseRC("v1.37.1-rc2+rke2r1"); got != "v1.37.1-rc2" {
		t.Fatal(got)
	}
	if got := BaseVersion("v1.37.1-rc2+rke2r1"); got != "v1.37.1" {
		t.Fatal(got)
	}
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
  - {name: r-slow, product: rke2, controller: mower, path: a/rke2_slow, priority: 3}
  - {name: r-smoke, product: rke2, controller: mower, path: a/rke2_smoke, priority: 1}
  - name: k-smoke
    product: k3s
    controller: mower
    path: a/k3s_smoke
    priority: 1
    params: {HOSTNAME_PREFIX: "{{PREFIX}}x"}
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	m, err := LoadMatrix(path)
	if err != nil {
		t.Fatal(err)
	}

	return m
}

func TestLoadMatrixRejectsUnknownController(t *testing.T) {
	path := filepath.Join(t.TempDir(), "m.yaml")
	_ = os.WriteFile(path, []byte("jobs:\n  - {name: x, product: k3s, controller: nope, path: a}\n"), 0o600)
	if _, err := LoadMatrix(path); err == nil {
		t.Fatal("expected error for unknown controller")
	}
}

func TestBuildPlan(t *testing.T) {
	m := testMatrix(t)
	req := Request{
		K3s:  []string{"v1.36.5-rc1+k3s1", "v1.37.1-rc1+k3s1"},
		RKE2: []string{"v1.37.1-rc2+rke2r1"},
	}

	p, err := BuildPlan(req, m, testNow, "rb-test")
	if err != nil {
		t.Fatal(err)
	}

	if m.DTFRef != "main" || p.Workflows[0].Inputs["k3s_versions"] != "v1.36.5-rc1+k3s1,v1.37.1-rc1+k3s1" {
		t.Fatalf("release-checks inputs: %+v", p.Workflows[0])
	}
	if got := p.Workflows[1].Inputs["rcs"]; got != "v1.36.5-rc1,v1.37.1-rc2" {
		t.Fatalf("qase rcs = %q", got)
	}
	if len(p.Warnings) != 1 {
		t.Fatalf("expected one rc mismatch warning, got %v", p.Warnings)
	}

	var order []string
	for _, j := range p.Jobs {
		order = append(order, j.Path+" "+j.Version)
	}
	want := []string{
		"a/rke2_smoke v1.37.1-rc2+rke2r1",
		"a/k3s_smoke v1.37.1-rc1+k3s1",
		"a/k3s_smoke v1.36.5-rc1+k3s1",
		"a/rke2_slow v1.37.1-rc2+rke2r1",
	}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("order:\n%v\nwant\n%v", order, want)
	}

	k := p.Jobs[1]
	if k.Params["INSTALL_VERSION"] != "v1.37.1-rc1+k3s1" || k.Params["HOSTNAME_PREFIX"] != "rbk1371x" {
		t.Fatalf("params: %v", k.Params)
	}
}

func TestBuildPlanEmpty(t *testing.T) {
	if _, err := BuildPlan(Request{}, testMatrix(t), testNow, "rb-test"); err == nil {
		t.Fatal("expected error")
	}
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

var testNow = time.Date(2026, time.September, 28, 12, 0, 0, 0, time.UTC)

func TestQaseRunTitleMatchesScript(t *testing.T) {
	// scripts/qase-patch-validation.sh: "$PRODUCT $MONTH $YEAR Patch Validation for $VERSION+<rke2r1|k3s1>".
	got := QaseRunTitle("rke2", "v1.37.1-rc2+rke2r2", testNow)
	if got != "RKE2 September 2026 Patch Validation for v1.37.1+rke2r1" {
		t.Fatal(got)
	}
	if got := QaseRunTitle("k3s", "v1.36.5-rc1", testNow); got != "K3S September 2026 Patch Validation for v1.36.5+k3s1" {
		t.Fatal(got)
	}
}

// fakeQase serves a scripted sequence of search results per title (the last one repeats).
type fakeQase struct {
	mu    sync.Mutex
	calls map[string]int
	runs  map[string][][]QaseRun
}

func (f *fakeQase) SearchRuns(_ context.Context, title string) ([]QaseRun, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	seq := f.runs[title]
	n := min(f.calls[title], len(seq)-1)
	f.calls[title]++

	return seq[n], nil
}

func TestQaseRunIDsFlowIntoJobs(t *testing.T) {
	m := testMatrix(t)
	m.Defaults["QASE_RUN_ID"] = "{{QASE_RUN_ID}}"
	p, err := BuildPlan(Request{RKE2: []string{"v1.37.1-rc1+rke2r1"}}, m, testNow, "rb-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(p.QaseTitles) != 2 || p.Workflows[1].Inputs["request_id"] != "rb-1" {
		t.Fatalf("titles %v, qase inputs %v", p.QaseTitles, p.Workflows[1].Inputs)
	}

	rTitle := "RKE2 September 2026 Patch Validation for v1.37.1+rke2r1"
	kTitle := "K3S September 2026 Patch Validation for v1.37.1+k3s1"
	mine := func(id int64, title string) QaseRun {
		return QaseRun{ID: id, Title: title, Description: "Version: v1.37.1-rc1 | Release bot request: rb-1"}
	}
	fq := &fakeQase{calls: map[string]int{}, runs: map[string][][]QaseRun{
		rTitle: {{}, {mine(1016, rTitle)}},
		kTitle: {{mine(1017, kTitle)}},
	}}

	ids, err := WaitQaseRuns(context.Background(), fq, p.QaseTitles, "rb-1", time.Millisecond, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if ids[rTitle] != 1016 || ids[kTitle] != 1017 {
		t.Fatalf("ids = %v", ids)
	}

	if err = ApplyQaseRunIDs(p.Jobs, ids); err != nil {
		t.Fatal(err)
	}
	for _, j := range p.Jobs {
		if j.Params["QASE_RUN_ID"] != "1016" {
			t.Fatalf("%s QASE_RUN_ID = %q", j.Path, j.Params["QASE_RUN_ID"])
		}
	}
}

// A run from another dispatch (manual or an earlier workflow finishing late) with the same title
// must not be taken, even when it is newer: only the run carrying this request id counts.
func TestMatchQaseRunIgnoresOtherDispatches(t *testing.T) {
	title := "RKE2 September 2026 Patch Validation for v1.37.1+rke2r1"
	runs := []QaseRun{
		{ID: 900, Title: title, Description: "Version: v1.37.1-rc1"},
		{ID: 901, Title: title, Description: "Version: v1.37.1-rc1 | Release bot request: rb-other"},
		{ID: 903, Title: title, Description: "Version: v1.37.1-rc1 | Release bot request: rb-1x"},
		{ID: 902, Title: title, Description: "Version: v1.37.1-rc1 | Release bot request: rb-1"},
		{ID: 950, Title: title + " (copy)", Description: "Version: v1.37.1-rc1 | Release bot request: rb-1"},
	}
	if id, ok := MatchQaseRun(runs, title, "rb-1"); !ok || id != 902 {
		t.Fatalf("got %d %v, want 902", id, ok)
	}
	if _, ok := MatchQaseRun(runs[:2], title, "rb-1"); ok {
		t.Fatal("must wait while only other dispatches' runs exist")
	}
	// Runs created by hand (-skip-workflows): newest with the exact title.
	if id, _ := MatchQaseRun(runs, title, ""); id != 903 {
		t.Fatalf("got %d, want 903", id)
	}
}

func TestWaitQaseRunsTimesOut(t *testing.T) {
	title := "RKE2 September 2026 Patch Validation for v1.37.1+rke2r1"
	fq := &fakeQase{calls: map[string]int{}, runs: map[string][][]QaseRun{
		title: {{{ID: 901, Title: title, Description: "Version: v1.37.1-rc1 | Release bot request: rb-other"}}},
	}}
	if _, err := WaitQaseRuns(context.Background(), fq, []string{title}, "rb-1",
		time.Millisecond, 20*time.Millisecond); err == nil {
		t.Fatal("expected timeout: only another dispatch's run exists")
	}
}

func TestNewRequestIDMatchesScriptCharset(t *testing.T) {
	id := NewRequestID(testNow)
	if !regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`).MatchString(id) || !strings.HasPrefix(id, "rb-20260928T120000-") {
		t.Fatal(id)
	}
	if id == NewRequestID(testNow) {
		t.Fatal("ids must differ within the same second")
	}
}

func TestApplyQaseRunIDsMissing(t *testing.T) {
	jobs := []JenkinsJob{{Path: "a", QaseTitle: "x", Params: map[string]string{"QASE_RUN_ID": "{{QASE_RUN_ID}}"}}}
	if err := ApplyQaseRunIDs(jobs, map[string]int64{}); err == nil {
		t.Fatal("expected error for unresolved run")
	}
}

// scriptBuilder answers queue/build queries from per-job scripts and tracks real concurrency:
// a build counts as running from Trigger until Finished reports it done.
type scriptBuilder struct {
	mu       sync.Mutex
	queue    map[string][]error // per path: errors returned by BuildFromQueue before it succeeds
	finished map[string][]error // per path: errors returned by Finished before it reports done
	lost     map[string]bool    // per path: Jenkins accepts the trigger but the response is lost
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
		return "", fmt.Errorf("trigger %s: %w: EOF", j.Path, ErrTriggerUnknown)
	}

	return j.Path, nil
}

func (b *scriptBuilder) BuildFromQueue(_ context.Context, path string) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if errs := b.queue[path]; len(errs) > 0 {
		b.queue[path] = errs[1:]
		if errors.Is(errs[0], ErrQueueCanceled) {
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

func runScheduler(t *testing.T, b *scriptBuilder, limit int, paths ...string) (out []Outcome, logs []string) {
	t.Helper()
	var mu sync.Mutex
	s := &Scheduler{
		Builders:      map[string]Builder{"mower": b},
		Limits:        map[string]Limits{"mower": {MaxConcurrent: limit}},
		Poll:          time.Millisecond,
		MaxPollErrors: 3,
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

// Review P1: a transient queue error (HTTP 503) must not free the slot of a job Jenkins accepted.
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

// Review P2: persistent query errors (403/404) are reported, bounded, and never treated as "running";
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

// With spare capacity, one unknown build only takes its own slot: the others still run.
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
	if ok != 2 || b.peak > 2 {
		t.Fatalf("want b and c to succeed within limit 2; ok=%d peak=%d out=%+v", ok, b.peak, out)
	}
}

// A canceled queue item never ran, so its slot is released right away.
func TestSchedulerCanceledQueueFreesSlot(t *testing.T) {
	b := &scriptBuilder{queue: map[string][]error{"a": {ErrQueueCanceled}}}
	out, _ := runScheduler(t, b, 1, "a", "b")
	for _, o := range out {
		switch o.Job.Path {
		case "a":
			if !errors.Is(o.Err, ErrQueueCanceled) {
				t.Fatalf("a: %+v", o)
			}
		case "b":
			if o.Result != "SUCCESS" {
				t.Fatalf("b: %+v", o)
			}
		}
	}
}

func outcomesByPath(out []Outcome) map[string]Outcome {
	m := map[string]Outcome{}
	for i := range out {
		m[out[i].Job.Path+" "+out[i].Job.Version] = out[i]
	}

	return m
}

// Review P1: Jenkins accepted the POST but the response was lost; the slot must stay taken.
func TestSchedulerLostTriggerResponseKeepsSlot(t *testing.T) {
	b := &scriptBuilder{lost: map[string]bool{"a": true}}
	out, _ := runScheduler(t, b, 1, "a", "b")
	if b.triggers != 1 || b.peak > 1 {
		t.Fatalf("triggers=%d peak=%d: b must not start while a may be running", b.triggers, b.peak)
	}
	got := outcomesByPath(out)
	if !errors.Is(got["a v1"].Err, ErrTriggerUnknown) {
		t.Fatalf("a: %+v", got["a v1"])
	}
	if e := got["b v1"].Err; e == nil || !strings.Contains(e.Error(), "unreachable") {
		t.Fatalf("b: %+v", got["b v1"])
	}
}

// Review P2: with limit 2, A unknown and B finished leaves one free slot: C must still run.
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

func depJobs(versions ...string) []JenkinsJob {
	var jobs []JenkinsJob
	for _, v := range versions {
		jobs = append(jobs,
			JenkinsJob{Name: "smoke", Product: "rke2", Version: v, Controller: "mower", Path: "smoke", Priority: 1},
			JenkinsJob{
				Name: "conf", Product: "rke2", Version: v, Controller: "mower", Path: "conf", Priority: 2,
				DependsOn: []string{"smoke"},
			},
			JenkinsJob{
				Name: "rpm", Product: "rke2", Version: v, Controller: "mower", Path: "rpm", Priority: 2,
				DependsOn: []string{"conf"},
			},
		)
	}

	return jobs
}

func runJobs(t *testing.T, b *scriptBuilder, limit int, jobs []JenkinsJob) []Outcome {
	t.Helper()
	s := &Scheduler{
		Builders:      map[string]Builder{"mower": b},
		Limits:        map[string]Limits{"mower": {MaxConcurrent: limit}},
		Poll:          time.Millisecond,
		MaxPollErrors: 3,
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

// Dependents start only after their dependency succeeded, and transitively.
func TestDependsOnReleasesAfterSuccess(t *testing.T) {
	b := &scriptBuilder{finished: map[string][]error{"smoke": {nil, nil}}}
	out := runJobs(t, b, 3, depJobs("v1"))
	for _, o := range out {
		if o.Result != "SUCCESS" {
			t.Fatalf("%+v", o)
		}
	}
	if strings.Join(b.order, ",") != "smoke,conf,rpm" {
		t.Fatalf("trigger order %v", b.order)
	}
}

// A failed smoke blocks its own dependents (transitively) but not the other version.
// Jobs waiting for dependencies take no capacity: with limit 1 the v2 chain still runs.
func TestDependsOnFailureBlocksOnlySameVersion(t *testing.T) {
	b := &scriptBuilder{results: map[string]string{}}
	jobs := depJobs("v1", "v2")
	// only v1's smoke fails: tell them apart by path
	for i := range jobs {
		jobs[i].Path += "-" + jobs[i].Version
	}
	b.results["smoke-v1"] = "FAILURE"

	out := runJobs(t, b, 1, jobs)
	got := outcomesByPath(out)
	if got["smoke-v1 v1"].Result != "FAILURE" {
		t.Fatalf("smoke v1: %+v", got["smoke-v1 v1"])
	}
	for _, p := range []string{"conf-v1 v1", "rpm-v1 v1"} {
		if e := got[p].Err; e == nil || !strings.Contains(e.Error(), "not triggered: dependency") {
			t.Fatalf("%s must be blocked: %+v", p, got[p])
		}
	}
	for _, p := range []string{"smoke-v2 v2", "conf-v2 v2", "rpm-v2 v2"} {
		if got[p].Result != "SUCCESS" {
			t.Fatalf("%s must run: %+v", p, got[p])
		}
	}
	if b.peak > 1 {
		t.Fatalf("peak %d > 1", b.peak)
	}
}

// Unknown state (lost trigger response) is not a success: dependents stay blocked.
func TestDependsOnUnknownBlocksDependents(t *testing.T) {
	b := &scriptBuilder{lost: map[string]bool{"smoke": true}}
	out := runJobs(t, b, 2, depJobs("v1"))
	got := outcomesByPath(out)
	if !errors.Is(got["smoke v1"].Err, ErrTriggerUnknown) || got["conf v1"].Err == nil || got["rpm v1"].Err == nil {
		t.Fatalf("%+v", out)
	}
	if b.triggers != 1 {
		t.Fatalf("only smoke may be triggered, got %v", b.order)
	}
}

// Unknown references and cycles are rejected before anything is triggered.
func TestDependsOnValidatedBeforeTriggering(t *testing.T) {
	unknown := depJobs("v1")
	unknown[2].DependsOn = []string{"nope"}
	cycle := depJobs("v1")
	cycle[0].DependsOn = []string{"rpm"}
	// Same product/version/name twice (e.g. a plan built by hand): ambiguous for dependents.
	duplicate := append(depJobs("v1"), depJobs("v1")[0])

	for name, jobs := range map[string][]JenkinsJob{"unknown": unknown, "cycle": cycle, "duplicate": duplicate} {
		b := &scriptBuilder{}
		for _, o := range runJobs(t, b, 3, jobs) {
			if o.Err == nil || !strings.HasPrefix(o.Err.Error(), "not triggered:") {
				t.Fatalf("%s: %+v", name, o)
			}
		}
		if b.triggers != 0 {
			t.Fatalf("%s: triggered %v", name, b.order)
		}
	}
}

func TestLoadMatrixRejectsBadDependencies(t *testing.T) {
	for name, jobs := range map[string]string{
		"unknown": "  - {name: a, product: k3s, controller: m, path: p, dependsOn: [zzz]}\n",
		"cross-product": "  - {name: a, product: k3s, controller: m, path: p}\n" +
			"  - {name: b, product: rke2, controller: m, path: p, dependsOn: [a]}\n",
		"cycle": "  - {name: a, product: k3s, controller: m, path: p, dependsOn: [b]}\n" +
			"  - {name: b, product: k3s, controller: m, path: p, dependsOn: [a]}\n",
		"duplicate": "  - {name: a, product: k3s, controller: m, path: p}\n" +
			"  - {name: a, product: k3s, controller: m, path: q}\n",
	} {
		path := filepath.Join(t.TempDir(), "m.yaml")
		body := "controllers:\n  m: {url: https://x, maxConcurrent: 1}\njobs:\n" + jobs
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadMatrix(path); err == nil {
			t.Fatalf("%s: expected error", name)
		}
	}
	if _, err := LoadMatrix("../../../config/releasebot/matrix.yaml"); err != nil {
		t.Fatalf("shipped matrix: %v", err)
	}
}

// The real client must tell a confirmed rejection from a trigger that may have been accepted.
func TestJenkinsTriggerErrorClassification(t *testing.T) {
	job := &JenkinsJob{Path: "f/j"}
	cases := map[string]*struct {
		handler func(w http.ResponseWriter, r *http.Request)
		unknown bool
	}{
		"accepted then connection dropped": {func(w http.ResponseWriter, _ *http.Request) {
			hj, _ := w.(http.Hijacker)
			conn, _, _ := hj.Hijack()
			_ = conn.Close()
		}, true},
		"502 from proxy": {func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusBadGateway) }, true},
		"201 without Location": {func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusCreated)
		}, true},
		"400 rejected": {func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusBadRequest) }, false},
	}
	for name, c := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.Contains(r.URL.Path, "crumbIssuer") {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			c.handler(w, r)
		}))
		_, err := NewJenkins(srv.URL, "u", "t").Trigger(context.Background(), job)
		srv.Close()
		if err == nil || errors.Is(err, ErrTriggerUnknown) != c.unknown {
			t.Fatalf("%s: err=%v, want unknown=%v", name, err, c.unknown)
		}
	}

	// Nothing listening: the request never left, so it is a confirmed rejection.
	srv := httptest.NewServer(http.NotFoundHandler())
	addr := srv.URL
	srv.Close()
	if _, err := NewJenkins(addr, "u", "t").Trigger(context.Background(), job); err == nil ||
		errors.Is(err, ErrTriggerUnknown) {
		t.Fatalf("connection refused: %v", err)
	}
}

// Review P1: a redirect after the POST must not be read as "never sent", even when the redirect
// target refuses the connection.
func TestJenkinsTriggerRedirectIsUnknown(t *testing.T) {
	dead := httptest.NewServer(http.NotFoundHandler())
	deadURL := dead.URL
	dead.Close()

	posts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "crumbIssuer") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		posts++
		http.Redirect(w, r, deadURL+"/queue/item/1/", http.StatusFound)
	}))
	defer srv.Close()

	_, err := NewJenkins(srv.URL, "u", "t").Trigger(context.Background(), &JenkinsJob{Path: "f/j"})
	if !errors.Is(err, ErrTriggerUnknown) || posts != 1 {
		t.Fatalf("err=%v posts=%d, want ErrTriggerUnknown after one POST", err, posts)
	}
}

// Review P2: unknown matrix keys (a depends_on typo) are rejected instead of silently dropping deps.
func TestLoadMatrixRejectsUnknownFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "m.yaml")
	body := "controllers:\n  m: {url: https://x, maxConcurrent: 1}\njobs:\n" +
		"  - {name: smoke, product: k3s, controller: m, path: p}\n" +
		"  - {name: conf, product: k3s, controller: m, path: q, depends_on: [smoke]}\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadMatrix(path); err == nil || !strings.Contains(err.Error(), "depends_on") {
		t.Fatalf("expected unknown-field error naming depends_on, got %v", err)
	}
}

// Review P3: cancellation and deadline stay identifiable through the Qase wait error.
func TestWaitQaseRunsKeepsContextError(t *testing.T) {
	title := "RKE2 September 2026 Patch Validation for v1.37.1+rke2r1"
	fq := &fakeQase{calls: map[string]int{}, runs: map[string][][]QaseRun{title: {{}}}}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := WaitQaseRuns(ctx, fq, []string{title}, "rb-1", time.Millisecond, time.Second)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled: %v", err)
	}

	_, err = WaitQaseRuns(context.Background(), fq, []string{title}, "rb-1", time.Millisecond, 5*time.Millisecond)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("deadline: %v", err)
	}
}

// The adapter over the shared qase-go client maps runs (including a null description) and matches them.
func TestQaseAdapterAgainstFakeAPI(t *testing.T) {
	title := "RKE2 September 2026 Patch Validation for v1.37.1+rke2r1"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"status":true,"result":{"entities":[
			{"id":901,"title":%[1]q,"description":null},
			{"id":902,"title":%[1]q,"description":"Version: v1.37.1-rc1 | Release bot request: rb-1"}]}}`, title)
	}))
	defer srv.Close()

	cfg := qaseclient.NewConfiguration()
	cfg.Servers = qaseclient.ServerConfigurations{{URL: srv.URL + "/v1"}}
	ctx := context.WithValue(context.Background(), qaseclient.ContextAPIKeys,
		map[string]qaseclient.APIKey{"TokenAuth": {Key: "tok"}})
	q := newQaseFrom(&qase.Client{QaseAPI: qaseclient.NewAPIClient(cfg), Ctx: ctx})

	ids, err := WaitQaseRuns(context.Background(), q, []string{title}, "rb-1", time.Millisecond, time.Second)
	if err != nil || ids[title] != 902 {
		t.Fatalf("ids=%v err=%v", ids, err)
	}
}

func TestNewQaseRequiresToken(t *testing.T) {
	t.Setenv("QASE_AUTOMATION_TOKEN", "")
	if _, err := NewQase(); err == nil {
		t.Fatal("expected error without QASE_AUTOMATION_TOKEN")
	}
}

func TestGitHubTagExists(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.EscapedPath() {
		case "/repos/rancher/rke2/git/ref/tags/v1.37.1-rc1+rke2r1":
			w.WriteHeader(http.StatusOK)
		case "/repos/rancher/rke2/git/ref/tags/v9.9.9-rc1+rke2r1":
			w.WriteHeader(http.StatusNotFound)
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()
	gh := &GitHub{BaseURL: srv.URL, HTTP: srv.Client()}

	if ok, err := gh.TagExists(context.Background(), "rke2", "v1.37.1-rc1+rke2r1"); !ok || err != nil {
		t.Fatalf("existing tag: %v %v", ok, err)
	}
	if ok, err := gh.TagExists(context.Background(), "rke2", "v9.9.9-rc1+rke2r1"); ok || err != nil {
		t.Fatalf("missing tag: %v %v", ok, err)
	}
	if _, err := gh.TagExists(context.Background(), "k3s", "v1.37.1-rc1+k3s1"); err == nil {
		t.Fatal("500 must be an error, not a missing tag")
	}
	if _, err := gh.TagExists(context.Background(), "nope", "v1"); err == nil {
		t.Fatal("unknown product must be an error")
	}
}

func TestGitHubDispatch(t *testing.T) {
	var got struct {
		Ref    string            `json:"ref"`
		Inputs map[string]string `json:"inputs"`
	}
	status := http.StatusNoContent
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/repos/o/r/actions/workflows/wf.yaml/dispatches" ||
			r.Header.Get("Authorization") != "Bearer tok" {
			t.Errorf("unexpected request %s %s auth=%q", r.Method, r.URL.Path, r.Header.Get("Authorization"))
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"message":"Unexpected inputs provided: [\"request_id\"]"}`))
	}))
	defer srv.Close()

	w := WorkflowDispatch{
		Repo: "o/r", Workflow: "wf.yaml", Ref: "main",
		Inputs: map[string]string{"rcs": "v1.37.1-rc1", "request_id": "rb-1", "empty": ""},
	}
	gh := &GitHub{BaseURL: srv.URL, Token: "tok", HTTP: srv.Client()}

	if err := gh.Dispatch(context.Background(), w); err != nil {
		t.Fatal(err)
	}
	if got.Ref != "main" || got.Inputs["rcs"] != "v1.37.1-rc1" || got.Inputs["request_id"] != "rb-1" {
		t.Fatalf("body = %+v", got)
	}
	if _, sent := got.Inputs["empty"]; sent {
		t.Fatal("empty inputs must be omitted")
	}

	// A workflow without the input answers 422: the error must carry GitHub's message.
	status = http.StatusUnprocessableEntity
	if err := gh.Dispatch(context.Background(), w); err == nil || !strings.Contains(err.Error(), "Unexpected inputs") {
		t.Fatalf("422: %v", err)
	}

	if err := (&GitHub{BaseURL: srv.URL, HTTP: srv.Client()}).Dispatch(context.Background(), w); err == nil {
		t.Fatal("dispatch without a token must fail")
	}
}
