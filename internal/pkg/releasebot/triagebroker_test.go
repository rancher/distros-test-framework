package releasebot

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// Tests cannot chown to root: their spools are owned by the test's own uid.
func init() { spoolRootUID = os.Geteuid() }

func newSpool(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	shared := 0o770 | os.ModeSetgid
	for d, mode := range map[string]os.FileMode{spoolIn: shared, spoolOut: shared, spoolWork: 0o700} {
		p := filepath.Join(dir, d)
		if err := os.Mkdir(p, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(p, mode); err != nil {
			t.Fatal(err)
		}
	}
	lock := filepath.Join(dir, BrokerLock)
	if err := os.WriteFile(lock, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(lock, 0o640); err != nil {
		t.Fatal(err)
	}

	return dir
}

func failedOutcome() Outcome {
	return Outcome{
		Job:      JenkinsJob{Name: "smoke", Path: "f/smoke", Product: "rke2", Version: "v1"},
		BuildURL: "https://jenkins/job/f/job/smoke/7/", Result: "FAILURE",
	}
}

func quickRequest() *TriageRequest {
	o := failedOutcome()

	return &TriageRequest{
		Mode: TriageQuick, JobName: o.Job.Name, JobPath: o.Job.Path, Product: o.Job.Product,
		Version: o.Job.Version, Result: o.Result, BuildURL: o.BuildURL, Attempt: 1,
	}
}

// The bot's request reaches the broker, and the broker's verdict comes back as a decision; the
// spool files are group-readable even under the bot's UMask=0077.
func TestSpoolTriagerAndBroker(t *testing.T) {
	old := syscall.Umask(0o077)
	defer syscall.Umask(old)
	spool := newSpool(t)
	got := make(chan TriageRequest, 1)
	b := &Broker{Spool: spool, Poll: time.Millisecond, Triage: func(_ context.Context, r *TriageRequest) (*Verdict, error) {
		info, err := os.Stat(filepath.Join(spool, spoolWork, r.ID+".json"))
		if err != nil || info.Mode().Perm() != spoolMode {
			t.Errorf("request file mode %v, err %v", info.Mode().Perm(), err)
		}
		got <- *r

		return &Verdict{
			Action: "rerun", Bucket: "INFRA", Confidence: 90, Summary: "EC2 capacity error",
			Evidence: []string{"InsufficientInstanceCapacity"},
		}, nil
	}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	holdBrokerLock(t, spool)
	go func() { _ = b.Serve(ctx) }()

	d, _ := (&BrokerClient{Dir: spool, Timeout: 5 * time.Second, Poll: time.Millisecond}).Ask(ctx, quickRequest())
	if !d.Rerun || d.Summary != "INFRA, 90% confidence: EC2 capacity error" {
		t.Fatalf("decision %+v", d)
	}
	r := <-got
	if r.BuildURL != "https://jenkins/job/f/job/smoke/7/" || r.Attempt != 1 || r.Result != "FAILURE" {
		t.Fatalf("request %+v", r)
	}
	for _, d := range []string{spoolIn, spoolWork, spoolOut} {
		if left, _ := os.ReadDir(filepath.Join(spool, d)); len(left) != 0 {
			t.Fatalf("%s not cleaned: %v", d, left)
		}
	}
}

// Anything but a clean rerun verdict asks a person: broker errors, and no broker at all (the
// request is withdrawn so a later broker does not triage it for nobody).
func TestSpoolTriagerAsksOnFailure(t *testing.T) {
	spool := newSpool(t)
	b := &Broker{Spool: spool, Poll: time.Millisecond, Triage: func(context.Context, *TriageRequest) (*Verdict, error) {
		return nil, errors.New("claude: exit status 1")
	}}
	ctx, cancel := context.WithCancel(context.Background())
	holdBrokerLock(t, spool)
	go func() { _ = b.Serve(ctx) }()
	d, _ := (&BrokerClient{Dir: spool, Timeout: 5 * time.Second, Poll: time.Millisecond}).Ask(ctx, quickRequest())
	cancel()
	if d.Rerun || !strings.Contains(d.Summary, "automatic triage failed: claude: exit status 1") {
		t.Fatalf("decision %+v", d)
	}

	// A broker that holds the lock but never answers: the request times out and is withdrawn.
	stuck := newSpool(t)
	holdBrokerLock(t, stuck)
	stuckClient := &BrokerClient{Dir: stuck, Timeout: 20 * time.Millisecond, Poll: time.Millisecond}
	d, _ = stuckClient.Ask(context.Background(), quickRequest())
	if d.Rerun || !strings.Contains(d.Summary, "no verdict within") {
		t.Fatalf("decision from a stuck broker %+v", d)
	}
	if left, _ := os.ReadDir(filepath.Join(stuck, spoolIn)); len(left) != 0 {
		t.Fatalf("request not withdrawn: %v", left)
	}

	// No broker at all: ask at once instead of waiting for the timeout.
	start := time.Now()
	noBroker := &BrokerClient{Dir: newSpool(t), Timeout: time.Hour, Poll: time.Millisecond}
	d, _ = noBroker.Ask(context.Background(), quickRequest())
	if d.Rerun || !strings.Contains(d.Summary, "broker is not running") || time.Since(start) > time.Second {
		t.Fatalf("decision without broker %+v after %s", d, time.Since(start))
	}
}

// holdBrokerLock takes the spool's broker lock as a running broker would.
func holdBrokerLock(t *testing.T, spool string) {
	t.Helper()
	release, err := AcquireBrokerLock(spool)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)
	if !brokerRunning(spool) {
		t.Fatal("brokerRunning does not see the held lock")
	}
}

// The summary goes to Slack: anything that looks like a credential is removed.
func TestVerdictRedactsSecrets(t *testing.T) {
	v := &Verdict{
		Action: "ask", Bucket: "PRODUCT", Confidence: 70,
		Summary: "token " + strings.Repeat("Zx9", 16) + " leaked; see log",
	}
	if s := v.Decision().Summary; strings.Contains(s, "Zx9") || !strings.HasSuffix(s, "token [redacted]") {
		t.Fatalf("summary %q", s)
	}
}

func TestParseClaudeVerdict(t *testing.T) {
	cases := map[string]struct {
		out, action, err string
	}{
		"structured": {out: `{"is_error":false,"result":"done","structured_output":{"action":"ask","bucket":"PRODUCT",` +
			`"confidence":80,"summary":"s"}}`, action: "ask"},
		"result text": {out: `{"is_error":false,"result":"{\"action\":\"rerun\",\"bucket\":\"INFRA\",` +
			`\"confidence\":90,\"summary\":\"s\"}"}`, action: "rerun"},
		"claude error": {out: `{"is_error":true,"result":"API error"}`, err: "claude reported an error"},
		"extra field": {out: `{"structured_output":{"action":"ask","bucket":"X","confidence":1,"summary":"s",` +
			`"rerun_now":true}}`, err: "does not match the schema"},
		"bad action": {
			out: `{"structured_output":{"action":"trigger","bucket":"X","confidence":1,"summary":"s"}}`,
			err: `action "trigger"`,
		},
		"not json": {out: `Error: no credentials`, err: "not JSON"},
	}
	for name, c := range cases {
		v, err := parseClaudeVerdict([]byte(c.out))
		switch {
		case c.err != "" && (err == nil || !strings.Contains(err.Error(), c.err)):
			t.Fatalf("%s: err %v, want %q", name, err, c.err)
		case c.err == "" && (err != nil || v.Action != c.action):
			t.Fatalf("%s: %+v %v", name, v, err)
		}
	}
}

// writeSkill creates a skill dir and its manifest; tamper lets a test break it afterwards.
func writeSkill(t *testing.T, root string) (dir, manifest string) {
	t.Helper()
	dir = filepath.Join(root, "skill")
	files := map[string]string{
		".claude-plugin/plugin.json":             `{"name":"distros-qa-skills"}`,
		"skills/jenkins-failure-triage/SKILL.md": "# triage\n",
	}
	var lines []string
	for rel, body := range files {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		lines = append(lines, sha256hex(body)+"  "+rel)
	}
	manifest = filepath.Join(root, "triage-skill.sha256")
	if err := os.WriteFile(manifest, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	return dir, manifest
}

func TestVerifySkill(t *testing.T) {
	for name, tamper := range map[string]func(dir string){
		"pinned": func(string) {},
		"modified": func(d string) {
			_ = os.WriteFile(filepath.Join(d, "skills", "jenkins-failure-triage", "SKILL.md"), []byte("x"), 0o644)
		},
		"extra":   func(d string) { _ = os.WriteFile(filepath.Join(d, "notes.md"), []byte("x"), 0o644) },
		"missing": func(d string) { _ = os.Remove(filepath.Join(d, ".claude-plugin", "plugin.json")) },
	} {
		dir, manifest := writeSkill(t, t.TempDir())
		tamper(dir)
		err := VerifySkill(dir, manifest)
		if (name == "pinned") != (err == nil) {
			t.Fatalf("%s: %v", name, err)
		}
	}

	// A symlink is refused as such, even one to a file with the pinned content outside the skill.
	root := t.TempDir()
	dir, manifest := writeSkill(t, root)
	p := filepath.Join(dir, "skills", "jenkins-failure-triage", "SKILL.md")
	outside := filepath.Join(root, "SKILL.md")
	if err := os.Rename(p, outside); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, p); err != nil {
		t.Fatal(err)
	}
	if err := VerifySkill(dir, manifest); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("symlinked file: %v", err)
	}
}

// ClaudeTriage runs the command with the pinned plugin, schema and tool limits, and never runs
// it when the skill does not match the manifest.
func TestClaudeTriageRunsSandbox(t *testing.T) {
	root := t.TempDir()
	dir, manifest := writeSkill(t, root)
	fake := filepath.Join(root, "claude-sandbox")
	argsFile, stdinFile := filepath.Join(root, "args"), filepath.Join(root, "stdin")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + argsFile + "\npwd >> " + argsFile + "\ncat > " + stdinFile + "\n" +
		`echo '{"is_error":false,"structured_output":` +
		`{"action":"ask","bucket":"PRODUCT","confidence":85,"summary":"s"}}'` + "\n"
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	ct := &ClaudeTriage{
		Command: fake, Workspace: root, SkillDir: dir, Manifest: manifest, Timeout: 10 * time.Second,
		MCPConfig: filepath.Join(root, "private", "mcp.json"),
	}

	v, err := ct.Triage(context.Background(), &TriageRequest{JobName: "smoke", BuildURL: "https://j/7/", Attempt: 1})
	if err != nil || v.Action != "ask" || v.Confidence != 85 {
		t.Fatalf("verdict %+v, err %v", v, err)
	}
	raw, _ := os.ReadFile(argsFile)
	args := string(raw)
	for _, want := range []string{
		"--plugin-dir\n/workspace/skill\n", "--json-schema\n", "--no-session-persistence\n",
		"--permission-mode\ndontAsk\n", "--strict-mcp-config\n--mcp-config\n/workspace/private/mcp.json\n",
		"Read(//workspace/private/**)\n",
		"Read(//workspace/skill/**)\n", "Glob(//workspace/skill/**)\n", "Grep(//workspace/skill/**)\n",
		"mcp__jenkins-mower__getBuildLog\n", "--disallowedTools\nBash\n",
		"mcp__jenkins-mower__triggerBuild\n", root + "\n",
	} {
		if !strings.Contains(args, want) {
			t.Fatalf("args missing %q:\n%s", want, args)
		}
	}
	// The prompt, with its log excerpt, is read from stdin and never shows in argv.
	if stdin, _ := os.ReadFile(stdinFile); !strings.Contains(string(stdin), "https://j/7/") ||
		strings.Contains(args, "https://j/7/") {
		t.Fatalf("prompt on stdin %q, args:\n%s", stdin, args)
	}
	if strings.Contains(args, "WebFetch\nmcp__jenkins-mower__getBuild") || strings.Count(args, "Bash") != 1 ||
		strings.Contains(args, "\nGlob\n") || strings.Contains(args, "\nGrep\n") {
		t.Fatalf("a denied or unscoped tool is allowed:\n%s", args)
	}

	_ = os.Remove(argsFile)
	_ = os.WriteFile(filepath.Join(dir, "skills", "jenkins-failure-triage", "SKILL.md"), []byte("tampered"), 0o644)
	if _, err = ct.Triage(context.Background(), &TriageRequest{}); err == nil || !strings.Contains(err.Error(), "pinned") {
		t.Fatalf("tampered skill: %v", err)
	}
	if _, statErr := os.Stat(argsFile); statErr == nil {
		t.Fatal("claude ran with a tampered skill")
	}
}

func sha256hex(s string) string {
	sum := sha256.Sum256([]byte(s))

	return hex.EncodeToString(sum[:])
}

// Verdicts the bot never collected are removed after KeepVerdicts; fresh ones stay.
func TestBrokerSweepsOldVerdicts(t *testing.T) {
	spool := newSpool(t)
	old := filepath.Join(spool, spoolOut, "old.json")
	fresh := filepath.Join(spool, spoolOut, "fresh.json")
	for _, p := range []string{old, fresh} {
		if err := os.WriteFile(p, []byte("{}"), spoolMode); err != nil {
			t.Fatal(err)
		}
	}
	past := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(old, past, past); err != nil {
		t.Fatal(err)
	}
	(&Broker{Spool: spool, KeepVerdicts: time.Hour}).sweep()
	if _, err := os.Stat(old); err == nil {
		t.Fatal("old verdict kept")
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Fatalf("fresh verdict removed: %v", err)
	}
}

// The MCP config (with the jenkins credential) and the skill must sit where the permission rules
// expect them: the skill at <workspace>/skill, the config outside it.
func TestClaudeTriageRefusesUnsafeLayout(t *testing.T) {
	root := t.TempDir()
	dir, manifest := writeSkill(t, root)
	for name, ct := range map[string]*ClaudeTriage{
		"config inside the skill": {
			Workspace: root, SkillDir: dir, Manifest: manifest,
			MCPConfig: filepath.Join(dir, "mcp.json"),
		},
		"config readable at the workspace root": {
			Workspace: root, SkillDir: dir, Manifest: manifest,
			MCPConfig: filepath.Join(root, "mcp.json"),
		},
		"config outside the workspace": {
			Workspace: root, SkillDir: dir, Manifest: manifest,
			MCPConfig: filepath.Join(t.TempDir(), "mcp.json"),
		},
		"skill elsewhere": {
			Workspace: root, SkillDir: filepath.Join(root, "other"), Manifest: manifest,
			MCPConfig: filepath.Join(root, "private", "mcp.json"),
		},
	} {
		if _, err := ct.Args(&TriageRequest{}); err == nil {
			t.Fatalf("%s: accepted", name)
		}
	}
}

// Canceling ends a wrapper with a foreground child, like claude-sandbox: the group gets SIGTERM,
// the trap runs, and the container is stopped by name.
func TestClaudeTriageCancelStopsWrapper(t *testing.T) {
	root := t.TempDir()
	dir, manifest := writeSkill(t, root)
	trapped := filepath.Join(root, "trapped")
	ready := filepath.Join(root, "ready")
	fake := filepath.Join(root, "claude-sandbox")
	script := "#!/bin/bash\ntrap 'echo stopped > " + trapped + "; exit 143' TERM\ntouch " + ready + "\nsleep 60\n"
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	stopped := make(chan int, 1)
	ct := &ClaudeTriage{
		Command: fake, Workspace: root, SkillDir: dir, Manifest: manifest, Timeout: time.Minute,
		MCPConfig: filepath.Join(root, "private", "mcp.json"), StopContainer: func(pid int) { stopped <- pid },
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
			if _, err := os.Stat(ready); err == nil {
				break
			}
		}
		cancel()
	}()

	start := time.Now()
	_, err := ct.Triage(ctx, &TriageRequest{})
	if err == nil || time.Since(start) > 20*time.Second {
		t.Fatalf("err %v after %s", err, time.Since(start))
	}
	if _, statErr := os.Stat(trapped); statErr != nil {
		t.Fatalf("the wrapper's TERM trap did not run: err %v", err)
	}
	select {
	case pid := <-stopped:
		if pid <= 0 {
			t.Fatalf("container stop for pid %d", pid)
		}
	default:
		t.Fatal("the container was not stopped after the cancel")
	}
}

