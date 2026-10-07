package releasebot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rancher/distros-test-framework/internal/pkg/slack"
)

const (
	testChan = "C1"
	openChan = "COPEN"
	testBot  = "UBOT"
	alice    = "UALICE" // allowed
	mallory  = "UMAL"   // not allowed
)

type fakeSlack struct {
	mu    sync.Mutex
	posts []string // "<thread>|<text>"
}

func (f *fakeSlack) post(_ context.Context, _, thread, text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.posts = append(f.posts, thread+"|"+text)

	return nil
}

func (f *fakeSlack) all() string {
	f.mu.Lock()
	defer f.mu.Unlock()

	return strings.Join(f.posts, "\n")
}

type testListener struct {
	*Listener
	t     *testing.T
	slack *fakeSlack
	runs  *atomic.Int32
	gate  chan struct{} // when set, runs block until closed (or until stopped)
	errs  []error       // returned by the runs in order; nil when exhausted
}

func newTestListener(t *testing.T, planErr error) *testListener {
	t.Helper()
	tl := &testListener{t: t, slack: &fakeSlack{}, runs: &atomic.Int32{}}
	tl.Listener = &Listener{
		Channels:     map[string]bool{testChan: true, openChan: true},
		OpenChannels: map[string]bool{openChan: true},
		BotUserID:    testBot,
		Allowed:      map[string]bool{alice: true},
		Post:         tl.slack.post,
		Plan: func(_ context.Context, text string) (*Planned, error) {
			if planErr != nil {
				return nil, planErr
			}
			req := ParseRequest(text)

			return tl.planned(append(req.K3s, req.RKE2...)), nil
		},
	}

	return tl
}

func (tl *testListener) planned(tags []string) *Planned {
	return &Planned{
		Summary: "PLAN for " + strings.Join(tags, " "),
		Tags:    tags,
		Run:     tl.run,
		Without: func(_ context.Context, drop []string) (*Planned, error) {
			var keep []string
			for _, t := range tags {
				if !slices.Contains(drop, t) {
					keep = append(keep, t)
				}
			}

			return tl.planned(keep), nil
		},
	}
}

func (tl *testListener) run(ctx context.Context, rc *RunControl) error {
	n := tl.runs.Add(1)
	rc.SetStatus(func() string { return "STATUS run " + strconv.Itoa(int(n)) })
	if tl.gate != nil {
		select {
		case <-tl.gate:
		case <-rc.Stop:
			rc.Notify("stopping")
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
			return errors.New("run neither released nor stopped")
		}
	}
	rc.Notify("triggered job-1")
	if int(n) <= len(tl.errs) {
		return tl.errs[n-1]
	}

	return nil
}

func (tl *testListener) Wait() {
	tl.t.Helper()
	done := make(chan struct{})
	go func() {
		tl.Listener.Wait()
		close(done)
	}()
	receive(tl.t, done)
}

func (tl *testListener) waitRuns(t *testing.T, n int32) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for tl.runs.Load() < n {
		if time.Now().After(deadline) {
			t.Fatalf("only %d of %d runs started", tl.runs.Load(), n)
		}
		time.Sleep(time.Millisecond)
	}
}

func mentionIn(channel, user, ts, tags string) *slack.Event {
	return &slack.Event{Type: "message", Channel: channel, User: user, TS: ts, Text: "<@" + testBot + "> " + tags}
}

func mention(user, ts string) *slack.Event {
	return mentionIn(testChan, user, ts, "v1.37.1-rc2+rke2r1")
}

func reply(user, thread, text string) *slack.Event {
	return &slack.Event{Type: "message", Channel: testChan, User: user, TS: thread + "1", ThreadTS: thread, Text: text}
}

// A mention with RC tags from an allowed user starts at once: no confirmation step.
func TestListenerStartsWithoutConfirm(t *testing.T) {
	tl := newTestListener(t, nil)
	tl.Handle(testContext(t), mention(alice, "100.1"))
	tl.Wait()

	got := tl.slack.all()
	if tl.runs.Load() != 1 || !strings.Contains(got, "100.1|PLAN for v1.37.1-rc2+rke2r1") ||
		!strings.Contains(got, "Started by <@"+alice+">: phase 1 is starting now") ||
		!strings.Contains(got, "100.1|triggered job-1") || !strings.Contains(got, "Release plan finished.") {
		t.Fatalf("runs=%d posts:\n%s", tl.runs.Load(), got)
	}
}

