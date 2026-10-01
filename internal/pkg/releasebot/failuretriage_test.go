package releasebot

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// countingBroker answers every request with the same verdict, counting calls; hold makes the
// first call wait until released, like a triage in progress.
type countingBroker struct {
	calls     atomic.Int32
	hold      chan struct{}
	failFirst bool // only the first request fails (the broker recovers)
}

func (b *countingBroker) serve(t *testing.T, spool string) {
	t.Helper()
	holdBrokerLock(t, spool)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	br := &Broker{Spool: spool, Poll: time.Millisecond, Triage: func(context.Context, *TriageRequest) (*Verdict, error) {
		n := b.calls.Add(1)
		if n == 1 && b.hold != nil {
			<-b.hold
		}
		if n == 1 && b.failFirst {
			return &Verdict{Error: "claude: exit status 1"}, nil
		}

		return &Verdict{
			Action: "rerun", Bucket: "INFRA", Confidence: 90, Summary: "EC2 capacity",
			Evidence: []string{"InsufficientInstanceCapacity in tofu apply"},
		}, nil
	}}
	go func() { _ = br.Serve(ctx) }()
}

func failureOn(version string) *Outcome {
	return &Outcome{
		Job:      JenkinsJob{Name: "k3s-validate-cluster", Product: "k3s", Version: version},
		BuildURL: "https://j/" + version + "/", Result: "FAILURE",
	}
}

// capacityLog is a provisioning failure the transient rules accept (EC2 had no capacity).
func capacityLog(host string) string {
	return strings.Join([]string{
		"[Pipeline] { (Configure and Build)",
		"ok: [" + host + "] => changed=0 failed=0",
		"Error: creating EC2 Instance: InsufficientInstanceCapacity: no capacity in us-east-2c for t3a.medium",
		"Finished: FAILURE",
	}, "\n")
}

func newFailureTriage(spool string, logs map[string]string) *FailureTriage {
	return &FailureTriage{
		Log:    func(_ context.Context, u string) (string, error) { return logs[u], nil },
		Broker: &BrokerClient{Dir: spool, Timeout: 5 * time.Second, Poll: time.Millisecond},
	}
}

// The same failure on another RC reuses the verdict (one broker call), even when it arrives
// while the first triage is still running.
func TestFailureTriageReusesVerdict(t *testing.T) {
	spool := newSpool(t)
	b := &countingBroker{hold: make(chan struct{})}
	b.serve(t, spool)
	ft := newFailureTriage(spool, map[string]string{
		"https://j/v1.37/": capacityLog("10.0.0.1"), "https://j/v1.35/": capacityLog("10.0.9.9"),
	})

	var wg sync.WaitGroup
	ds := make([]Decision, 2)
	for i, v := range []string{"v1.37", "v1.35"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ds[i] = ft.Quick(context.Background(), failureOn(v))
		}()
		for b.calls.Load() == 0 { // v1.35 arrives while v1.37 is being triaged
			time.Sleep(time.Millisecond)
		}
	}
	close(b.hold)
	wg.Wait()

	if b.calls.Load() != 1 || !ds[0].Rerun || !ds[1].Rerun ||
		!strings.Contains(ds[1].Summary, "same failure as k3s-validate-cluster v1.37") {
		t.Fatalf("calls %d, decisions %+v", b.calls.Load(), ds)
	}
}

// One infrastructure outage that fails several jobs is triaged once; a test failure with the
// same signature in another job is triaged again.
func TestFailureTriageReuseAcrossJobs(t *testing.T) {
	spool := newSpool(t)
	b := &countingBroker{}
	b.serve(t, spool)
	test := ginkgoLog("x", "v", 1)
	ft := newFailureTriage(spool, map[string]string{
		"https://j/infra-a/": capacityLog("10.0.0.1"), "https://j/infra-b/": capacityLog("10.0.0.2"),
		"https://j/test-a/": test, "https://j/test-b/": test,
	})
	on := func(job, build string) *Outcome {
		return &Outcome{Job: JenkinsJob{Name: job, Version: "v1"}, BuildURL: "https://j/" + build + "/", Result: "FAILURE"}
	}

	ft.Quick(context.Background(), on("validate", "infra-a"))
	d := ft.Quick(context.Background(), on("conformance", "infra-b"))
	if b.calls.Load() != 1 || !strings.Contains(d.Summary, "same failure as validate v1") {
		t.Fatalf("infra: calls %d, %+v", b.calls.Load(), d)
	}
	ft.Quick(context.Background(), on("validate", "test-a"))
	ft.Quick(context.Background(), on("conformance", "test-b"))
	if b.calls.Load() != 3 {
		t.Fatalf("test failures: calls %d, want 3", b.calls.Load())
	}
}