// A SIGTERM sent before the wrapper starts its child (forced here: TERM ignored for a second) is
// lost; the repeated signal still ends the run through the trap.
func TestClaudeTriageCancelReachesLateChild(t *testing.T) {
	root := t.TempDir()
	dir, manifest := writeSkill(t, root)
	trapped := filepath.Join(root, "trapped")
	ready := filepath.Join(root, "ready")
	fake := filepath.Join(root, "claude-sandbox")
	script := "#!/bin/bash\ntrap '' TERM\ntouch " + ready + "\nsleep 1\n" +
		"trap 'echo stopped > " + trapped + "; exit 143' TERM\nsleep 60\n"
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	ct := &ClaudeTriage{
		Command: fake, Workspace: root, SkillDir: dir, Manifest: manifest, Timeout: time.Minute,
		MCPConfig: filepath.Join(root, "private", "mcp.json"), StopContainer: func(int) {},
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
			if _, err := os.Stat(ready); err == nil {
				break
			}
		}
		cancel()
	}()

	start := time.Now()
	if _, err := ct.Triage(ctx, &TriageRequest{}); err == nil || time.Since(start) > 20*time.Second {
		t.Fatalf("err %v after %s", err, time.Since(start))
	}
	if _, err := os.Stat(trapped); err != nil {
		t.Fatal("the child started after the first SIGTERM was never signaled")
	}
}