// In an open channel anyone may start; elsewhere others only get the plan.
func TestListenerAllowlistAndOpenChannels(t *testing.T) {
	tl := newTestListener(t, nil)
	ctx := testContext(t)

	tl.Handle(ctx, mention(mallory, "300.1"))
	tl.Wait()
	if tl.runs.Load() != 0 || !strings.Contains(tl.slack.all(), "300.1|PLAN") ||
		!strings.Contains(tl.slack.all(), "Dry-run only: <@"+mallory+"> is not on the list") {
		t.Fatalf("mallory started a run in a closed channel:\n%s", tl.slack.all())
	}

	tl.Handle(ctx, mentionIn(openChan, mallory, "301.1", "v1.37.1-rc2+k3s1"))
	tl.Wait()
	if tl.runs.Load() != 1 || !strings.Contains(tl.slack.all(), "301.1|PLAN for v1.37.1-rc2+k3s1") {
		t.Fatalf("open channel run did not start (runs=%d):\n%s", tl.runs.Load(), tl.slack.all())
	}
}

func TestListenerIgnoresNoise(t *testing.T) {
	tl := newTestListener(t, nil)
	ctx := testContext(t)
	for _, e := range []*slack.Event{
		{Type: "message", Channel: "COTHER", User: alice, TS: "1", Text: "<@" + testBot + "> v1.37.1-rc2+rke2r1"},
		{Type: "message", Channel: testChan, BotID: "B1", TS: "2", Text: "<@" + testBot + "> report"},
		{Type: "message", Channel: testChan, User: testBot, TS: "3", Text: "<@" + testBot + ">"},
		{
			Type: "message", Subtype: "message_changed", Channel: testChan, User: alice, TS: "4",
			Text: "<@" + testBot + ">",
		},
		{Type: "message", Channel: testChan, User: alice, TS: "5", Text: "no mention here"},
		reply(alice, "999.1", "stop"),   // thread without a run
		reply(alice, "999.1", "status"), // thread without a run
	} {
		tl.Handle(ctx, e)
	}
	if got := tl.slack.all(); got != "" {
		t.Fatalf("expected no replies, got:\n%s", got)
	}
}

// RC tags already being validated are refused with a pointer to that run; other tags run alongside.
func TestListenerRefusesDuplicateTags(t *testing.T) {
	tl := newTestListener(t, nil)
	tl.gate = make(chan struct{})
	tl.Now = func() time.Time { return time.Date(2026, 9, 29, 15, 4, 0, 0, time.UTC) }
	ctx := testContext(t)

	tl.Handle(ctx, mentionIn(testChan, alice, "500.1", "v1.37.1-rc2+rke2r1 v1.36.5-rc2+rke2r1"))
	tl.waitRuns(t, 1)
	tl.Handle(ctx, mentionIn(testChan, alice, "501.1", "v1.36.5-rc2+rke2r1"))
	tl.Handle(ctx, mentionIn(testChan, alice, "502.1", "v1.37.1-rc2+k3s1"))
	tl.waitRuns(t, 2)
	close(tl.gate)
	tl.Wait()

	got := tl.slack.all()
	if tl.runs.Load() != 2 || !strings.Contains(got, "501.1|PLAN for v1.36.5-rc2+rke2r1\n\nNot started: "+
		"v1.36.5-rc2+rke2r1 already running in the <#"+testChan+"> thread started by <@"+alice+"> at 15:04 UTC") {
		t.Fatalf("runs=%d posts:\n%s", tl.runs.Load(), got)
	}
}

// A message with a tag already running and a new one starts the new one alone, saying what was
// left out; the plan is rebuilt without the duplicate.
func TestListenerRunsNewTagsNextToDuplicates(t *testing.T) {
	tl := newTestListener(t, nil)
	tl.gate = make(chan struct{})
	tl.Now = func() time.Time { return time.Date(2026, 9, 29, 15, 4, 0, 0, time.UTC) }
	ctx := testContext(t)

	tl.Handle(ctx, mentionIn(testChan, alice, "550.1", "v1.37.1-rc2+rke2r1"))
	tl.waitRuns(t, 1)
	tl.Handle(ctx, mentionIn(testChan, alice, "551.1", "v1.37.1-rc2+rke2r1 v1.36.5-rc2+rke2r1"))
	tl.waitRuns(t, 2)
	close(tl.gate)
	tl.Wait()

	got := tl.slack.all()
	want := "551.1|PLAN for v1.36.5-rc2+rke2r1\n\nLeft out: v1.37.1-rc2+rke2r1 already running in the <#" +
		testChan + "> thread started by <@" + alice + "> at 15:04 UTC.\nStarted by <@" + alice + ">"
	if !strings.Contains(got, want) {
		t.Fatalf("posts:\n%s", got)
	}
}

