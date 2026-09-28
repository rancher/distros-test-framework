package testcase

import (
	"errors"
	"strings"
	"testing"
)

func TestParseRPMVerify(t *testing.T) {
	out := "S.5....T.    /usr/bin/rke2\n" +
		"S.5....T.  c /etc/rancher/rke2/config.yaml\n" +
		"missing     /usr/share/rke2/README\n" +
		".....UG..    /usr/share/rke2/rke2-cis-sysctl.conf\n" +
		"\n"
	got, err := parseRPMVerify(out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := map[string]string{
		"/usr/bin/rke2":                        "S.5....T.",
		"/etc/rancher/rke2/config.yaml":        "S.5....T.",
		"/usr/share/rke2/README":               "missing",
		"/usr/share/rke2/rke2-cis-sysctl.conf": ".....UG..",
	}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("%s: got %q, want %q", k, got[k], v)
		}
	}

	for _, bad := range []string{
		"error: package rke2-common is not installed",
		"sudo: a password is required",
		"prelink: /usr/bin/rke2: not a file",
		"S.5....T.    relative/path",
	} {
		if _, err := parseRPMVerify(bad + "\n"); err == nil {
			t.Fatalf("expected %q to be rejected", bad)
		}
	}
}

type rpmNode struct {
	rpm        bool
	bin        string
	owner      string // "" -> "not owned"
	qfErr      error  // forces an rpm -qf failure (e.g. SSH)
	version    string
	versionErr error
	verify     string
	verifyRC   string
	verifyErr  error
	cmds       []string
}

func (n *rpmNode) hasRPM(string) (bool, error) { return n.rpm, nil }

// run mimics the node helper: a failing plain command returns no stdout, while the
// marker-wrapped probes always exit 0 and carry the inner rc in their output.
func (n *rpmNode) run(cmd, _ string) (string, error) {
	n.cmds = append(n.cmds, cmd)
	switch {
	case strings.HasPrefix(cmd, "readlink -f"):
		return n.bin + "\n", nil
	case strings.HasPrefix(cmd, "rpm -qf"):
		if n.qfErr != nil {
			return "", n.qfErr
		}
		if n.owner == "" {
			return "file " + n.bin + " is not owned by any package\n" + rcMarker + "1\n", nil
		}

		return n.owner + "\n" + rcMarker + "0\n", nil
	case strings.Contains(cmd, "--version"):
		if n.versionErr != nil {
			return "", n.versionErr
		}

		return n.version + "\n", nil
	case strings.HasPrefix(cmd, "sudo rpm -V"):
		if n.verifyErr != nil {
			return "", n.verifyErr
		}
		rc := n.verifyRC
		if rc == "" {
			rc = "0"
		}

		return n.verify + rcMarker + rc + "\n", nil
	}

	return "", errors.New("unexpected command " + cmd)
}

func rpmInstalled() *rpmNode {
	return &rpmNode{
		rpm: true, bin: "/usr/bin/rke2", owner: "rke2-common-1.34.11~rke2r1-0.slemicro.x86_64",
		version: "rke2 version v1.34.11+rke2r1 (2506fee)",
	}
}

func TestCaptureRPMBinaryState(t *testing.T) {
	t.Run("no rpm on the node: nil, nothing else run", func(t *testing.T) {
		n := &rpmNode{rpm: false}
		state, err := captureRPMBinaryState(n.run, n.hasRPM, "rke2", "1.2.3.4")
		if err != nil || state != nil || len(n.cmds) != 0 {
			t.Fatalf("expected nil state and no commands, got %v %v %q", state, err, n.cmds)
		}
	})

	t.Run("tarball install: binary unowned, nil", func(t *testing.T) {
		n := &rpmNode{rpm: true, bin: "/opt/rke2/bin/rke2"}
		state, err := captureRPMBinaryState(n.run, n.hasRPM, "rke2", "1.2.3.4")
		if err != nil || state != nil {
			t.Fatalf("expected nil state for unowned binary, got %v %v", state, err)
		}
	})

	t.Run("rpm install: owner, version and rc=1 verify captured", func(t *testing.T) {
		n := rpmInstalled()
		n.verify, n.verifyRC = "S.5....T.  c /etc/rancher/rke2/config.yaml\n", "1"
		state, err := captureRPMBinaryState(n.run, n.hasRPM, "rke2", "1.2.3.4")
		if err != nil || state == nil {
			t.Fatalf("unexpected: %v %v", state, err)
		}
		if state.Owner != n.owner || state.BinPath != n.bin || !strings.Contains(state.BinVer, "v1.34.11") {
			t.Fatalf("bad state %+v", *state)
		}
		if state.Modified["/etc/rancher/rke2/config.yaml"] != "S.5....T." {
			t.Fatalf("expected the config diff to be captured, got %v", state.Modified)
		}
	})
}

type probeFailureCase struct {
	name    string
	node    func() *rpmNode
	wantErr string
}

// probeFailureCases: every way a probe can lie must surface as an error, never as data or N/A.
func probeFailureCases() []probeFailureCase {
	sshErr := errors.New("dial tcp 1.2.3.4:22: connection refused")

	return []probeFailureCase{
		{name: "rpm -qf SSH failure is not a tarball", wantErr: "rpm -qf", node: func() *rpmNode {
			n := rpmInstalled()
			n.qfErr = sshErr

			return n
		}},
		{name: "--version failure is not accepted", wantErr: "--version", node: func() *rpmNode {
			n := rpmInstalled()
			n.version, n.versionErr = "", errors.New("exit status 127")

			return n
		}},
		{name: "empty --version is not accepted", wantErr: "returned nothing", node: func() *rpmNode {
			n := rpmInstalled()
			n.version = ""

			return n
		}},
	}
}

