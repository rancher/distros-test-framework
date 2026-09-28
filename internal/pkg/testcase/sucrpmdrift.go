package testcase

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/rancher/distros-test-framework/internal/provisioning/driver"
	"github.com/rancher/distros-test-framework/internal/resources"
)

// RPMBinaryState is what a binary-based SUC upgrade may change on an rpm-installed node:
// the product binary itself, never the package database or any other packaged file.
type RPMBinaryState struct {
	IP       string
	BinPath  string
	Owner    string            // NEVRA of the rpm owning BinPath; "" for tarball installs
	BinVer   string            // first line of `<product> --version`
	Modified map[string]string // `rpm -V Owner`: path -> verify flags
}

// CaptureRPMBinaryStates snapshots every node; nodes without rpm, with an unowned (tarball)
// binary or whose probe failed get a nil entry and are skipped by TestSUCRPMDrift.
func CaptureRPMBinaryStates(cluster *driver.Cluster) map[string]*RPMBinaryState {
	states := make(map[string]*RPMBinaryState)
	for _, ip := range append(append([]string{}, cluster.ServerIPs...), cluster.AgentIPs...) {
		state, err := captureRPMBinaryState(resources.RunCommandOnNode, rpmPresent, cluster.Config.Product, ip)
		if err != nil {
			resources.LogLevel("warn", "pre-SUC rpm snapshot on %s skipped: %v", ip, err)
			state = nil
		}
		states[ip] = state
		if state != nil {
			resources.LogLevel("info", "pre-SUC %s: %s owned by %s, %q, rpm -V diffs: %d",
				ip, state.BinPath, state.Owner, state.BinVer, len(state.Modified))
		}
	}

	return states
}

// TestSUCRPMDrift reports what a binary-based SUC did on rpm-installed nodes. Diagnostic only:
// the expected rpm-vs-binary drift and any anomaly are logged, never asserted.
func TestSUCRPMDrift(cluster *driver.Cluster, before map[string]*RPMBinaryState) {
	for ip, prev := range before {
		if prev == nil {
			resources.LogLevel("info", "%s: no rpm-owned %s binary, SUC rpm drift report is N/A", ip, cluster.Config.Product)
			continue
		}

		after, err := captureRPMBinaryState(resources.RunCommandOnNode, rpmPresent, cluster.Config.Product, ip)
		if err != nil || after == nil {
			resources.LogLevel("warn", "%s: post-SUC rpm snapshot unavailable (err=%v), report skipped", ip, err)
			continue
		}

		notes, driftErr := sucRPMDrift(prev, after)
		for _, n := range notes {
			resources.LogLevel("info", "%s: %s", ip, n)
		}
		if driftErr != nil {
			resources.LogLevel("warn", "%s: %v", ip, driftErr)
		}
	}
}

// rpmPresent is a read-only presence probe: unlike FindPath it writes nothing on the node.
func rpmPresent(ip string) (bool, error) {
	out, err := resources.RunCommandOnNode("command -v rpm >/dev/null 2>&1 && echo yes || echo no", ip)
	if err != nil {
		return false, fmt.Errorf("rpm presence probe on %s: %w", ip, err)
	}

	return strings.TrimSpace(out) == "yes", nil
}

// captureRPMBinaryState is the injectable core of the snapshot. Every probe failure is an
// error for the caller to report; only "rpm absent" and "binary not owned" mean N/A (nil).
func captureRPMBinaryState(
	run func(cmd, ip string) (string, error),
	hasRPM func(ip string) (bool, error),
	product, ip string,
) (*RPMBinaryState, error) {
	rpm, err := hasRPM(ip)
	if err != nil || !rpm {
		return nil, err
	}

	findBin := fmt.Sprintf("readlink -f \"$(command -v %[1]s || ls /usr/bin/%[1]s /usr/local/bin/%[1]s "+
		"/opt/%[1]s/bin/%[1]s 2>/dev/null | head -1)\"", product)
	binPath, err := run(findBin, ip)
	if err != nil {
		return nil, fmt.Errorf("locate %s binary on %s: %w", product, ip, err)
	}
	binPath = strings.TrimSpace(binPath)
	if binPath == "" {
		return nil, fmt.Errorf("no %s binary found on %s", product, ip)
	}

	// RunCommandOnNode drops stdout on a non-zero exit, so the rc travels inside the output.
	owner, rc, err := runWithRC(run, "rpm -qf "+binPath, ip)
	if err != nil {
		return nil, fmt.Errorf("rpm -qf %s on %s: %w", binPath, ip, err)
	}
	if rc == "1" && strings.Contains(owner, "is not owned by any package") {
		return nil, nil //nolint:nilnil // tarball install: nothing to compare
	}
	if rc != "0" || owner == "" || strings.ContainsAny(owner, " \n") {
		return nil, fmt.Errorf("rpm -qf %s on %s (rc=%s): %q", binPath, ip, rc, owner)
	}

	binVer, err := run("sudo "+binPath+" --version", ip)
	if err != nil {
		return nil, fmt.Errorf("%s --version on %s: %w", binPath, ip, err)
	}
	binVer = strings.TrimSpace(strings.SplitN(binVer, "\n", 2)[0])
	if binVer == "" {
		return nil, fmt.Errorf("%s --version on %s returned nothing", binPath, ip)
	}

	modified, err := rpmVerify(run, owner, ip)
	if err != nil {
		return nil, err
	}

	return &RPMBinaryState{IP: ip, BinPath: binPath, Owner: owner, BinVer: binVer, Modified: modified}, nil
}