// Duplicates are removed before a refusal is checked: only the plan with both tags is refused here
// (the running tag has its own blocker), and the new tag still starts.
func TestListenerRefusalOfDuplicateDoesNotBlockNewTags(t *testing.T) {
	tl := newTestListener(t, nil)
	tl.gate = make(chan struct{})
	plan := tl.Plan
	tl.Plan = func(ctx context.Context, text string) (*Planned, error) {
		p, err := plan(ctx, text)
		if len(p.Tags) > 1 {
			p.Refusal = "1 required jobs cannot run (v1.37.1-rc2+rke2r1: upgrade start version unknown)"
		}

		return p, err
	}
	ctx := testContext(t)
	tl.Handle(ctx, mentionIn(testChan, alice, "560.1", "v1.37.1-rc2+rke2r1"))
	tl.waitRuns(t, 1)
	tl.Handle(ctx, mentionIn(testChan, alice, "561.1", "v1.37.1-rc2+rke2r1 v1.36.5-rc2+rke2r1"))
	tl.waitRuns(t, 2)
	close(tl.gate)
	tl.Wait()

	got := tl.slack.all()
	if !strings.Contains(got, "561.1|PLAN for v1.36.5-rc2+rke2r1\n\nLeft out: v1.37.1-rc2+rke2r1") ||
		strings.Contains(got, "Not started") {
		t.Fatalf("posts:\n%s", got)
	}
}

// `status` answers from the run's snapshot; `stop` from someone allowed stops starting new jobs.
func TestListenerStopAndStatus(t *testing.T) {
	tl := newTestListener(t, nil)
	tl.gate = make(chan struct{})
	ctx := testContext(t)

	tl.Handle(ctx, mention(alice, "600.1"))
	tl.waitRuns(t, 1)
	tl.Handle(ctx, reply(mallory, "600.1", "status"))
	tl.Handle(ctx, reply(mallory, "600.1", "stop"))
	tl.Handle(ctx, reply(alice, "600.1", "Stop"))
	tl.Handle(ctx, reply(alice, "600.1", "stop")) // already stopped: no second message
	tl.Wait()

	got := tl.slack.all()
	for _, want := range []string{
		"600.1|STATUS run 1", "600.1|<@" + mallory + "> is not allowed to stop",
		"600.1|Stopped by <@" + alice + ">", "600.1|stopping",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Count(got, "Stopped by") != 1 {
		t.Fatalf("stop answered twice:\n%s", got)
	}
}

// `retry`/`skip` reach the run's scheduler and its answer is posted; help requests mention the
// person who started the run.
func TestListenerRetrySkipAndHelp(t *testing.T) {
	tl := newTestListener(t, nil)
	got := make(chan Command, 1)
	tl.Plan = func(context.Context, string) (*Planned, error) {
		run := func(ctx context.Context, rc *RunControl) error {
			tl.runs.Add(1)
			rc.Help("Needs help: smoke v1 ended with FAILURE.")
			var c Command
			select {
			case c = <-rc.Commands:
			case <-ctx.Done():
				return ctx.Err()
			}
			c.Reply <- "Retrying " + c.Job + " " + c.Version + ", asked by <@" + c.By + ">."
			got <- c

			return nil
		}

		return &Planned{Summary: "PLAN", Tags: []string{"v1.37.1-rc2+rke2r1"}, Run: run}, nil
	}
	ctx := testContext(t)
	tl.Handle(ctx, mention(alice, "1500.1"))
	tl.waitRuns(t, 1)
	tl.Handle(ctx, reply(mallory, "1500.1", "retry smoke v1"))
	tl.Handle(ctx, reply(alice, "1500.1", "retry"))
	tl.Handle(ctx, reply(alice, "1500.1", "Retry smoke v1"))
	tl.Wait()

	c := receive(t, got)
	if c.Action != CommandRetry || c.Job != "smoke" || c.Version != "v1" || c.By != alice {
		t.Fatalf("command %+v", c)
	}
	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(tl.slack.all(), "Retrying smoke v1") && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	all := tl.slack.all()
	for _, want := range []string{
		"1500.1|<@" + alice + "> Needs help: smoke v1 ended with FAILURE.",
		"<@" + mallory + "> is not allowed to retry, skip or triage jobs.",
		"Usage: `retry <job> [rc]`",
		"1500.1|Retrying smoke v1, asked by <@" + alice + ">.",
	} {
		if !strings.Contains(all, want) {
			t.Fatalf("missing %q in:\n%s", want, all)
		}
	}
}

// A stop sent with "Also send to channel" (thread_broadcast) counts.
func TestListenerAcceptsThreadBroadcast(t *testing.T) {
	tl := newTestListener(t, nil)
	tl.gate = make(chan struct{})
	ctx := testContext(t)
	tl.Handle(ctx, mention(alice, "1300.1"))
	tl.waitRuns(t, 1)
	r := reply(alice, "1300.1", "stop")
	r.Subtype = "thread_broadcast"
	tl.Handle(ctx, r)
	tl.Wait()
	if !strings.Contains(tl.slack.all(), "Stopped by") {
		t.Fatalf("thread_broadcast stop ignored:\n%s", tl.slack.all())
	}
}

// A plan that cannot run (e.g. a required job the controller lacks) is shown but not started.
func TestListenerRefusesUnrunnablePlan(t *testing.T) {
	tl := newTestListener(t, nil)
	plan := tl.Plan
	tl.Plan = func(ctx context.Context, text string) (*Planned, error) {
		p, err := plan(ctx, text)
		p.Refusal = "1 required jobs cannot run"

		return p, err
	}
	tl.Handle(testContext(t), mention(alice, "660.1"))
	tl.Wait()
	if tl.runs.Load() != 0 || !strings.Contains(tl.slack.all(), "660.1|PLAN for v1.37.1-rc2+rke2r1\n\n"+
		"Not started: 1 required jobs cannot run") {
		t.Fatalf("runs=%d posts:\n%s", tl.runs.Load(), tl.slack.all())
	}
}

func TestListenerPlanError(t *testing.T) {
	tl := newTestListener(t, errors.New("tags not found on gitHub: v9.9.9-rc1+rke2r1"))
	tl.Handle(testContext(t), mention(alice, "650.1"))
	tl.Wait()
	if !strings.Contains(tl.slack.all(), "650.1|Could not build a release plan: tags not found") {
		t.Fatalf("posts:\n%s", tl.slack.all())
	}
}

// The last progress lines and the outcome are posted even after the run's ctx is canceled.
func TestListenerPostsOutcomeAfterShutdown(t *testing.T) {
	tl := newTestListener(t, nil)
	var mu sync.Mutex
	var posts []string
	tl.Post = func(ctx context.Context, _, _, text string) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		mu.Lock()
		posts = append(posts, text)
		mu.Unlock()

		return nil
	}
	ctx, cancel := context.WithCancel(testContext(t))
	tl.Plan = func(context.Context, string) (*Planned, error) {
		return &Planned{Summary: "PLAN", Run: func(runCtx context.Context, rc *RunControl) error {
			cancel() // SIGTERM while running
			<-runCtx.Done()
			rc.Notify("stopping: 2 builds still on jenkins")

			return errors.Join(fmt.Errorf("%w: job-a v1", errUnreconciledBuilds), runCtx.Err())
		}}, nil
	}
	tl.Handle(ctx, mention(alice, "900.1"))
	tl.Wait()

	all := strings.Join(posts, "\n")
	if !strings.Contains(all, "stopping: 2 builds still on jenkins") ||
		!strings.Contains(all, "this run is saved and resumes here") {
		t.Fatalf("posts after shutdown were lost:\n%s", all)
	}
}