// Anything short of the full contract asks: from claude, and again on the spool.
func TestVerdictContract(t *testing.T) {
	for name, raw := range map[string]string{
		"action only":        `{"action":"rerun"}`,
		"confidence 900":     `{"action":"rerun","bucket":"INFRA","confidence":900,"summary":"s"}`,
		"empty summary":      `{"action":"rerun","bucket":"INFRA","confidence":90,"summary":" "}`,
		"trailing object":    `{"action":"ask","bucket":"X","confidence":1,"summary":"s"} {"action":"rerun"}`,
		"trailing bracket":   `{"action":"rerun","bucket":"INFRA","confidence":90,"summary":"s","evidence":["e"]}]`,
		"trailing brace":     `{"action":"rerun","bucket":"INFRA","confidence":90,"summary":"s","evidence":["e"]}}`,
		"missing confidence": `{"action":"rerun","bucket":"INFRA","summary":"s","evidence":["e"]}`,
		"null confidence":    `{"action":"rerun","bucket":"INFRA","confidence":null,"summary":"s","evidence":["e"]}`,
		"string confidence":  `{"action":"rerun","bucket":"INFRA","confidence":"90","summary":"s","evidence":["e"]}`,
		"null evidence":      `{"action":"rerun","bucket":"INFRA","confidence":90,"summary":"s","evidence":[null]}`,
		"unknown bucket":     `{"action":"rerun","bucket":"INFRA-X","confidence":90,"summary":"s","evidence":["e"]}`,
		"empty evidence":     `{"action":"rerun","bucket":"INFRA","confidence":90,"summary":"s","evidence":[""]}`,
		"blank evidence":     `{"action":"rerun","bucket":"INFRA","confidence":90,"summary":"s","evidence":["  "]}`,
		"missing bucket":     `{"action":"rerun","confidence":90,"summary":"s","evidence":["e"]}`,
		"too much evidence": `{"action":"ask","bucket":"X","confidence":1,"summary":"s",` +
			`"evidence":["a","b","c","d","e","f"]}`,
	} {
		if _, err := decodeVerdict([]byte(raw)); err == nil {
			t.Fatalf("%s: accepted", name)
		}
	}
	if _, err := parseClaudeVerdict([]byte(`{"structured_output":{"error":"x"}}`)); err == nil {
		t.Fatal("a model-set error was accepted")
	}

	spool := newSpool(t)
	holdBrokerLock(t, spool)
	ctx, cancel := context.WithCancel(testContext(t))
	defer cancel()
	answered := make(chan error, 1)
	go func() {
		answered <- writeMalformedVerdict(ctx, spool)
	}()
	c := &BrokerClient{Dir: spool, Timeout: 5 * time.Second, Poll: time.Millisecond}
	d, ok := c.Ask(ctx, quickRequest())
	cancel()
	if err := receive(t, answered); err != nil {
		t.Fatalf("malformed-verdict responder did not answer: %v", err)
	}
	if d.Rerun || ok || !strings.Contains(d.Summary, "bad verdict") {
		t.Fatalf("a bare rerun on the spool was accepted: %+v", d)
	}
}

