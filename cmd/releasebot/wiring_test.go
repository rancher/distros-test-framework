package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rancher/distros-test-framework/internal/pkg/releasebot"
)

func TestWiringAppliesDTFRefEnvironment(t *testing.T) {
	t.Setenv(releasebot.DTFRefEnv, "env-ref")
	path := filepath.Join(t.TempDir(), "matrix.yaml")
	raw := []byte("dtfRef: matrix-ref\ndefaultParams:\n  BRANCH: '{{DTF_REF}}'\n")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	app := newApp(&options{matrixPath: path, skipTagCheck: true})
	p, err := app.Prepare(t.Context(), releasebot.Request{RKE2: []string{"v1.37.1-rc2+rke2r1"}})
	if err != nil {
		t.Fatal(err)
	}
	if p.Matrix.DTFRef != "env-ref" || p.Matrix.Defaults["BRANCH"] != "env-ref" {
		t.Fatalf("override not applied: %+v", p.Matrix)
	}
}

// A real CLI run takes the listener's lock, so it never schedules next to the Slack bot (both would
// fill a controller's maxConcurrent); a dry-run triggers nothing and needs no lock.
func TestRunRefusesWhileAnotherBotRuns(t *testing.T) {
	state := filepath.Join(t.TempDir(), "state.json")
	release, err := releasebot.AcquireInstanceLock(state + ".lock")
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	o := &options{message: "v1.37.1-rc1+rke2r1", stateFile: state, matrixPath: "missing.yaml"}
	runErr := run(o)
	if runErr == nil || !strings.Contains(runErr.Error(), "already running") {
		t.Fatalf("execute next to a running bot: expected the lock error, got %v", runErr)
	}

	o.dryRun = true
	if dryErr := run(o); dryErr == nil || strings.Contains(dryErr.Error(), "already running") {
		t.Fatalf("dry-run must not take the lock (it fails on the missing matrix instead), got %v", dryErr)
	}
}