// In dry-run mode even an allowed user only gets the plan.
func TestListenerDryRunNeverExecutes(t *testing.T) {
	tl := newTestListener(t, nil)
	tl.DryRun = true
	tl.Handle(testContext(t), mention(alice, "1000.1"))
	tl.Wait()
	if tl.runs.Load() != 0 || !strings.Contains(tl.slack.all(), "started without `-dry-run=false`") {
		t.Fatalf("runs=%d posts:\n%s", tl.runs.Load(), tl.slack.all())
	}
}

// Each post has its own timeout: a run longer than it still gets its progress and outcome posted.
func TestListenerPostsAfterLongRun(t *testing.T) {
	tl := newTestListener(t, nil)
	tl.PostTimeout = 50 * time.Millisecond
	tl.ProgressInterval = time.Hour
	var mu sync.Mutex
	var posts []string
	tl.Post = func(ctx context.Context, _, _, text string) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		mu.Lock()
		posts = append(posts, text)
		mu.Unlock()

		return nil
	}
	tl.Plan = func(context.Context, string) (*Planned, error) {
		return &Planned{Summary: "PLAN", Run: func(_ context.Context, rc *RunControl) error {
			time.Sleep(200 * time.Millisecond) // longer than PostTimeout
			rc.Notify("finished job-1")

			return nil
		}}, nil
	}
	tl.Handle(testContext(t), mention(alice, "1400.1"))
	tl.Wait()

	all := strings.Join(posts, "\n")
	if !strings.Contains(all, "finished job-1") || !strings.Contains(all, "Release plan finished.") {
		t.Fatalf("posts after a long run were lost:\n%s", all)
	}
}