func writeMalformedVerdict(ctx context.Context, spool string) error {
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for ctx.Err() == nil {
		entries, err := os.ReadDir(filepath.Join(spool, spoolIn))
		if err != nil {
			return err
		}
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".json") {
				id := strings.TrimSuffix(e.Name(), ".json")
				return os.WriteFile(filepath.Join(spool, spoolOut, id+".json"), []byte(`{"action":"rerun"}`), spoolMode)
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}

	return ctx.Err()
}

func TestMalformedVerdictResponderStopsWithoutRequest(t *testing.T) {
	spool := newSpool(t)
	ctx, cancel := context.WithTimeout(testContext(t), 10*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- writeMalformedVerdict(ctx, spool) }()
	if err := receive(t, done); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("responder without a request did not time out: %v", err)
	}
}

// Failure messages go to Slack too: they are scrubbed like summaries.
func TestVerdictErrorIsRedacted(t *testing.T) {
	d := (&Verdict{Error: "claude: token " + strings.Repeat("Zx9", 16) + " rejected"}).Decision()
	if strings.Contains(d.Summary, "Zx9") || !strings.HasSuffix(d.Summary, "token [redacted]") {
		t.Fatalf("summary %q", d.Summary)
	}
}

// Spool writes never follow a planted symlink, and symlinked requests are not claimed.
func TestSpoolIgnoresSymlinks(t *testing.T) {
	spool := newSpool(t)
	sentinel := filepath.Join(t.TempDir(), "precious")
	if err := os.WriteFile(sentinel, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Links at the name an older writer used and at a name like the temporary ones.
	for _, name := range []string{".abc.tmp", ".spool-1.tmp"} {
		_ = os.Symlink(sentinel, filepath.Join(spool, spoolOut, name))
	}
	if err := writeSpoolFile(filepath.Join(spool, spoolOut), "abc", &Verdict{Action: "ask"}); err != nil {
		t.Fatal(err)
	}
	if raw, _ := os.ReadFile(sentinel); string(raw) != "keep" {
		t.Fatalf("sentinel overwritten: %q", raw)
	}

	_ = os.Symlink(sentinel, filepath.Join(spool, spoolIn, "evil.json"))
	b := &Broker{Spool: spool, Triage: func(context.Context, *TriageRequest) (*Verdict, error) {
		t.Error("a symlinked request was triaged")
		return &Verdict{}, nil
	}}
	if served, _ := b.serveOne(context.Background()); served {
		t.Fatal("a symlinked request was claimed")
	}
	if _, err := readSpoolFile(filepath.Join(spool, spoolIn, "evil.json")); err == nil {
		t.Fatal("readSpoolFile followed a symlink")
	}
}

// A request a crashed broker left in work/ is answered by the next broker.
func TestBrokerRequeuesAfterCrash(t *testing.T) {
	spool := newSpool(t)
	req := quickRequest()
	req.ID = "left"
	if err := writeSpoolFile(filepath.Join(spool, spoolWork), req.ID, req); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	b := &Broker{Spool: spool, Poll: time.Millisecond, Triage: func(context.Context, *TriageRequest) (*Verdict, error) {
		return &Verdict{Action: "ask", Bucket: "INFRA", Confidence: 50, Summary: "s"}, nil
	}}
	go func() { _ = b.Serve(ctx) }()

	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(spool, spoolOut, "left.json")); err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("the request left in work/ was never answered")
		}
		time.Sleep(time.Millisecond)
	}
}