// A triage that failed (broker error) is not reused: the next occurrence asks again, and once
// the broker answers, that verdict is the one reused.
func TestFailureTriageDoesNotReuseFailures(t *testing.T) {
	spool := newSpool(t)
	b := &countingBroker{failFirst: true}
	b.serve(t, spool)
	log := capacityLog("10.0.0.1")
	ft := newFailureTriage(spool, map[string]string{
		"https://j/v1.37/": log, "https://j/v1.36/": log, "https://j/v1.35/": log,
	})

	first := ft.Quick(context.Background(), failureOn("v1.37"))
	second := ft.Quick(context.Background(), failureOn("v1.36"))
	third := ft.Quick(context.Background(), failureOn("v1.35"))
	if first.Rerun || !second.Rerun || strings.Contains(second.Summary, "same failure") ||
		!strings.Contains(third.Summary, "same failure as k3s-validate-cluster v1.36") || b.calls.Load() != 2 {
		t.Fatalf("calls %d, decisions %+v %+v %+v", b.calls.Load(), first, second, third)
	}
}

// Reuse ends after the reuse window, and Full never reuses.
func TestFailureTriageReuseWindow(t *testing.T) {
	spool := newSpool(t)
	b := &countingBroker{}
	b.serve(t, spool)
	log := capacityLog("10.0.0.1")
	ft := newFailureTriage(spool, map[string]string{"https://j/v1.37/": log, "https://j/v1.35/": log})
	now := time.Unix(1_000_000, 0)
	ft.Now = func() time.Time { return now }
	ft.Reuse = time.Hour

	ft.Quick(context.Background(), failureOn("v1.37"))
	now = now.Add(2 * time.Hour)
	ft.Quick(context.Background(), failureOn("v1.35"))
	ft.Full(context.Background(), failureOn("v1.35"))
	if b.calls.Load() != 3 {
		t.Fatalf("calls %d, want 3 (expired reuse, and full never reuses)", b.calls.Load())
	}
}

// Quick triage runs the smaller model without the verifier; full keeps the default model and
// may start the verifier.
func TestClaudeTriageModes(t *testing.T) {
	root := t.TempDir()
	dir, manifest := writeSkill(t, root)
	ct := &ClaudeTriage{
		Workspace: root, SkillDir: dir, Manifest: manifest, QuickModel: "sonnet",
		MCPConfig: filepath.Join(root, "private", "mcp.json"),
	}

	quickReq := &TriageRequest{Mode: TriageQuick, Excerpt: "[FAILED] boom"}
	quick, _ := ct.Args(quickReq)
	full, _ := ct.Args(&TriageRequest{Mode: TriageFull})
	prompt := triagePrompt(quickReq)
	q, f := strings.Join(quick, "\n"), strings.Join(full, "\n")
	if !strings.Contains(q, "--model\nsonnet\n") || strings.Contains(f, "--model") {
		t.Fatalf("models:\nquick %q\nfull %q", q, f)
	}
	if !strings.HasSuffix(q, "\nAgent") || !strings.Contains(prompt, "Do not start the independent verifier") ||
		!strings.Contains(prompt, "<excerpt>\n[FAILED] boom\n</excerpt>") {
		t.Fatalf("quick args:\n%s\nprompt:\n%s", q, prompt)
	}
	allowed, denied, ok := strings.Cut(f, "--disallowedTools")
	if !ok || !strings.Contains(allowed, "\nAgent\n") || strings.Contains(denied, "Agent") {
		t.Fatalf("full triage must allow the verifier:\n%s", f)
	}
}