// Progress lines are grouped into one message per interval (Slack allows ~1 message/s/channel).
func TestProgressIsBatched(t *testing.T) {
	var mu sync.Mutex
	var posts []string
	b := newProgressBatcher(func(s string) {
		mu.Lock()
		posts = append(posts, s)
		mu.Unlock()
	}, time.Hour)
	for i := range 5 {
		b.add(fmt.Sprintf("line %d", i))
	}
	b.close()
	if len(posts) != 1 || posts[0] != "line 0\nline 1\nline 2\nline 3\nline 4" {
		t.Fatalf("posts = %q", posts)
	}
}

// Add never waits on Slack, and batches keep their order.
func TestProgressBatcherNeverBlocksCaller(t *testing.T) {
	release := make(chan struct{})
	var mu sync.Mutex
	var batches []string
	b := newProgressBatcher(func(s string) {
		<-release // Slack is slow (or rate limited)
		mu.Lock()
		batches = append(batches, s)
		mu.Unlock()
	}, time.Hour)

	done := make(chan struct{})
	go func() {
		for i := range 3 * maxBatchLines {
			b.add(strconv.Itoa(i))
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("add blocked on a slow post")
	}
	close(release)
	b.close()

	var got []string
	for _, batch := range batches {
		got = append(got, strings.Split(batch, "\n")...)
	}
	for i, v := range got {
		if v != strconv.Itoa(i) {
			t.Fatalf("progress out of order at %d: %v", i, got)
		}
	}
	if len(got) != 3*maxBatchLines {
		t.Fatalf("lost lines: %d", len(got))
	}
}

func TestInstanceLockAllowsOneBot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "state.json.lock")
	release, err := AcquireInstanceLock(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = AcquireInstanceLock(path); !errors.Is(err, errAlreadyRunning) {
		t.Fatalf("second instance: %v", err)
	}
	release()
	release2, err := AcquireInstanceLock(path)
	if err != nil {
		t.Fatalf("lock not released: %v", err)
	}
	release2()
}

func (tl *testListener) waitPost(t *testing.T, want string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(tl.slack.all(), want) {
		if time.Now().After(deadline) {
			t.Fatalf("no post %q:\n%s", want, tl.slack.all())
		}
		time.Sleep(time.Millisecond)
	}
}

// A plan stuck on a slow jenkins does not hold the event loop: `stop` and `status` in a running
// thread are answered while it is being built.
func TestListenerControlWhilePlanning(t *testing.T) {
	tl := newTestListener(t, nil)
	tl.gate = make(chan struct{})
	ctx := testContext(t)
	tl.Handle(ctx, mention(alice, "900.1"))
	tl.waitRuns(t, 1)

	slow := make(chan struct{})
	plan := tl.Plan
	tl.Plan = func(ctx context.Context, text string) (*Planned, error) {
		select {
		case <-slow:
		case <-ctx.Done():
			return nil, ctx.Err()
		}

		return plan(ctx, text)
	}
	tl.Handle(ctx, mention(alice, "901.1"))
	tl.Handle(ctx, reply(alice, "900.1", "stop"))
	tl.waitPost(t, "900.1|Stopped by")
	close(slow)
	close(tl.gate)
	tl.Wait()
}