// A spool whose directories could be swapped (group-writable root, a symlinked out/) is refused.
func TestVerifySpool(t *testing.T) {
	if err := verifySpool(newSpool(t)); err != nil {
		t.Fatalf("good spool: %v", err)
	}

	writable := newSpool(t)
	if err := os.Chmod(writable, 0o770); err != nil {
		t.Fatal(err)
	}
	if err := verifySpool(writable); err == nil {
		t.Fatal("group-writable spool root accepted")
	}

	for name, change := range map[string]func(dir string) error{
		"group-writable work/": func(dir string) error { return os.Chmod(filepath.Join(dir, spoolWork), 0o770) },
		"in/ without setgid":   func(dir string) error { return os.Chmod(filepath.Join(dir, spoolIn), 0o770) },
		"world-readable out/": func(dir string) error {
			return os.Chmod(filepath.Join(dir, spoolOut), 0o775|os.ModeSetgid)
		},
		"work/ is a file": func(dir string) error {
			if err := os.Remove(filepath.Join(dir, spoolWork)); err != nil {
				return err
			}

			return os.WriteFile(filepath.Join(dir, spoolWork), nil, 0o600)
		},
	} {
		dir := newSpool(t)
		if err := change(dir); err != nil {
			t.Fatal(err)
		}
		if err := verifySpool(dir); err == nil {
			t.Fatalf("%s accepted", name)
		}
	}

	linked := newSpool(t)
	elsewhere := t.TempDir()
	if err := os.Remove(filepath.Join(linked, spoolOut)); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(elsewhere, filepath.Join(linked, spoolOut)); err != nil {
		t.Fatal(err)
	}
	if err := verifySpool(linked); err == nil {
		t.Fatal("symlinked out/ accepted")
	}
	b := &Broker{Spool: linked, Triage: func(context.Context, *TriageRequest) (*Verdict, error) { return &Verdict{}, nil }}
	if err := b.Serve(context.Background()); err == nil {
		t.Fatal("the broker served a spool with a symlinked out/")
	}
}

// When the bot stops waiting for a triage the broker already claimed (stop, shutdown, timeout),
// the broker's triage is canceled too instead of running to its own timeout.
func TestCancelReachesClaimedTriage(t *testing.T) {
	spool := newSpool(t)
	holdBrokerLock(t, spool)
	claimed := make(chan struct{})
	canceled := make(chan struct{})
	triage := func(ctx context.Context, _ *TriageRequest) (*Verdict, error) {
		close(claimed)
		<-ctx.Done()
		close(canceled)

		return nil, ctx.Err()
	}
	b := &Broker{Spool: spool, Poll: time.Millisecond, Triage: triage}
	serveCtx, stopServe := context.WithCancel(context.Background())
	defer stopServe()
	go func() { _ = b.Serve(serveCtx) }()

	askCtx, cancelAsk := context.WithCancel(context.Background())
	go func() {
		<-claimed
		cancelAsk()
	}()
	c := &BrokerClient{Dir: spool, Timeout: time.Minute, Poll: time.Millisecond}
	if d, ok := c.Ask(askCtx, quickRequest()); ok || d.Rerun {
		t.Fatalf("canceled ask returned %+v", d)
	}
	select {
	case <-canceled:
	case <-time.After(5 * time.Second):
		t.Fatal("the broker kept triaging after the bot canceled")
	}

	// The marker is removed once the triage ends.
	deadline := time.Now().Add(5 * time.Second)
	for {
		left, _ := os.ReadDir(filepath.Join(spool, spoolIn))
		if len(left) == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("left in in/: %v", left)
		}
		time.Sleep(time.Millisecond)
	}
}

// Old cancel markers (for triages that already ended) are swept like uncollected verdicts.
func TestBrokerSweepsOldCancelMarkers(t *testing.T) {
	spool := newSpool(t)
	marker := filepath.Join(spool, spoolIn, "gone"+cancelSuffix)
	if err := os.WriteFile(marker, nil, spoolMode); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-2 * time.Hour)
	_ = os.Chtimes(marker, past, past)
	(&Broker{Spool: spool, KeepVerdicts: time.Hour}).sweep()
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("stale cancel marker kept")
	}
}

// The lock is taken only as the regular 0640 file prepare makes: a lock that is a symlink or a
// hard link is refused without touching its target, and a lock with another mode is not re-moded.
func TestAcquireBrokerLock(t *testing.T) {
	dir := newSpool(t)
	release, err := AcquireBrokerLock(dir)
	if err != nil {
		t.Fatalf("good lock: %v", err)
	}
	if !brokerRunning(dir) {
		t.Fatal("the bot does not see the broker's lock")
	}
	if _, err = AcquireBrokerLock(dir); !errors.Is(err, errAlreadyRunning) {
		t.Fatalf("second broker: %v", err)
	}
	release()

	if err = os.Chmod(dir, 0o770); err != nil {
		t.Fatal(err)
	}
	if _, err = AcquireBrokerLock(dir); err == nil {
		t.Fatal("lock taken in a spool whose root others can write")
	}

	target := filepath.Join(t.TempDir(), "secret")
	// The target looks like a good lock, so only the link itself can give it away.
	if err = os.WriteFile(target, []byte("s"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(target, 0o640); err != nil {
		t.Fatal(err)
	}
	for name, plant := range map[string]func(lock string) error{
		"symlink":   func(lock string) error { return os.Symlink(target, lock) },
		"hard link": func(lock string) error { return os.Link(target, lock) },
		"mode 0600": func(lock string) error { return os.WriteFile(lock, nil, 0o600) },
	} {
		dir = newSpool(t)
		lock := filepath.Join(dir, BrokerLock)
		if err = os.Remove(lock); err != nil {
			t.Fatal(err)
		}
		if err = plant(lock); err != nil {
			t.Fatal(err)
		}
		if _, err = AcquireBrokerLock(dir); err == nil {
			t.Fatalf("%s lock accepted", name)
		}
		if info, statErr := os.Stat(target); statErr != nil || info.Mode().Perm() != 0o640 {
			t.Fatalf("%s: the target changed: %v %v", name, info.Mode(), statErr)
		}
	}
}

// In production the spool root, in/ and out/ must be root's: a spool the broker owns is refused.
func TestVerifySpoolWantsRoot(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: the spool is root's")
	}
	dir := newSpool(t)
	spoolRootUID = 0
	defer func() { spoolRootUID = os.Geteuid() }()
	if err := verifySpool(dir); err == nil || !strings.Contains(err.Error(), "only root writes") {
		t.Fatalf("a spool root owned by the broker passed as root's: %v", err)
	}
}

// A lock outside the spool's group is refused: the bot could not open it to see the broker.
func TestBrokerLockWantsSpoolGroup(t *testing.T) {
	dir := newSpool(t)
	other := -1
	groups, _ := os.Getgroups()
	for _, g := range groups {
		if g != fileGIDOf(t, dir) {
			other = g
			break
		}
	}
	if other < 0 {
		t.Skip("the test user has a single group")
	}
	if err := os.Lchown(filepath.Join(dir, BrokerLock), -1, other); err != nil {
		t.Fatal(err)
	}
	if _, err := AcquireBrokerLock(dir); err == nil {
		t.Fatal("lock in another group accepted")
	}

	dir = newSpool(t)
	out := filepath.Join(dir, spoolOut)
	if err := os.Lchown(out, -1, other); err != nil {
		t.Fatal(err)
	}
	// chown clears setgid; put it back so only the group differs.
	if err := os.Chmod(out, 0o770|os.ModeSetgid); err != nil {
		t.Fatal(err)
	}
	if err := verifySpool(dir); err == nil {
		t.Fatal("out/ in another group accepted")
	}
}

func fileGIDOf(t *testing.T, path string) int {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}

	return fileGID(info)
}

