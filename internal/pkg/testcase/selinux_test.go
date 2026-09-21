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

// recordingRunner scripts the rpm probe and the rpm -qa reply separately and records
// every step, so a test can assert which of them actually ran.
type recordingRunner struct {
	hasRPM   bool
	probeErr error
	rpmOut   string
	rpmErr   error
	cmds     []string
}

func (r *recordingRunner) probe(_ string) (bool, error) {
	r.cmds = append(r.cmds, "probe")

	return r.hasRPM, r.probeErr
}

func (r *recordingRunner) run(cmd, _ string) (string, error) {
	r.cmds = append(r.cmds, cmd)

	return r.rpmOut, r.rpmErr
}

func (r *recordingRunner) rpmQueries() int {
	n := 0
	for _, c := range r.cmds {
		if strings.HasPrefix(c, "rpm -qa") {
			n++
		}
	}

	return n
}

func TestCheckUninstallPolicyFlow(t *testing.T) {
	const rpmCmd = "rpm -qa k3s-selinux"
	noSleep := func(time.Duration) {}

	t.Run("rpm absent: probe only, rpm -qa never runs, no error", func(t *testing.T) {
		r := &recordingRunner{hasRPM: false, rpmErr: errors.New("rpm -qa must not run")}
		if err := checkUninstallPolicy(r.probe, r.run, noSleep, "k3s", "1.2.3.4", rpmCmd); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(r.cmds) != 1 || r.cmds[0] != "probe" {
			t.Fatalf("expected exactly the probe, got %q", r.cmds)
		}
		if r.rpmQueries() != 0 {
			t.Fatalf("rpm -qa must not run on a node without rpm, got %q", r.cmds)
		}
	})

	t.Run("rpm present: probe then verification, package gone", func(t *testing.T) {
		r := &recordingRunner{hasRPM: true, rpmOut: ""}
		if err := checkUninstallPolicy(r.probe, r.run, noSleep, "k3s", "1.2.3.4", rpmCmd); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if r.rpmQueries() != 1 || r.cmds[len(r.cmds)-1] != rpmCmd {
			t.Fatalf("expected the probe followed by %q, got %q", rpmCmd, r.cmds)
		}
	})

	t.Run("rpm present: package still installed fails", func(t *testing.T) {
		r := &recordingRunner{hasRPM: true, rpmOut: "k3s-selinux-1.6-1.sle.noarch\n"}
		err := checkUninstallPolicy(r.probe, r.run, noSleep, "k3s", "1.2.3.4", rpmCmd)
		if err == nil || !strings.Contains(err.Error(), "still installed") {
			t.Fatalf("expected still-installed error, got %v", err)
		}
		if r.rpmQueries() != uninstallVerifyAttempts {
			t.Fatalf("expected %d rpm -qa attempts, got %d", uninstallVerifyAttempts, r.rpmQueries())
		}
	})

	t.Run("probe SSH error: fails, verification not attempted", func(t *testing.T) {
		r := &recordingRunner{probeErr: errors.New("dial tcp 1.2.3.4:22: connection refused")}
		err := checkUninstallPolicy(r.probe, r.run, noSleep, "k3s", "1.2.3.4", rpmCmd)
		if err == nil || !strings.Contains(err.Error(), "connection refused") {
			t.Fatalf("expected the probe error, got %v", err)
		}
		if r.rpmQueries() != 0 {
			t.Fatalf("verification must not run after a probe error, got %q", r.cmds)
		}
	})
}

func TestK3sSles16ServerLogsOptional(t *testing.T) {
	// server/logs only exists with audit logging configured; the default hardened jobs do not set it.
	selectSelinuxPolicy("k3s", "sles16")
	if osPolicyRequired[rootLs(k3s+"/server/logs "+ignoreDir)] {
		t.Fatal("server/logs must not be a required path")
	}
	if !osPolicyRequired[rootLs(k3s+"/server/tls "+ignoreDir)] {
		t.Fatal("server/tls must stay required")
	}
}

func TestK3sSles16PolicyExists(t *testing.T) {
	// getContext maps SLES 16 to "sles16"; a missing k3s entry made TestSelinuxContext pass vacuously.
	if selectSelinuxPolicy("k3s", "sles16") == nil {
		t.Fatal("k3s_sles16 context table is missing")
	}
	for cmd := range selectSelinuxPolicy("k3s", "sles16") {
		if !osPolicyRequired[cmd] && !sles16Optional[cmd] {
			t.Fatalf("every sles16 command must be required unless listed optional, %q is not", cmd)
		}
		if !strings.HasPrefix(cmd, "sudo sh -c 'ls -laZ ") {
			t.Fatalf("%q must run in a root shell so globs under root-only dirs expand", cmd)
		}
		if strings.Contains(cmd, "s?bin") {
			t.Fatalf("glob %q never matches /usr/local/bin/k3s", cmd)
		}
	}
}