// Planning is bounded: the planner gets a context that ends after PlanTimeout.
func TestListenerPlanTimeout(t *testing.T) {
	tl := newTestListener(t, nil)
	tl.PlanTimeout = 20 * time.Millisecond
	tl.Plan = func(ctx context.Context, _ string) (*Planned, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	tl.Handle(testContext(t), mention(alice, "910.1"))
	tl.Wait()
	if !strings.Contains(tl.slack.all(), "910.1|Could not build a release plan: context deadline exceeded") {
		t.Fatalf("posts:\n%s", tl.slack.all())
	}
}

// Planning work is bounded: beyond the workers and the queue, a mention is answered "busy" at once
// instead of waiting in an ever-growing backlog.
func TestListenerPlanQueueIsBounded(t *testing.T) {
	tl := newTestListener(t, nil)
	tl.MaxPlanning, tl.QueuedPlans = 1, 1
	release := make(chan struct{})
	entered := make(chan struct{}, 2)
	var planned atomic.Int32
	plan := tl.Plan
	tl.Plan = func(ctx context.Context, text string) (*Planned, error) {
		planned.Add(1)
		entered <- struct{}{}
		select {
		case <-release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}

		return plan(ctx, text)
	}
	ctx := testContext(t)
	tl.Handle(ctx, mention(mallory, "920.1")) // not allowed: plan only, no run
	receive(t, entered)
	tl.Handle(ctx, mention(mallory, "921.1")) // queued
	tl.Handle(ctx, mention(mallory, "922.1")) // over the bound
	tl.waitPost(t, "922.1|The bot is busy")
	close(release)
	tl.Wait()
	if n := planned.Load(); n != 2 || strings.Contains(tl.slack.all(), "921.1|The bot is busy") {
		t.Fatalf("planned %d, posts:\n%s", n, tl.slack.all())
	}
}

// A run that ends with builds in unknown state, or part-way, is reported in its thread and blocks
// nothing: the next plan starts.
func TestListenerDoesNotBlockAfterBadEnds(t *testing.T) {
	tl := newTestListener(t, nil)
	tl.errs = []error{
		fmt.Errorf("1 of 2 jobs did not succeed; %w: job-a v1", errUnreconciledBuilds),
		fmt.Errorf("%w: dispatch b.yaml: 502", errPartialRun),
	}
	ctx := testContext(t)
	for _, ts := range []string{"700.1", "701.1", "702.1"} {
		tl.Handle(ctx, mention(alice, ts))
		tl.Wait()
	}
	got := tl.slack.all()
	if tl.runs.Load() != 3 || !strings.Contains(got, "700.1|Release plan finished; some builds stayed in unknown") ||
		!strings.Contains(got, "701.1|Release plan stopped part-way") || strings.Contains(got, "blocked") {
		t.Fatalf("runs=%d posts:\n%s", tl.runs.Load(), got)
	}
}

// A run's progress is saved at each change; after a restart (the old process killed) it resumes in
// its own thread from what was saved, takes `stop` there, and is removed from the state once over.
func TestListenerResumesSavedRuns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	tl := newTestListener(t, nil)
	tl.StatePath = path
	saved := Progress{RequestID: "rb-1", Stage: StageJobs, Jobs: []JobRecord{
		{Job: JenkinsJob{Name: "smoke", Version: "v1"}, State: jobRunning, BuildURL: "b/1"},
	}}
	tl.Plan = func(context.Context, string) (*Planned, error) {
		killedWhileRunning := func(ctx context.Context, rc *RunControl) error {
			_ = rc.Save(saved)
			<-ctx.Done() // the process is killed while the build runs

			return ctx.Err()
		}

		return &Planned{Summary: "PLAN", Tags: []string{"v1.37.1-rc2+rke2r1"}, Run: killedWhileRunning}, nil
	}
	ctx, kill := context.WithCancel(testContext(t))
	tl.Handle(ctx, mention(alice, "800.1"))
	waitFile(t, path, `"build_url":"b/1"`)
	kill()
	tl.Wait()

	next := newTestListener(t, nil)
	next.StatePath = path
	resumed := make(chan Progress, 1)
	next.Resume = func(_ context.Context, rc *RunControl, p Progress) error {
		rc.Ready() // holds no slot: the other resumed runs (none here) may go
		<-rc.Go
		resumed <- p
		<-rc.Stop

		return nil
	}
	if err := next.Restore(); err != nil || next.SavedRuns() != 1 {
		t.Fatalf("restore: %v, saved %d", err, next.SavedRuns())
	}
	ctx2 := testContext(t)
	if n := next.ResumeRuns(ctx2); n != 1 {
		t.Fatalf("resumed %d", n)
	}
	if p := receive(t, resumed); p.RequestID != "rb-1" || len(p.Jobs) != 1 || p.Jobs[0].BuildURL != "b/1" {
		t.Fatalf("resumed from %+v", p)
	}
	next.Handle(ctx2, mention(alice, "801.1")) // same tag: the resumed run still holds it
	next.waitPost(t, "801.1|")
	next.Handle(ctx2, reply(alice, "800.1", "stop"))
	next.Wait()
	got := next.slack.all()
	if !strings.Contains(got, "800.1|The bot restarted; resuming this run") ||
		!strings.Contains(got, "801.1|PLAN for v1.37.1-rc2+rke2r1\n\nNot started: v1.37.1-rc2+rke2r1 already running") ||
		!strings.Contains(got, "800.1|Release plan finished.") || next.SavedRuns() != 0 {
		t.Fatalf("saved %d, posts:\n%s", next.SavedRuns(), got)
	}
	if raw, _ := os.ReadFile(path); strings.Contains(string(raw), "800.1") {
		t.Fatalf("finished run still saved: %s", raw)
	}
}

