package testcase

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestIsSessionKilledByUninstall(t *testing.T) {
	// The matched substring comes from crypto/ssh, so it is product-agnostic.
	rke2Killed := errors.New("failed to run command: /usr/bin/rke2-uninstall.sh, error: " +
		"command: sudo /usr/bin/rke2-uninstall.sh failed on run ssh: 3.138.103.224 with error: " +
		"wait: remote command exited without exit status or exit signal uninstall failed for rke2-server")
	k3sKilled := errors.New("failed to run command: /usr/local/bin/k3s-uninstall.sh, error: " +
		"command: sudo /usr/local/bin/k3s-uninstall.sh failed on run ssh: 3.21.28.248 with error: " +
		"wait: remote command exited without exit status or exit signal uninstall failed for k3s-server")
	for _, killed := range []error{rke2Killed, k3sKilled} {
		if !isSessionKilledByUninstall(killed) {
			t.Fatalf("expected session-killed error to match: %v", killed)
		}
	}
	if isSessionKilledByUninstall(errors.New("exit status 1")) {
		t.Fatal("plain failure must not match")
	}
	if isSessionKilledByUninstall(nil) {
		t.Fatal("nil must not match")
	}
}

func TestIsUninstallScriptMissing(t *testing.T) {
	absent := errors.New("failed to find rke2-uninstall.sh script for rke2: " +
		"path for rke2-uninstall.sh not found")
	if !isUninstallScriptMissing(absent) {
		t.Fatal("genuine script absence must match")
	}

	// Connection failures share the "failed to find ... script" wrapper but
	// must NOT be classified as already-uninstalled (build #12 regression).
	connRefused := errors.New("failed to find rke2-uninstall.sh script for rke2: " +
		"failed to get common paths: dial tcp 3.138.103.224:22: connect: connection refused")
	if isUninstallScriptMissing(connRefused) {
		t.Fatal("connection failure must not be classified as script missing")
	}
	if isUninstallScriptMissing(nil) {
		t.Fatal("nil must not match")
	}
}

type verifyStep struct {
	res string
	err error
}

func runVerifySteps(t *testing.T, steps []verifyStep) (calls, sleeps int, err error) {
	t.Helper()
	run := func(_, _ string) (string, error) {
		step := steps[len(steps)-1]
		if calls < len(steps) {
			step = steps[calls]
		}
		calls++

		return step.res, step.err
	}
	sleep := func(time.Duration) { sleeps++ }
	err = waitUninstallPolicyRemoved(run, sleep, "rke2", "1.2.3.4", "rpm -qa rke2-selinux")

	return calls, sleeps, err
}

func TestWaitUninstallSSHRecovers(t *testing.T) {
	calls, sleeps, err := runVerifySteps(t, []verifyStep{
		{"", errors.New("dial tcp: connection refused")},
		{"", errors.New("dial tcp: connection refused")},
		{"", nil},
	})
	if err != nil {
		t.Fatalf("expected success after ssh recovery, got: %v", err)
	}
	if calls != 3 || sleeps != 2 {
		t.Fatalf("expected 3 calls / 2 sleeps, got %d / %d", calls, sleeps)
	}
}

func TestWaitUninstallRPMDrains(t *testing.T) {
	_, sleeps, err := runVerifySteps(t, []verifyStep{
		{"rke2-selinux-0.24-12.slemicro\n", nil},
		{"rke2-selinux-0.24-12.slemicro\n", nil},
		{"  \n", nil},
	})
	if err != nil {
		t.Fatalf("expected success once rpm output drains, got: %v", err)
	}
	if sleeps != 2 {
		t.Fatalf("expected 2 sleeps, got %d", sleeps)
	}
}

func TestWaitUninstallRPMStuck(t *testing.T) {
	calls, sleeps, err := runVerifySteps(t, []verifyStep{
		{"rke2-selinux-0.24-12.slemicro", nil},
	})
	if err == nil || !strings.Contains(err.Error(), "rke2-selinux-0.24-12.slemicro") {
		t.Fatalf("expected error naming the leftover package, got: %v", err)
	}
	if calls != uninstallVerifyAttempts {
		t.Fatalf("expected %d attempts, got %d", uninstallVerifyAttempts, calls)
	}
	if sleeps != uninstallVerifyAttempts-1 {
		t.Fatalf("expected no sleep after the last attempt (%d sleeps), got %d",
			uninstallVerifyAttempts-1, sleeps)
	}
}

func TestWaitUninstallPermanentSSHError(t *testing.T) {
	permanent := errors.New("dial tcp: connection refused")
	_, sleeps, err := runVerifySteps(t, []verifyStep{{"", permanent}})
	if err == nil || !errors.Is(err, permanent) {
		t.Fatalf("expected wrapped permanent ssh error, got: %v", err)
	}
	if sleeps != uninstallVerifyAttempts-1 {
		t.Fatalf("expected no sleep after the last attempt, got %d sleeps", sleeps)
	}
}