// A second `triage` for the same job while one runs (Slack redelivery, a double reply) does not
// start another paid analysis.
func TestHeldJobFullTriageOnce(t *testing.T) {
	h := newTriageHarness(map[string]int{"smoke": 9}, askTriager)
	release := make(chan struct{})
	var runs atomic.Int32
	h.s.DeepTriage = func(context.Context, *Outcome) Decision {
		runs.Add(1)
		<-release
		return Decision{Summary: "done"}
	}
	done := runAsync(h.s, smokeThenConf())
	h.waitHelp(t, 1)
	h.send(t, Command{Action: CommandTriage, Job: "smoke", By: "U1"})
	if r := h.send(t, Command{Action: CommandTriage, Job: "smoke", By: "U2"}); !strings.Contains(r, "already running") {
		t.Fatalf("second triage reply %q", r)
	}
	close(release)
	h.waitHelp(t, 2)
	if n := runs.Load(); n != 1 {
		t.Fatalf("full triage ran %d times", n)
	}
	h.send(t, Command{Action: CommandSkip, Job: "smoke", By: "U1"})
	waitOut(t, done)
}

// A run that ends while a full triage is running (the job was skipped meanwhile) still posts its
// result: the run waits for it instead of dropping it.
func TestFullTriageOutlivesItsJob(t *testing.T) {
	h := newTriageHarness(map[string]int{"smoke": 9}, askTriager)
	release := make(chan struct{})
	h.s.DeepTriage = func(ctx context.Context, _ *Outcome) Decision {
		select {
		case <-release:
			return Decision{Summary: "PRODUCT: k3s#14508"}
		case <-ctx.Done():
			return Decision{Summary: "canceled"}
		}
	}
	done := runAsync(h.s, smokeThenConf())
	h.waitHelp(t, 1)
	h.send(t, Command{Action: CommandTriage, Job: "smoke", By: "U1"})
	h.send(t, Command{Action: CommandSkip, Job: "smoke", By: "U1"})
	select {
	case <-done:
		t.Fatal("the run ended before the full triage it promised to post")
	case <-time.After(200 * time.Millisecond):
	}
	close(release)
	waitOut(t, done)
	if msg := h.waitHelp(t, 2); msg != "Full triage of smoke v1 (asked by <@U1>): PRODUCT: k3s#14508" {
		t.Fatalf("posted %q", msg)
	}
}

// The run ends only after its last help message is delivered: a slow Slack post of the full
// triage is not cut off by the run returning (and the process exiting).
func TestRunEndsAfterHelpIsDelivered(t *testing.T) {
	h := newTriageHarness(map[string]int{"smoke": 9}, askTriager)
	posting := make(chan struct{})
	delivered := make(chan struct{})
	plain := h.s.Help
	h.s.Help = func(m string) {
		if strings.HasPrefix(m, "Full triage") {
			close(posting)
			time.Sleep(100 * time.Millisecond) // a slow post
			defer close(delivered)
		}
		plain(m)
	}
	h.s.DeepTriage = func(context.Context, *Outcome) Decision { return Decision{Summary: "PRODUCT"} }
	done := runAsync(h.s, smokeThenConf())
	h.waitHelp(t, 1)
	h.send(t, Command{Action: CommandTriage, Job: "smoke", By: "U1"})
	<-posting
	h.send(t, Command{Action: CommandSkip, Job: "smoke", By: "U1"})
	waitOut(t, done)
	select {
	case <-delivered:
	default:
		t.Fatal("the run returned before its full triage was posted")
	}
}

// `triage` on a waiting job posts the full analysis; the job keeps waiting for retry/skip.
func TestHeldJobFullTriage(t *testing.T) {
	h := newTriageHarness(map[string]int{"smoke": 9}, askTriager)
	h.s.DeepTriage = func(context.Context, *Outcome) Decision { return Decision{Summary: "PRODUCT, 85%: k3s#14508"} }
	done := runAsync(h.s, smokeThenConf())
	h.waitHelp(t, 1)
	r := h.send(t, Command{Action: CommandTriage, Job: "smoke", By: "U1"})
	if !strings.Contains(r, "Running the full triage") {
		t.Fatalf("reply %q", r)
	}
	if msg := h.waitHelp(t, 2); msg != "Full triage of smoke v1 (asked by <@U1>): PRODUCT, 85%: k3s#14508" {
		t.Fatalf("posted %q", msg)
	}
	if st := h.s.Status(); !strings.Contains(st, "1 need help") {
		t.Fatalf("job stopped waiting: %q", st)
	}
	h.send(t, Command{Action: CommandSkip, Job: "smoke", By: "U1"})
	waitOut(t, done)
}