// A run saved before its workflows were all dispatched is not resumed (what started is unknown):
// its thread says so and it leaves the state.
func TestListenerDropsUnresumableRuns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	state := listenerState{Runs: map[string]*savedRun{
		threadKey(testChan, "810.1"): {
			Channel: testChan, TS: "810.1", User: alice, Progress: Progress{Stage: StageDispatching},
		},
	}}
	raw, _ := json.Marshal(state)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	tl := newTestListener(t, nil)
	tl.StatePath = path
	tl.Resume = func(context.Context, *RunControl, Progress) error {
		t.Fatal("an unresumable run was resumed")
		return nil
	}
	if err := tl.Restore(); err != nil {
		t.Fatal(err)
	}
	if n := tl.ResumeRuns(testContext(t)); n != 0 || tl.SavedRuns() != 0 ||
		!strings.Contains(tl.slack.all(), "810.1|<@"+alice+"> The bot restarted before this run had dispatched all") {
		t.Fatalf("resumed %d, saved %d, posts:\n%s", n, tl.SavedRuns(), tl.slack.all())
	}
}

func waitFile(t *testing.T, path, want string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if raw, _ := os.ReadFile(path); strings.Contains(string(raw), want) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s never contained %q", path, want)
		}
		time.Sleep(time.Millisecond)
	}
}

// Resumed runs wait for each other: a run with only pending jobs triggers nothing until the run whose
// build is still on jenkins has taken its slot back, so together they stay within the limit.
func TestResumedRunsTakeTheirSlotsFirst(t *testing.T) {
	smoke := JenkinsJob{Name: "smoke", Product: "rke2", Version: "v1", Controller: "mower", Path: "smoke"}
	busy := JenkinsJob{Name: "busy", Product: "rke2", Version: "v2", Controller: "mower", Path: "busy"}
	b := &resumeBuilder{triggers: map[string]int{}, hold: map[string]chan struct{}{"busy": make(chan struct{})}}
	capacity := &Capacity{}
	tl := newTestListener(t, nil)
	tl.saved = map[string]*savedRun{
		threadKey(testChan, "1"): {
			Channel: testChan, TS: "1", User: alice,
			Progress: Progress{Stage: StageJobs, Jobs: []JobRecord{{Job: smoke, State: jobPending}}},
		},
		threadKey(testChan, "2"): {
			Channel: testChan, TS: "2", User: alice,
			Progress: Progress{Stage: StageJobs, Jobs: []JobRecord{{Job: busy, State: jobRunning, BuildURL: "b/busy"}}},
		},
	}
	post := tl.Post
	tl.Post = func(ctx context.Context, channel, thread, text string) error {
		if thread == "2" && strings.HasPrefix(text, "The bot restarted;") {
			time.Sleep(100 * time.Millisecond) // the run holding a build starts late
		}

		return post(ctx, channel, thread, text)
	}
	tl.Resume = func(ctx context.Context, rc *RunControl, p Progress) error {
		s := &Scheduler{
			Builders: map[string]Builder{"mower": b}, Limits: map[string]Limits{"mower": {MaxConcurrent: 1}},
			Poll: time.Millisecond, Capacity: capacity, Owner: rc.ID, Resume: p.Jobs, Ready: rc.Ready, Go: rc.Go,
		}
		s.Run(ctx, nil)

		return nil
	}
	if n := tl.ResumeRuns(testContext(t)); n != 2 {
		t.Fatalf("resumed %d", n)
	}
	time.Sleep(50 * time.Millisecond) // the pending run had every chance to trigger
	if b.count("smoke") != 0 || capacity.InUse("mower") != 1 {
		t.Fatalf("smoke triggered %d times past the limit, slots %d", b.count("smoke"), capacity.InUse("mower"))
	}
	close(b.hold["busy"])
	tl.Wait()
	if b.count("smoke") != 1 {
		t.Fatalf("smoke triggered %d times once the slot was free", b.count("smoke"))
	}
}

// `stop` is in the saved state at once, so a run stopped just before a restart resumes stopped.
func TestStopIsSaved(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	tl := newTestListener(t, nil)
	tl.StatePath = path
	tl.gate = make(chan struct{})
	ctx := testContext(t)
	tl.Handle(ctx, mention(alice, "820.1"))
	tl.waitRuns(t, 1)
	tl.Handle(ctx, reply(alice, "820.1", "stop"))
	raw, _ := os.ReadFile(path)
	if !strings.Contains(string(raw), `"stopped":true`) {
		t.Fatalf("stop not saved: %s", raw)
	}
	tl.Wait()
}