// verifyFailureCases: rpm -V outcomes that must be rejected instead of read as a clean or valid diff.
func verifyFailureCases() []probeFailureCase {
	sshErr := errors.New("dial tcp 1.2.3.4:22: connection refused")
	const badVerify = "unexpected rpm -V output"

	return []probeFailureCase{
		{name: "rpm -V SSH failure is reported", wantErr: "rpm -V", node: func() *rpmNode {
			n := rpmInstalled()
			n.verifyErr = sshErr

			return n
		}},
		{name: "rpm -V rc=1 with an error message", wantErr: badVerify, node: func() *rpmNode {
			n := rpmInstalled()
			n.verify, n.verifyRC = "error: package rke2-common is not installed\n", "1"

			return n
		}},
		{name: "rpm -V sudo refusal", wantErr: badVerify, node: func() *rpmNode {
			n := rpmInstalled()
			n.verify, n.verifyRC = "sudo: a password is required\n", "1"

			return n
		}},
		{name: "rpm -V rc=1 without verify lines is inconsistent", wantErr: "inconsistent", node: func() *rpmNode {
			n := rpmInstalled()
			n.verify, n.verifyRC = "", "1"

			return n
		}},
		{name: "rpm -V rc=0 with verify lines is inconsistent", wantErr: "inconsistent", node: func() *rpmNode {
			n := rpmInstalled()
			n.verify, n.verifyRC = "S.5....T.    /usr/bin/rke2\n", "0"

			return n
		}},
		{name: "rpm -qf unexpected rc is a failed probe", wantErr: "rpm -qf", node: func() *rpmNode {
			n := rpmInstalled()
			n.owner = "error: cannot open Packages database"

			return n
		}},
	}
}

func TestCaptureRPMBinaryStateProbeFailures(t *testing.T) {
	for _, tc := range append(probeFailureCases(), verifyFailureCases()...) {
		t.Run(tc.name, func(t *testing.T) {
			n := tc.node()
			state, err := captureRPMBinaryState(n.run, n.hasRPM, "rke2", "1.2.3.4")
			if state != nil {
				t.Fatalf("a failed probe must not yield a state, got %+v", *state)
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}

type sucDriftCase struct {
	name    string
	before  *RPMBinaryState
	after   func() *RPMBinaryState
	wantErr string
}

// base is the pre-SUC rpm state; upgraded is what a correct binary-only SUC leaves behind.
func base() *RPMBinaryState {
	return &RPMBinaryState{
		IP: "1.2.3.4", BinPath: "/usr/bin/rke2", Owner: "rke2-common-1.34.11~rke2r1-0.slemicro.x86_64",
		BinVer: "rke2 version v1.34.11+rke2r1 (2506fee)", Modified: map[string]string{},
	}
}

func upgraded() *RPMBinaryState {
	s := base()
	s.BinVer = "rke2 version v1.34.12-rc2+rke2r1 (fb8b9be)"
	s.Modified = map[string]string{"/usr/bin/rke2": "S.5....T."}

	return s
}

func withConfigDiff(s *RPMBinaryState, flags string) *RPMBinaryState {
	s.Modified["/etc/rancher/rke2/config.yaml"] = flags

	return s
}

func sucDriftCases() []sucDriftCase {
	return []sucDriftCase{
		{name: "only the binary changed, rpm db untouched", before: base(), after: upgraded},
		{
			name:   "pre-existing config diff with the same flags is tolerated",
			before: withConfigDiff(base(), "S.5....T."),
			after:  func() *RPMBinaryState { return withConfigDiff(upgraded(), "S.5....T.") },
		},
		{
			name: "pre-existing diff whose flags change is reported", wantErr: "S.5....T. -> missing",
			before: withConfigDiff(base(), "S.5....T."),
			after:  func() *RPMBinaryState { return withConfigDiff(upgraded(), "missing") },
		},
		{
			name: "pre-existing diff that disappears is reported", wantErr: "-> clean",
			before: withConfigDiff(base(), ".....UG.."), after: upgraded,
		},
		{
			name: "another packaged file changed", before: base(), wantErr: "besides the binary",
			after: func() *RPMBinaryState {
				s := upgraded()
				s.Modified["/usr/share/rke2/rke2-cis-sysctl.conf"] = "S.5....T."

				return s
			},
		},
		{
			name: "rpm owner changed: package manager touched", before: base(), wantErr: "rpm owner changed",
			after: func() *RPMBinaryState {
				s := upgraded()
				s.Owner = "rke2-common-1.34.12~rc2.rke2r1-0.slemicro.x86_64"

				return s
			},
		},
		{
			name: "binary version unchanged", before: base(), wantErr: "binary version unchanged",
			after: func() *RPMBinaryState {
				s := base()
				s.Modified = map[string]string{"/usr/bin/rke2": "S.5....T."}

				return s
			},
		},
		{
			name: "version changed but rpm -V clean", before: base(), wantErr: "does not report /usr/bin/rke2",
			after: func() *RPMBinaryState {
				s := upgraded()
				s.Modified = map[string]string{}

				return s
			},
		},
	}
}

func TestSUCRPMDriftEvaluation(t *testing.T) {
	for _, tc := range sucDriftCases() {
		t.Run(tc.name, func(t *testing.T) {
			notes, err := sucRPMDrift(tc.before, tc.after())
			if len(notes) == 0 || !strings.Contains(notes[0], "rpm database still reports") {
				t.Fatalf("expected the expected-drift note first, got %q", notes)
			}
			if tc.wantErr == "" && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
				t.Fatalf("expected error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}
