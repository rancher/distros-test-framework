package testcase

import (
	"errors"
	"fmt"
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

type needsRebootCase struct {
	name          string
	transactional bool
	probeErr      error
	rpmOut        string
	rpmErr        error
	want          bool
	wantQueries   int
	wantErr       string
}

func needsRebootCases() []needsRebootCase {
	probeErr := errors.New("dial tcp 1.2.3.4:22: connection refused")

	return []needsRebootCase{
		{
			name: "plain rpm host: no reboot, rpm not queried", transactional: false,
			rpmOut: "rke2-server-1.34.11", wantQueries: 0,
		},
		{
			name: "transactional with packages installed: reboot", transactional: true,
			rpmOut: "rke2-server-1.34.11\n", want: true, wantQueries: 1,
		},
		{
			name: "transactional tarball install: nothing to activate", transactional: true,
			rpmOut: "", wantQueries: 1,
		},
		{name: "probe error fails closed", probeErr: probeErr, wantErr: "connection refused"},
		{
			name: "rpm query error fails closed", transactional: true,
			rpmErr: errors.New("rpm: db locked"), wantQueries: 1, wantErr: "pre-uninstall rpm query",
		},
	}
}

func TestUninstallNeedsReboot(t *testing.T) {
	const rpmCmd = "rpm -qa rke2-server rke2-selinux"

	for _, tc := range needsRebootCases() {
		t.Run(tc.name, func(t *testing.T) {
			queries := 0
			probe := func(string) (bool, error) { return tc.transactional, tc.probeErr }
			run := func(cmd, _ string) (string, error) {
				if cmd != rpmCmd {
					t.Fatalf("unexpected command %q", cmd)
				}
				queries++

				return tc.rpmOut, tc.rpmErr
			}

			got, err := uninstallNeedsReboot(probe, run, "1.2.3.4", rpmCmd)
			if queries != tc.wantQueries {
				t.Fatalf("expected %d rpm queries, got %d", tc.wantQueries, queries)
			}
			if tc.wantErr == "" && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
				t.Fatalf("expected error containing %q, got %v", tc.wantErr, err)
			}
			if got != tc.want {
				t.Fatalf("expected reboot=%v, got %v", tc.want, got)
			}
		})
	}
}

// bootIDRunner answers boot_id reads from a queue so a test can decide whether the node "rebooted".
type bootIDRunner struct {
	bootIDs   []string
	readErr   error
	rebooted  bool
	rebootErr error
}

func (b *bootIDRunner) run(cmd, _ string) (string, error) {
	switch cmd {
	case bootIDCmd:
		if b.readErr != nil {
			return "", b.readErr
		}
		id := b.bootIDs[0]
		if len(b.bootIDs) > 1 {
			b.bootIDs = b.bootIDs[1:]
		}

		return id + "\n", nil
	case "sudo systemctl reboot":
		b.rebooted = true

		return "", b.rebootErr
	default:
		return "", fmt.Errorf("unexpected command %q", cmd)
	}
}

func TestRebootAndVerifyBootID(t *testing.T) {
	noSleep := func(time.Duration) {}
	sshOK := func(string) error { return nil }
	sshTimeout := func(string) error { return errors.New("timed out waiting 5m0s for SSH Ready") }
	sessionDrop := errors.New("wait: remote command exited without exit status")

	t.Run("new boot_id proves the reboot even though the session dropped", func(t *testing.T) {
		r := &bootIDRunner{bootIDs: []string{"aaaa", "bbbb"}, rebootErr: sessionDrop}
		if err := rebootAndVerifyBootID(r.run, sshOK, noSleep, "1.2.3.4"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !r.rebooted {
			t.Fatal("reboot command was not issued")
		}
	})

	t.Run("same boot_id means the reboot never happened", func(t *testing.T) {
		r := &bootIDRunner{bootIDs: []string{"aaaa", "aaaa"}}
		err := rebootAndVerifyBootID(r.run, sshOK, noSleep, "1.2.3.4")
		if err == nil || !strings.Contains(err.Error(), "did not reboot") {
			t.Fatalf("expected did-not-reboot error, got %v", err)
		}
	})

	t.Run("SSH never comes back", func(t *testing.T) {
		r := &bootIDRunner{bootIDs: []string{"aaaa", "bbbb"}}
		err := rebootAndVerifyBootID(r.run, sshTimeout, noSleep, "1.2.3.4")
		if err == nil || !strings.Contains(err.Error(), "reboot after uninstall on 1.2.3.4") {
			t.Fatalf("expected SSH timeout error, got %v", err)
		}
	})

	t.Run("boot_id unreadable before reboot fails without rebooting", func(t *testing.T) {
		r := &bootIDRunner{bootIDs: []string{"aaaa"}, readErr: errors.New("permission denied")}
		err := rebootAndVerifyBootID(r.run, sshOK, noSleep, "1.2.3.4")
		if err == nil || !strings.Contains(err.Error(), "boot_id before reboot") {
			t.Fatalf("expected boot_id read error, got %v", err)
		}
		if r.rebooted {
			t.Fatal("must not reboot when the baseline boot_id is unknown")
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