// A stopped Slack bot keeps its unfinished runs to resume, with their builds possibly still on
// Jenkins: a real CLI run refuses until they end, while a dry-run (it triggers nothing) still plans.
func TestRunRefusesWhileSlackRunsAreSaved(t *testing.T) {
	state := filepath.Join(t.TempDir(), "state.json")
	saved := []byte(`{"version":1,"runs":{"rb-1":{"channel":"C1","ts":"1.2","tags":["v1.37.1-rc1+rke2r1"]}}}`)
	if err := os.WriteFile(state, saved, 0o600); err != nil {
		t.Fatal(err)
	}

	o := &options{message: "v1.37.1-rc1+rke2r1", stateFile: state, matrixPath: "missing.yaml"}
	runErr := run(o)
	if runErr == nil || !strings.Contains(runErr.Error(), "1 unfinished runs") {
		t.Fatalf("execute next to saved Slack runs: expected a refusal, got %v", runErr)
	}

	o.dryRun = true
	if dryErr := run(o); dryErr == nil || strings.Contains(dryErr.Error(), "unfinished") {
		t.Fatalf("dry-run must still plan (it fails on the missing matrix instead), got %v", dryErr)
	}

	// The refusal releases the lock: once the runs end, the next request may run.
	if err := os.WriteFile(state, []byte(`{"version":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	o.dryRun = false
	if err := run(o); err == nil || strings.Contains(err.Error(), "unfinished") ||
		strings.Contains(err.Error(), "already running") {
		t.Fatalf("no saved runs: expected the matrix error, got %v", err)
	}
}

// A CLI run saved by an interrupted execution may still have builds on jenkins: a new execution refuses
// (it would trigger them again) and says how to carry it on; -resume needs it, and is never a dry-run.
func TestRunRefusesNewExecutionWhileACLIRunIsSaved(t *testing.T) {
	state := filepath.Join(t.TempDir(), "state.json")
	o := &options{resume: true, stateFile: state, matrixPath: "missing.yaml"}
	if err := run(o); err == nil || !strings.Contains(err.Error(), "no saved run") {
		t.Fatalf("-resume without a saved run: %v", err)
	}

	raw := []byte(`{"version":1,"request":"v1.37.1-rc1+rke2r1","progress":{"request_id":"rb-1"}}`)
	if err := os.WriteFile(cliStatePath(state), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	o = &options{message: "v1.37.1-rc1+rke2r1", stateFile: state, matrixPath: "missing.yaml"}
	if err := run(o); err == nil || !strings.Contains(err.Error(), "carry it on with -resume") {
		t.Fatalf("a new execution next to a saved CLI run: %v", err)
	}

	o = &options{resume: true, dryRun: true, stateFile: state, matrixPath: "missing.yaml"}
	if err := run(o); err == nil || !strings.Contains(err.Error(), "needs -dry-run=false") {
		t.Fatalf("-resume as a dry-run: %v", err)
	}
}

// The Slack bot refuses to execute next to a run an interrupted CLI saved, as the CLI refuses next to
// the bot's saved runs; a dry-run bot triggers nothing and starts.
func TestListenRefusesNextToASavedCLIRun(t *testing.T) {
	t.Setenv("SLACK_BOT_TOKEN", "xoxb-test")
	t.Setenv("SLACK_APP_TOKEN", "xapp-test")
	t.Setenv("RELEASEBOT_CHANNELS", "C1")
	state := filepath.Join(t.TempDir(), "state.json")
	raw := []byte(`{"version":1,"request":"v1.37.1-rc1+rke2r1","progress":{"request_id":"rb-1","stage":"jobs"}}`)
	if err := os.WriteFile(cliStatePath(state), raw, 0o600); err != nil {
		t.Fatal(err)
	}

	err := runListen(&options{stateFile: state, matrixPath: "missing.yaml"})
	if err == nil || !strings.Contains(err.Error(), "not serving: the CLI run") {
		t.Fatalf("listen next to a saved CLI run: %v", err)
	}
	if dryErr := refuseNextToCLIRun(&options{dryRun: true, stateFile: state}); dryErr != nil {
		t.Fatalf("a dry-run bot was refused: %v", dryErr)
	}
}

// /dev/null is a character device, not a person: commands are read from a real terminal only.
func TestIsTerminalRefusesDevNull(t *testing.T) {
	devNull, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer devNull.Close()

	if isTerminal(devNull) {
		t.Fatal("/dev/null taken for a terminal")
	}
}

// A saved CLI run and a saved bump run block each other and the Slack bot (their builds may still
// run); each mode still carries on its own with -resume.
func TestModesRefuseEachOthersSavedRun(t *testing.T) {
	dir := t.TempDir()
	state := filepath.Join(dir, "state.json")
	matrix := filepath.Join(dir, "matrix.yaml")
	bumps := filepath.Join(dir, "bumps.yaml")
	controllers := []byte("controllers:\n  mower:\n    url: https://m\n    maxConcurrent: 6\n")
	if err := os.WriteFile(matrix, controllers, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bumps, []byte("jobs: {k3s: k, rke2: r}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	saved := []byte(`{"version":1,"request":"bumps 2026-11","progress":{"request_id":"rb-1","stage":"jobs"}}`)

	if err := os.WriteFile(bumpsStatePath(state), saved, 0o600); err != nil {
		t.Fatal(err)
	}
	cli := &options{message: "v1.37.1-rc1+rke2r1", stateFile: state, matrixPath: "missing.yaml"}
	if err := run(cli); err == nil || !strings.Contains(err.Error(), "the bump run") {
		t.Fatalf("a CLI run next to a saved bump run: %v", err)
	}
	if err := refuseNextToCLIRun(&options{stateFile: state}); err == nil || !strings.Contains(err.Error(), "the bump run") {
		t.Fatalf("the Slack bot next to a saved bump run: %v", err)
	}

	if err := os.Rename(bumpsStatePath(state), cliStatePath(state)); err != nil {
		t.Fatal(err)
	}
	bump := &options{bumps: true, bumpsConfig: bumps, matrixPath: matrix, stateFile: state}
	if err := runBumps(bump); err == nil || !strings.Contains(err.Error(), "the CLI run") {
		t.Fatalf("a bump run next to a saved CLI run: %v", err)
	}
	if err := refuseSavedTerminalRuns(state, cliStatePath(state)); err != nil {
		t.Fatalf("the CLI's own saved run blocks its -resume: %v", err)
	}
}