const rcMarker = "__dtf_rc="

// runWithRC runs cmd so the remote exit status never fails the SSH call: stdout+stderr come
// back as body and the command's own rc separately (the node helper drops output on rc!=0).
func runWithRC(run func(cmd, ip string) (string, error), cmd, ip string) (body, rc string, err error) {
	out, err := run(cmd+" 2>&1; echo "+rcMarker+"$?", ip)
	if err != nil {
		return "", "", err
	}
	body, rc, found := strings.Cut(out, rcMarker)
	if !found {
		return "", "", fmt.Errorf("no exit status marker in output of %q: %q", cmd, strings.TrimSpace(out))
	}

	return strings.TrimSpace(body), strings.TrimSpace(rc), nil
}

// rpmVerify runs `rpm -V`: rc 0 means clean, rc 1 must come with parseable verify lines only.
// Error text (missing package, sudo refusal) or an rc without lines is a failed probe.
func rpmVerify(run func(cmd, ip string) (string, error), owner, ip string) (map[string]string, error) {
	body, rc, err := runWithRC(run, "sudo rpm -V "+owner, ip)
	if err != nil {
		return nil, fmt.Errorf("rpm -V %s on %s: %w", owner, ip, err)
	}
	modified, parseErr := parseRPMVerify(body)
	switch {
	case parseErr != nil:
		return nil, fmt.Errorf("rpm -V %s on %s (rc=%s): %w", owner, ip, rc, parseErr)
	case rc == "0" && len(modified) == 0:
		return modified, nil
	case rc == "1" && len(modified) > 0:
		return modified, nil
	default:
		return nil, fmt.Errorf("rpm -V %s on %s: rc=%s with %d verify lines is inconsistent: %q",
			owner, ip, rc, len(modified), body)
	}
}

// rpm -V lines: "SM5DLUGTP" flags (or "missing"), an optional attribute column, then the path.
var rpmVerifyLineRE = regexp.MustCompile(`^(missing|[SM5DLUGTP.]{8,9})(?:\s+[cdglr]+)?\s+(/\S.*)$`)

// parseRPMVerify maps `rpm -V` output to path -> flags; any line that is not a verify line
// (error text, sudo prompts, dependency notes) makes the whole output untrusted.
func parseRPMVerify(out string) (map[string]string, error) {
	modified := make(map[string]string)
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		m := rpmVerifyLineRE.FindStringSubmatch(line)
		if m == nil {
			return nil, fmt.Errorf("unexpected rpm -V output line: %q", line)
		}
		modified[m[2]] = m[1]
	}

	return modified, nil
}

// sucRPMDrift returns human-readable notes plus an error for anything a binary-only
// upgrade must not do. An unchanged rpm version is expected, not an error.
func sucRPMDrift(before, after *RPMBinaryState) ([]string, error) {
	var notes []string
	var problems []string

	notes = append(notes, fmt.Sprintf("rpm database still reports %s while the binary reports %q: "+
		"expected, SUC replaces the binary without the package manager", after.Owner, after.BinVer))

	if after.Owner != before.Owner {
		problems = append(problems, fmt.Sprintf("rpm owner changed from %s to %s", before.Owner, after.Owner))
	}
	if after.BinVer == before.BinVer {
		problems = append(problems, fmt.Sprintf("binary version unchanged (%q), SUC did not replace %s",
			after.BinVer, after.BinPath))
	}
	if after.BinPath != before.BinPath {
		problems = append(problems, fmt.Sprintf("binary path moved from %s to %s", before.BinPath, after.BinPath))
	}

	var extra []string
	for path, flags := range after.Modified {
		if path == after.BinPath {
			continue
		}
		if prevFlags, preexisting := before.Modified[path]; !preexisting {
			extra = append(extra, path+" ("+flags+")")
		} else if prevFlags != flags {
			extra = append(extra, path+" ("+prevFlags+" -> "+flags+")")
		}
	}
	for path, prevFlags := range before.Modified {
		if _, still := after.Modified[path]; !still && path != after.BinPath {
			extra = append(extra, path+" ("+prevFlags+" -> clean)")
		}
	}
	sort.Strings(extra)
	if len(extra) > 0 {
		problems = append(problems, "packaged files besides the binary changed: "+strings.Join(extra, ", "))
	}
	if _, ok := after.Modified[after.BinPath]; !ok {
		problems = append(problems, fmt.Sprintf("rpm -V does not report %s as modified although its version changed",
			after.BinPath))
	} else {
		notes = append(notes, fmt.Sprintf("rpm -V %s: %s %s (only the binary, as expected)",
			after.Owner, after.Modified[after.BinPath], after.BinPath))
	}

	if len(problems) > 0 {
		return notes, errors.New("binary-based SUC drift: " + strings.Join(problems, "; "))
	}

	return notes, nil
}
