package resources

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// Guards the CommandContext/Cancel pairing: exec.Cmd rejects a non-nil Cancel
// on commands not created with CommandContext, which would break every call.
func TestRunHostArgsExecutes(t *testing.T) {
	out, err := RunHostArgs("echo", "hello", "argv world")
	if err != nil {
		t.Fatalf("RunHostArgs failed: %v", err)
	}
	if !strings.Contains(out, "hello argv world") {
		t.Errorf("unexpected output %q", out)
	}
}

func TestRunHostArgsContextTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	start := time.Now()
	_, err := RunHostArgsContext(ctx, "sleep", "60")
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if elapsed := time.Since(start); elapsed > 15*time.Second {
		t.Errorf("command outlived context by too much: %s", elapsed)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("errors.Is(err, context.DeadlineExceeded) must hold, got: %v", err)
	}
}

// A spawned child must die with the group — killing only bash would leave it
// alive holding the pipes. The child PID is printed to stderr (returned on
// error) and probed with kill(pid, 0) until ESRCH.
func TestRunCommandHostWithTimeoutKillsProcessGroup(t *testing.T) {
	start := time.Now()
	errOut, err := RunCommandHostWithTimeout(2*time.Second,
		"sleep 60 & echo CHILD=$! 1>&2; wait")
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("errors.Is(err, context.DeadlineExceeded) must hold, got: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 15*time.Second {
		t.Errorf("Run blocked past deadline+WaitDelay: %s", elapsed)
	}

	m := regexp.MustCompile(`CHILD=(\d+)`).FindStringSubmatch(errOut)
	if m == nil {
		t.Fatalf("child pid not found in stderr output %q", errOut)
	}
	pid, _ := strconv.Atoi(m[1])

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if killErr := syscall.Kill(pid, 0); errors.Is(killErr, syscall.ESRCH) {
			return // child is gone — group kill worked
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("child pid %d still alive after group kill", pid)
}

func TestRunCommandHostNoDeadline(t *testing.T) {
	out, err := RunCommandHost("echo unbounded")
	if err != nil || !strings.Contains(out, "unbounded") {
		t.Fatalf("RunCommandHost failed: %v %q", err, out)
	}
}