// Redaction keeps job paths, build URLs and resource names readable and still hides tokens.
func TestRedactSecrets(t *testing.T) {
	for _, keep := range []string{
		"distros_qa/rke2-tests/rke2_validate_cluster_qainfra",
		"https://mower.jenkins.qa.rancher.space/job/distros_qa/job/rke2_validate_cluster_arm_qainfra/42/",
		"pod/rke2-canal-node-ip-172-31-44-171.us-east-2.compute.internal",
	} {
		if got := redactSecrets("see " + keep); got != "see "+keep {
			t.Fatalf("redacted a readable name: %q", got)
		}
	}
	for _, secret := range []string{
		"11d3f9e8a7b6c5d4e3f2a1b0c9d8e7f6a5",                                            // jenkins API token (hex)
		"wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",                                      // AWS secret key
		"ghp_R3aLlyL0ngT0kenValue1234567890abcd",                                        // gitHub token
		"eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n", // JWT
	} {
		if got := redactSecrets("got " + secret + " end"); got != "got [redacted] end" {
			t.Fatalf("not redacted: %q", got)
		}
	}
}

// The MCP config (jenkins credential) is accepted only as the broker's own 0600 file in its own
// 0700 directory; a readable directory, a readable file or a link is refused.
func TestVerifyPrivate(t *testing.T) {
	newPrivate := func() string {
		dir := filepath.Join(t.TempDir(), "private")
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		p := filepath.Join(dir, "mcp.json")
		if err := os.WriteFile(p, []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}

		return p
	}
	if err := VerifyPrivate(newPrivate()); err != nil {
		t.Fatalf("good config: %v", err)
	}
	for name, change := range map[string]func(p string) error{
		"readable directory": func(p string) error { return os.Chmod(filepath.Dir(p), 0o750) },
		"readable file":      func(p string) error { return os.Chmod(p, 0o640) },
		"unreadable file":    func(p string) error { return os.Chmod(p, 0o200) },
		"linked file": func(p string) error {
			target := filepath.Join(t.TempDir(), "real.json")
			if err := os.WriteFile(target, []byte("{}"), 0o600); err != nil {
				return err
			}
			if err := os.Remove(p); err != nil {
				return err
			}

			return os.Symlink(target, p)
		},
	} {
		p := newPrivate()
		if err := change(p); err != nil {
			t.Fatal(err)
		}
		if err := VerifyPrivate(p); err == nil {
			t.Fatalf("%s accepted", name)
		}
	}
}

// A quick triage has its own, shorter limit, so a queue of them fits the bot's wait.
func TestQuickTriageTimeout(t *testing.T) {
	root := t.TempDir()
	dir, manifest := writeSkill(t, root)
	fake := filepath.Join(root, "claude-sandbox")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\nexec sleep 30\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	ct := &ClaudeTriage{
		Command: fake, Workspace: root, SkillDir: dir, Manifest: manifest,
		Timeout: time.Minute, QuickTimeout: 200 * time.Millisecond,
		MCPConfig: filepath.Join(root, "private", "mcp.json"), StopContainer: func(int) {},
	}
	start := time.Now()
	v, err := ct.Triage(context.Background(), &TriageRequest{Mode: TriageQuick})
	if time.Since(start) > 10*time.Second || (err == nil && (v == nil || v.Error == "")) {
		t.Fatalf("quick triage ran %s: %+v %v", time.Since(start), v, err)
	}
}

// A wait the bot ends itself (stop, shutdown) says so, instead of "no verdict within 1h".
func TestCanceledWaitSaysCanceled(t *testing.T) {
	spool := newSpool(t)
	holdBrokerLock(t, spool) // a broker "runs" but never answers
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	c := &BrokerClient{Dir: spool, Timeout: time.Hour, Poll: time.Millisecond}
	if d, _ := c.Ask(ctx, quickRequest()); !strings.Contains(d.Summary, "triage canceled") {
		t.Fatalf("summary %q", d.Summary)
	}
}

// A broker stopped mid-triage leaves the request in work/ (no verdict); the next broker answers it.
func TestBrokerStopRequeuesTriage(t *testing.T) {
	spool := newSpool(t)
	claimed := make(chan struct{})
	held := func(ctx context.Context, _ *TriageRequest) (*Verdict, error) {
		close(claimed)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	first := &Broker{Spool: spool, Poll: time.Millisecond, Triage: held}
	req := quickRequest()
	req.ID = "r1"
	if err := writeSpoolFile(filepath.Join(spool, spoolIn), req.ID, req); err != nil {
		t.Fatal(err)
	}
	ctx, stop := context.WithCancel(context.Background())
	served := make(chan struct{})
	go func() { _ = first.Serve(ctx); close(served) }()
	<-claimed
	stop()
	<-served
	if _, err := os.Stat(filepath.Join(spool, spoolWork, "r1.json")); err != nil {
		t.Fatalf("request not kept for the next broker: %v", err)
	}
	if _, err := os.Stat(filepath.Join(spool, spoolOut, "r1.json")); err == nil {
		t.Fatal("a stopping broker answered the request")
	}

	next := &Broker{Spool: spool, Poll: time.Millisecond, Triage: func(context.Context, *TriageRequest) (*Verdict, error) {
		return &Verdict{Action: "ask", Bucket: "PRODUCT", Confidence: 90, Summary: "s"}, nil
	}}
	ctx2, stop2 := context.WithCancel(context.Background())
	defer stop2()
	go func() { _ = next.Serve(ctx2) }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(spool, spoolOut, "r1.json")); err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("the next broker never answered the requeued request")
		}
		time.Sleep(time.Millisecond)
	}
}