// The tail is the log's real end even when the log is several times the kept size, and a log
// past the read limit is refused rather than cut short silently.
func TestConsoleTail(t *testing.T) {
	log := strings.Repeat("x", 5*1000) + "[FAILED] the end"
	got, err := consoleTail(strings.NewReader(log), 1000, 1<<20)
	if err != nil || len(got) != 1000 || !strings.HasSuffix(got, "[FAILED] the end") {
		t.Fatalf("len %d, err %v", len(got), err)
	}
	if _, err = consoleTail(strings.NewReader(log), 1000, 4000); err == nil {
		t.Fatal("a log past the read limit was accepted")
	}

	// A log of five times the real kept size, streamed quickly, still ends with its failure.
	start := time.Now()
	big := strings.Repeat("y", 5*maxConsole) + "[FAILED] real end"
	got, err = consoleTail(strings.NewReader(big), maxConsole, maxConsoleRead)
	took := time.Since(start)
	if err != nil || len(got) != maxConsole || !strings.HasSuffix(got, "[FAILED] real end") || took > 5*time.Second {
		t.Fatalf("len %d, err %v, took %s", len(got), err, took)
	}
}

// ConsoleText keeps the end of a log that is longer than the limit (the failure is there).
func TestJenkinsConsoleTextKeepsTail(t *testing.T) {
	big := strings.Repeat("x", maxConsole) + "\n[FAILED] the end"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/job/j/7/consoleText" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(big))
	}))
	defer srv.Close()
	log, err := (&Jenkins{BaseURL: srv.URL, HTTP: srv.Client()}).ConsoleText(context.Background(), srv.URL+"/job/j/7/")
	if err != nil || len(log) != maxConsole || !strings.HasSuffix(log, "[FAILED] the end") {
		t.Fatalf("len %d, err %v", len(log), err)
	}
}

// A rerun needs the rule to grant it; the verdict can only veto. Either alone asks.
func TestRerunNeedsTransientRule(t *testing.T) {
	capacity := AnalyzeFailure(capacityLog("10.0.0.1"))
	product := AnalyzeFailure(ginkgoLog("0158ab16-37b5-447f-97da-6d35ee3873ab", "v1.37", 178))
	for name, c := range map[string]struct {
		d     Decision
		fail  *Failure
		rerun bool
		note  string
	}{
		"verdict and rule agree": {
			Decision{Rerun: true, Summary: "INFRA"},
			&capacity, true,
			"[rerun rule aws-insufficient-capacity",
		},
		"verdict says rerun on a test failure": {
			Decision{Rerun: true, Summary: "INFRA"},
			&product, false,
			"matches no transient-infrastructure rule",
		},
		"rule alone":      {Decision{Summary: "PRODUCT"}, &capacity, false, ""},
		"no log to check": {Decision{Rerun: true, Summary: "INFRA"}, &Failure{}, false, "matches no transient"},
	} {
		d := GateRerun(c.d, c.fail)
		if d.Rerun != c.rerun || !strings.Contains(d.Summary, c.note) {
			t.Fatalf("%s: %+v", name, d)
		}
	}
}

// After a failed triage, the occurrences waiting for it coordinate again: one asks, the rest
// reuse its verdict (two broker calls for three occurrences, not three).
func TestFailureTriageRecoversTogether(t *testing.T) {
	spool := newSpool(t)
	b := &countingBroker{failFirst: true, hold: make(chan struct{})}
	b.serve(t, spool)
	log := capacityLog("10.0.0.1")
	ft := newFailureTriage(spool, map[string]string{
		"https://j/v1.37/": log, "https://j/v1.36/": log, "https://j/v1.35/": log,
	})
	var wg sync.WaitGroup
	for _, v := range []string{"v1.37", "v1.36", "v1.35"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ft.Quick(context.Background(), failureOn(v))
		}()
		for b.calls.Load() == 0 { // the others arrive while the first triage is held
			time.Sleep(time.Millisecond)
		}
	}
	close(b.hold)
	wg.Wait()
	if n := b.calls.Load(); n != 2 {
		t.Fatalf("broker calls %d, want 2", n)
	}
}
