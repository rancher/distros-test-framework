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

// Two runs that both end with unknown builds keep separate blocks: unblocking one frees only its
// slots, new plans stay blocked until the other is unblocked too, and both survive a restart.
func TestListenerKeepsOneBlockPerRun(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	tl := newTestListener(t, nil)
	tl.StatePath = path
	tl.Capacity = &Capacity{}
	tl.errs = []error{
		fmt.Errorf("%w: job-a v1", ErrUnreconciledBuilds),
		fmt.Errorf("%w: job-b v2", ErrUnreconciledBuilds),
	}
	ctx := testContext(t)
	for _, ts := range []string{"730.1", "731.1"} {
		tl.Capacity.acquire("mower", 6)
		tl.Capacity.abandon("mower", threadKey(testChan, ts))
	}
	tl.gate = make(chan struct{})
	tl.Handle(ctx, mentionIn(testChan, alice, "730.1", "v1.37.1-rc2+rke2r1"))
	tl.Handle(ctx, mentionIn(testChan, alice, "731.1", "v1.36.5-rc2+rke2r1"))
	tl.waitRuns(t, 2)
	close(tl.gate)
	tl.Wait()

	restarted := newTestListener(t, nil)
	restarted.StatePath = path
	if err := restarted.Restore(); err != nil {
		t.Fatal(err)
	}
	if b := restarted.Blocked(); !strings.Contains(b, "thread 730.1") || !strings.Contains(b, "thread 731.1") {
		t.Fatalf("blocks after restart: %q", b)
	}

	tl.Handle(ctx, reply(alice, "731.1", "unblock"))
	stillBlocked := strings.Contains(tl.slack.all(), "1 other runs are still blocked")
	if tl.Blocked() == "" || tl.Capacity.InUse("mower") != 1 || !stillBlocked {
		t.Fatalf("after unblocking 731.1: blocked %q, in use %d:\n%s", tl.Blocked(), tl.Capacity.InUse("mower"),
			tl.slack.all())
	}
	tl.Handle(ctx, mention(alice, "732.1"))
	tl.Wait()
	if tl.runs.Load() != 2 {
		t.Fatalf("a new plan started while run 730.1 was still blocked")
	}

	tl.Handle(ctx, reply(alice, "730.1", "unblock"))
	tl.Handle(ctx, mention(alice, "733.1"))
	tl.Wait()
	if tl.Blocked() != "" || tl.Capacity.InUse("mower") != 0 || tl.runs.Load() != 3 {
		t.Fatalf("after both unblocks: blocked %q, in use %d, runs %d", tl.Blocked(), tl.Capacity.InUse("mower"),
			tl.runs.Load())
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

// unblock frees the slots that run's unknown builds held, and only those.
func TestListenerUnblockFreesItsSlots(t *testing.T) {
	tl := newTestListener(t, nil)
	tl.Capacity = &Capacity{}
	tl.errs = []error{fmt.Errorf("%w: job-a v1", ErrUnreconciledBuilds)}
	ctx := testContext(t)
	for _, owner := range []string{threadKey(testChan, "720.1"), threadKey(testChan, "other")} {
		tl.Capacity.acquire("mower", 6)
		tl.Capacity.abandon("mower", owner)
	}

	tl.Handle(ctx, mention(alice, "720.1"))
	tl.Wait()
	tl.Handle(ctx, reply(alice, "720.1", "unblock"))
	if tl.Capacity.InUse("mower") != 1 || tl.Capacity.Abandoned("mower") != 1 ||
		!strings.Contains(tl.slack.all(), "Freed 1 Jenkins slots") {
		t.Fatalf("in use %d, abandoned %d:\n%s", tl.Capacity.InUse("mower"), tl.Capacity.Abandoned("mower"),
			tl.slack.all())
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
	tl := newTestListener(t, errors.New("tags not found on GitHub: v9.9.9-rc1+rke2r1"))
	tl.Handle(testContext(t), mention(alice, "650.1"))
	tl.Wait()
	if !strings.Contains(tl.slack.all(), "650.1|Could not build a release plan: tags not found") {
		t.Fatalf("posts:\n%s", tl.slack.all())
	}
}

// A run that ends with builds in unknown state blocks the next plan until someone allowed replies
// `unblock` in that run's thread; their Jenkins capacity was never released.
func TestListenerBlocksAfterUnreconciledBuilds(t *testing.T) {
	tl := newTestListener(t, nil)
	tl.errs = []error{fmt.Errorf("1 of 2 jobs did not succeed; %w: job-a v1 (no build url)", ErrUnreconciledBuilds)}
	ctx := testContext(t)

	tl.Handle(ctx, mention(alice, "700.1"))
	tl.Wait()
	tl.Handle(ctx, mention(alice, "701.1"))
	tl.Wait()
	if tl.runs.Load() != 1 || !strings.Contains(tl.slack.all(), "701.1|PLAN for v1.37.1-rc2+rke2r1\n\nNot started: "+
		"new plans are blocked") {
		t.Fatalf("second plan ran while builds were unreconciled (runs=%d):\n%s", tl.runs.Load(), tl.slack.all())
	}

	tl.Handle(ctx, reply(mallory, "700.1", "unblock"))
	tl.Handle(ctx, reply(alice, "701.1", "unblock")) // wrong thread: ignored
	if tl.Blocked() == "" {
		t.Fatalf("unblocked by the wrong user or thread:\n%s", tl.slack.all())
	}

	tl.Handle(ctx, reply(alice, "700.1", "unblock"))
	tl.Handle(ctx, mention(alice, "702.1"))
	tl.Wait()
	if tl.runs.Load() != 2 || !strings.Contains(tl.slack.all(), "Unblocked this run, by <@"+alice+">") {
		t.Fatalf("runs=%d posts:\n%s", tl.runs.Load(), tl.slack.all())
	}
}

// A run that stopped part-way blocks new runs and keeps the block across restarts; a run that
// failed before starting anything does not.
func TestListenerBlocksAfterPartialRun(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	tl := newTestListener(t, nil)
	tl.StatePath = path
	tl.errs = []error{
		errors.New("GITHUB_TOKEN is required to dispatch wf.yaml"),
		fmt.Errorf("%w: %w", ErrPartialRun, context.Canceled),
	}
	ctx := testContext(t)

	for _, ts := range []string{"750.1", "751.1"} {
		tl.Handle(ctx, mention(alice, ts))
		tl.Wait()
	}
	if tl.runs.Load() != 2 || !strings.Contains(tl.slack.all(), "751.1|Release plan stopped part-way") {
		t.Fatalf("an error before any side effect must not block (runs=%d):\n%s", tl.runs.Load(), tl.slack.all())
	}

	next := newTestListener(t, nil)
	next.StatePath = path
	if err := next.Restore(); err != nil {
		t.Fatal(err)
	}
	next.Handle(ctx, mention(alice, "752.1"))
	next.Wait()
	if next.runs.Load() != 0 || next.Blocked() == "" || !strings.Contains(next.slack.all(), "new plans are blocked") {
		t.Fatalf("partial run did not block after restart (runs=%d):\n%s", next.runs.Load(), next.slack.all())
	}
}

// Runs in progress survive a restart as a block, even without a clean shutdown.
func TestListenerStateSurvivesRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	tl := newTestListener(t, nil)
	tl.StatePath = path
	tl.gate = make(chan struct{})
	ctx := testContext(t)

	tl.Handle(ctx, mention(alice, "800.1"))
	tl.waitRuns(t, 1)
	// Process killed here: only the state file is left.
	raw, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(raw), `"running":["`+testChan+`|800.1"]`) {
		t.Fatalf("in-progress marker not written before the run: %s %v", raw, err)
	}

	next := newTestListener(t, nil)
	next.StatePath = path
	if err = next.Restore(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(next.Blocked(), "stopped during the run in thread 800.1") {
		t.Fatalf("restart did not block: %q", next.Blocked())
	}
	next.Handle(ctx, mention(alice, "801.1"))
	next.waitPost(t, "801.1|") // plans run beside the event loop: let this one answer first
	next.Handle(ctx, reply(alice, "800.1", "unblock"))
	next.Handle(ctx, mention(alice, "802.1"))
	next.Wait()
	if next.runs.Load() != 1 || !strings.Contains(next.slack.all(), "801.1|PLAN for v1.37.1-rc2+rke2r1\n\nNot started") {
		t.Fatalf("runs=%d posts:\n%s", next.runs.Load(), next.slack.all())
	}
	raw, _ = os.ReadFile(path)
	var st listenerState
	_ = json.Unmarshal(raw, &st)
	if len(st.Blocks) != 0 || len(st.Running) != 0 {
		t.Fatalf("state after unblock and a clean run: %s", raw)
	}

	close(tl.gate)
	tl.Wait()
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
			rc.Notify("stopping: 2 builds still on Jenkins")

			return fmt.Errorf("%w: job-a v1", ErrUnreconciledBuilds)
		}}, nil
	}
	tl.Handle(ctx, mention(alice, "900.1"))
	tl.Wait()

	all := strings.Join(posts, "\n")
	if !strings.Contains(all, "stopping: 2 builds still on Jenkins") ||
		!strings.Contains(all, "finished with builds in unknown state") {
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
	if _, err = AcquireInstanceLock(path); !errors.Is(err, ErrAlreadyRunning) {
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

// A plan stuck on a slow Jenkins does not hold the event loop: `stop` and `status` in a running
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