// The model's summary reaches Slack as plain text: no mention, channel ping or disguised link.
func TestDecisionEscapesSlackMarkup(t *testing.T) {
	v := Verdict{
		Action: "ask", Bucket: "PRODUCT", Confidence: 90,
		Summary: "<!here> ask <@U123> in <#C1> or see <https://evil.example|the docs> & more",
	}
	got := v.Decision().Summary
	want := "PRODUCT, 90% confidence: &lt;!here&gt; ask &lt;@U123&gt; in &lt;#C1&gt; or see " +
		"&lt;https://evil.example|the docs&gt; &amp; more"
	if got != want {
		t.Fatalf("summary %q", got)
	}
	if e := (&Verdict{Error: "claude: <!channel>"}).Decision().Summary; strings.Contains(e, "<!channel>") {
		t.Fatalf("error %q", e)
	}
}

// A stopped broker claims no more requests: they stay in in/ for the next one.
func TestStoppedBrokerClaimsNothing(t *testing.T) {
	spool := newSpool(t)
	for _, id := range []string{"a", "b"} {
		req := quickRequest()
		req.ID = id
		if err := writeSpoolFile(filepath.Join(spool, spoolIn), id, req); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	b := &Broker{Spool: spool, Poll: time.Millisecond, Triage: func(context.Context, *TriageRequest) (*Verdict, error) {
		calls++
		cancel() // the broker is told to stop during its first triage
		return &Verdict{Action: "ask", Bucket: "PRODUCT", Confidence: 90, Summary: "s"}, nil
	}}
	_ = b.Serve(ctx)
	left, _ := os.ReadDir(filepath.Join(spool, spoolIn))
	if calls != 1 || len(left) != 1 {
		t.Fatalf("triages %d, left in in/ %d; want 1 and 1", calls, len(left))
	}
}

// A broker starting while the bot probes the lock (shared, for an instant) still starts.
func TestBrokerLockWaitsOutAProbe(t *testing.T) {
	dir := newSpool(t)
	probe, err := os.Open(filepath.Join(dir, BrokerLock))
	if err != nil {
		t.Fatal(err)
	}
	defer probe.Close()
	if err = syscall.Flock(int(probe.Fd()), syscall.LOCK_SH); err != nil {
		t.Fatal(err)
	}
	fd := int(probe.Fd())
	unlocked := make(chan struct{})
	go func() {
		defer close(unlocked)
		time.Sleep(300 * time.Millisecond)
		_ = syscall.Flock(fd, syscall.LOCK_UN)
	}()
	release, err := AcquireBrokerLock(dir)
	<-unlocked // the probe file is closed (deferred) only after the goroutine is done with it
	if err != nil {
		t.Fatalf("broker refused because of a probe: %v", err)
	}
	release()
}

// From a credential label on, the rest of the line goes, whatever separator, bridge or quoting
// follows; only all-prose rests and Kubernetes Secret statuses stay, and other lines are untouched.
func TestRedactRestOfCredentialLine(t *testing.T) {
	//nolint:gosec // fake credentials and resource names
	for in, want := range map[string]string{
		"password for admin is correcthorsebattery":                   "password [redacted]",
		"password => correcthorsebattery":                             "password => [redacted]",
		"password for admin: correcthorsebattery":                     "password [redacted]",
		`password="abc\"def ghi" next`:                                `password=[redacted]`,
		`external_db_password = "DB_SENTINEL";`:                       `external_db_password = [redacted]`,
		"AWS access key ID: AKIAIOSFODNN7EXAMPLE":                     "AWS access key ID: [redacted]",
		"aws_access_key_id=AKIAIOSFODNN7EXAMPLE ok":                   "aws_access_key_id=[redacted]",
		"api key: alphabeticsecret":                                   "api key: [redacted]",
		"Authorization: Bearer abc def":                               "Authorization: [redacted]",
		"-e SLACK_TOKEN=xoxb-short -e X=1":                            "-e SLACK_TOKEN=[redacted]",
		`{"password": 'two words', "user": "qa"}`:                     `{"password": [redacted]`,
		"the token expired.":                                          "the token expired.",
		"password is invalid":                                         "password is invalid",
		"E2E password rotation test passed":                           "E2E password rotation test passed",
		"password: invalid":                                           "password: [redacted]",
		`Error: secret "rke2-serving" not found`:                      `Error: secret "rke2-serving" not found`,
		`secret "x" not found; password=hunter2`:                      `secret "x" not found; password=[redacted]`,
		"level=error msg=\"node not ready\"":                          "level=error msg=\"node not ready\"",
		"token: a\nnext: line":                                        "token: [redacted]\nnext: line",
		`password=before secret "rke2-serving" not found aftersecret`: "password=[redacted]",
		"DB__PASSWORD__VALUE=hunter2":                                 "DB__PASSWORD__VALUE=[redacted]",
		"PASSWORD__B64=aHVudGVyMg==":                                  "PASSWORD__B64=[redacted]",
		"clientSecretKey=abc123":                                      "clientSecretKey=[redacted]",
		"clientAPIKeyValue=abc123":                                    "clientAPIKeyValue=[redacted]",
		"APIKeyValue: alphabetic":                                     "APIKeyValue: [redacted]",
		`{"APIKeyValue":"alphabetic"}`:                                `{"APIKeyValue":[redacted]`,
		"APIKEYValue=abc":                                             "APIKEYValue=[redacted]",
		"AccessKeyID: AKIAEXAMPLE":                                    "AccessKeyID: [redacted]",
		"apikeyrotation finished ok":                                  "apikeyrotation finished ok",
		"dbPasswordHash: p4ssw0rd":                                    "dbPasswordHash: [redacted]",
		`{"apiKeyValue":"alphabetic"}`:                                `{"apiKeyValue":[redacted]`,
		"password rotation spec failed in BeforeSuite":                "password rotation spec failed in BeforeSuite",
		"clientSecretKey abc123":                                      "clientSecretKey [redacted]",
		"dbPasswordHash is p4ssw0rd":                                  "dbPasswordHash [redacted]",
		"the rotateSecretKey step failed":                             "the rotateSecretKey [redacted]",
		"dbpassword=hunter2":                                          "dbpassword=[redacted]",
		"password_plaintext=hunter2":                                  "password_plaintext=[redacted]",
		"client_secret_key=abc123":                                    "client_secret_key=[redacted]",
		"db_password_hash=p4ssw0rd":                                   "db_password_hash=[redacted]",
		"REGISTRY_PASSWORD_B64: aHVudGVyMg==":                         "REGISTRY_PASSWORD_B64: [redacted]",
		"secretsencryption suite failed":                              "secretsencryption suite failed",
	} {
		if got := redactSecrets(in); got != want {
			t.Fatalf("%q: got %q, want %q", in, got, want)
		}
	}
}

// A Go test name keeps a test result after it ("TestX: failed in BeforeSuite") or nothing; any
// assignment or other value after one with a credential word goes.
func TestRedactTestNames(t *testing.T) {
	//nolint:gosec // fake credentials
	for in, want := range map[string]string{
		"Test_E2ESecretsReload timed out":              "Test_E2ESecretsReload timed out",
		"Test_E2ESecretsEncryption failed":             "Test_E2ESecretsEncryption failed",
		"TestSecretsEncryption failed in BeforeSuite":  "TestSecretsEncryption failed in BeforeSuite",
		"TestSecretsEncryption: failed in BeforeSuite": "TestSecretsEncryption: failed in BeforeSuite",
		"TestPasswordRotation: failed":                 "TestPasswordRotation: failed",
		"TestClientSecretKey=abc123":                   "TestClientSecretKey=[redacted]",
		"TestClientSecretKey=invalid":                  "TestClientSecretKey=[redacted]",
		"TestClientSecretKey: invalid":                 "TestClientSecretKey: [redacted]",
		"TestApiKeyValue: missing":                     "TestApiKeyValue: [redacted]",
		"TestPasswordHash: empty":                      "TestPasswordHash: [redacted]",
		`"TestPasswordHash": expired`:                  `"TestPasswordHash": [redacted]`,
		"config TestApiKeyValue => empty":              "config TestApiKeyValue => [redacted]",
		"TestTokenRefresh -> expired":                  "TestTokenRefresh -> [redacted]",
		`{"TestPasswordHash": "missing"}`:              `{"TestPasswordHash": [redacted]`,
		`{"TestPasswordHash":"p4ssw0rd"}`:              `{"TestPasswordHash":[redacted]`,
		"config TestApiKeyValue => alphabetic":         "config TestApiKeyValue => [redacted]",
		"TestSecretsEncryption":                        "TestSecretsEncryption",
	} {
		if got := redactSecrets(in); got != want {
			t.Fatalf("%q: got %q, want %q", in, got, want)
		}
	}
}

func TestNewRequestIDMatchesScriptCharset(t *testing.T) {
	id := newRequestID(testNow)
	if !regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`).MatchString(id) || !strings.HasPrefix(id, "rb-20260928T120000-") {
		t.Fatal(id)
	}
	if id == newRequestID(testNow) {
		t.Fatal("ids must differ within the same second")
	}
}

func TestMoveRequestDistinguishesWithdrawalFromFailure(t *testing.T) {
	for _, present := range []bool{false, true} {
		t.Run(map[bool]string{false: "withdrawn", true: "destination missing"}[present], func(t *testing.T) {
			dir := t.TempDir()
			from := filepath.Join(dir, "request.json")
			if present {
				if err := os.WriteFile(from, []byte("request"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			moved, err := moveRequest(from, filepath.Join(dir, "missing", "request.json"))
			if moved || (err != nil) != present {
				t.Fatalf("moved=%v err=%v, source present=%v", moved, err, present)
			}
			if present {
				if _, err = os.Stat(from); err != nil {
					t.Fatalf("request lost after failed move: %v", err)
				}
			}
		})
	}
}

func obstructRequest(t *testing.T, spool, dir, id string) {
	t.Helper()
	path := filepath.Join(spool, dir, id+".json")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "keep"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestBrokerClaimReportsMoveFailure(t *testing.T) {
	spool := newSpool(t)
	if err := writeSpoolFile(filepath.Join(spool, spoolIn), "blocked", quickRequest()); err != nil {
		t.Fatal(err)
	}
	obstructRequest(t, spool, spoolWork, "blocked")
	id, err := (&Broker{Spool: spool}).claim()
	if err == nil || id != "" {
		t.Fatalf("failed claim reported as empty queue: id=%q err=%v", id, err)
	}
	if _, err = os.Stat(filepath.Join(spool, spoolIn, "blocked.json")); err != nil {
		t.Fatalf("queued request lost: %v", err)
	}
}

func TestBrokerRecoveryReportsFailureAndRecoversOtherRequests(t *testing.T) {
	spool := newSpool(t)
	for _, id := range []string{"blocked", "healthy"} {
		if err := writeSpoolFile(filepath.Join(spool, spoolWork), id, quickRequest()); err != nil {
			t.Fatal(err)
		}
	}
	obstructRequest(t, spool, spoolIn, "blocked")
	b := &Broker{Spool: spool}
	if err := b.requeue(); err == nil {
		t.Fatal("recovery silently ignored the blocked move")
	}
	if _, err := os.Stat(filepath.Join(spool, spoolWork, "blocked.json")); err != nil {
		t.Fatalf("unfinished request lost: %v", err)
	}
	if _, err := os.Stat(filepath.Join(spool, spoolIn, "healthy.json")); err != nil {
		t.Fatalf("healthy request not recovered: %v", err)
	}
	if err := b.Serve(testContext(t)); err == nil || errors.Is(err, os.ErrNotExist) {
		t.Fatalf("startup did not report the failed recovery: %v", err)
	}
}

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
	log, err := (&jenkins{BaseURL: srv.URL, HTTP: srv.Client()}).ConsoleText(context.Background(), srv.URL+"/job/j/7/")
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