// A run's save reports a failure (the scheduler then triggers nothing), and the state file carries
// a version: a newer one is refused instead of misread.
func TestStateSaveErrorsAndVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	l := &Listener{StatePath: path}
	run := l.newRunLocked("C", "1", "U", []string{"v1"}, time.Now())
	if err := run.ctl.Save(Progress{Stage: StageJobs}); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path+".tmp", 0o700); err != nil { // the next write cannot replace the file
		t.Fatal(err)
	}
	if err := run.ctl.Save(Progress{Stage: StageJobs, RequestID: "x"}); err == nil {
		t.Fatal("a failed save was not reported")
	}

	newer, _ := json.Marshal(map[string]any{"version": stateVersion + 1})
	if err := os.WriteFile(path, newer, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := (&Listener{StatePath: path}).Restore(); err == nil {
		t.Fatal("a state of a newer format was read")
	}
}

// Without a Resume the saved runs are kept for a bot that can resume them, and nobody is told
// anything wrong.
func TestNoResumeKeepsSavedRuns(t *testing.T) {
	tl := newTestListener(t, nil)
	tl.saved = map[string]*savedRun{threadKey(testChan, "1"): {
		Channel: testChan, TS: "1",
		Progress: Progress{Stage: StageJobs},
	}}
	if n := tl.ResumeRuns(testContext(t)); n != 0 || tl.SavedRuns() != 1 || tl.slack.all() != "" {
		t.Fatalf("resumed %d, saved %d, posts %q", n, tl.SavedRuns(), tl.slack.all())
	}
}

// A run that ended on its own just as the bot stopped is over: it is not saved to be resumed.
func TestRunEndingAtShutdownIsNotSaved(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	tl := newTestListener(t, nil)
	tl.StatePath = path
	ctx, stop := context.WithCancel(testContext(t))
	tl.Plan = func(context.Context, string) (*Planned, error) {
		return &Planned{Summary: "PLAN", Run: func(context.Context, *RunControl) error {
			stop() // SIGTERM arrives as the run finishes by itself

			return nil
		}}, nil
	}
	tl.Handle(ctx, mention(alice, "830.1"))
	tl.Wait()
	if tl.SavedRuns() != 0 || !strings.Contains(tl.slack.all(), "830.1|Release plan finished.") {
		t.Fatalf("saved %d, posts:\n%s", tl.SavedRuns(), tl.slack.all())
	}
}

// The saved state is a copy: a run changing its own job parameters (Qase run ids) while another run
// saves the whole state is no data race (run with -race).
func TestSavedStateIsACopy(t *testing.T) {
	l := &Listener{StatePath: filepath.Join(t.TempDir(), "state.json")}
	a := l.newRunLocked("C", "1", "U", []string{"v1"}, time.Now())
	b := l.newRunLocked("C", "2", "U", []string{"v2"}, time.Now())
	params := map[string]string{"QASE_RUN_ID": "{{QASE_RUN_ID}}"}
	progress := Progress{Stage: StageDispatched, Jobs: []JobRecord{{Job: JenkinsJob{Params: params}}}}
	if err := a.ctl.Save(progress); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := range 200 {
			params["QASE_RUN_ID"] = strconv.Itoa(i)
		}
	}()
	for range 200 {
		_ = b.ctl.Save(Progress{Stage: StageJobs})
	}
	<-done
}

// A resume that could not be set up keeps the run saved and its tags taken (its builds may run).
func TestFailedResumeKeepsTheRun(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	tl := newTestListener(t, nil)
	tl.StatePath = path
	tl.saved = map[string]*savedRun{threadKey(testChan, "840.1"): {
		Channel: testChan, TS: "840.1", User: alice,
		Tags: []string{"v1.37.1-rc2+rke2r1"}, Progress: Progress{Stage: StageJobs},
	}}
	tl.Resume = func(_ context.Context, rc *RunControl, _ Progress) error {
		rc.Ready()
		return fmt.Errorf("%w: set JENKINS_MOWER_AUTH=user:apitoken", errResumeFailed)
	}
	ctx := testContext(t)
	tl.ResumeRuns(ctx)
	tl.Wait()
	tl.Handle(ctx, mention(alice, "841.1"))
	tl.Wait()
	raw, _ := os.ReadFile(path)
	got := tl.slack.all()
	if tl.SavedRuns() != 1 || !strings.Contains(string(raw), "840.1") ||
		!strings.Contains(got, "840.1|<@"+alice+"> could not resume the run: set JENKINS_MOWER_AUTH") ||
		!strings.Contains(got, "841.1|PLAN for v1.37.1-rc2+rke2r1\n\nNot started: v1.37.1-rc2+rke2r1 already running") {
		t.Fatalf("saved %d, state %s, posts:\n%s", tl.SavedRuns(), raw, got)
	}
}
