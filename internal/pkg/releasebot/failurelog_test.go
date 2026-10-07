package releasebot

import (
	"strconv"
	"strings"
	"testing"
)

// ginkgoLog is a failed DTF build's console as jenkins prints it (stamps, colors), with the
// details that differ between RCs (volume ids, durations) as parameters.
func ginkgoLog(pvc, version string, seconds int) string {
	return strings.Join([]string{
		"[2026-09-29T00:10:00.000Z] [Pipeline] { (Run TestCombination)",
		"[2026-09-29T00:10:01.000Z] installing " + version,
		"[2026-09-29T01:00:59.356Z] \x1b[38;5;9m• [FAILED] [" + strconv.Itoa(seconds) + ".805 seconds]\x1b[0m",
		"[2026-09-29T01:00:59.356Z] Test: [It] Verifies Local Path Provisioner storage",
		"[2026-09-29T01:00:59.356Z]   \x1b[38;5;9m[FAILED] local-path volume cleanup failed",
		"[2026-09-29T01:00:59.356Z]   Unexpected error:",
		"[2026-09-29T01:00:59.356Z]       PV pvc-" + pvc + " was not deleted within 2m0s: PV still exists",
		"[2026-09-29T01:00:59.357Z] ------------------------------",
		"[2026-09-29T01:01:56.154Z] \x1b[1mSummarizing 1 Failure:\x1b[0m",
		"[2026-09-29T01:01:56.154Z]   \x1b[38;5;9m[FAIL]\x1b[0m Test: [It] Verifies Local Path Provisioner storage",
		"[2026-09-29T01:01:56.154Z] ",
		"[2026-09-29T01:01:56.154Z] FAIL! -- 14 Passed | 1 Failed",
		"[2026-09-29T01:02:00.246Z] Finished: FAILURE",
	}, "\n")
}

// The same failure on two RCs has one signature; a different failure has another.
func TestExtractFailureSignature(t *testing.T) {
	ex1, sig1 := extractFailure(ginkgoLog("0158ab16-37b5-447f-97da-6d35ee3873ab", "v1.37.1-rc2+k3s1", 178))
	_, sig2 := extractFailure(ginkgoLog("9a1c2d3e-1111-4222-8333-444455556666", "v1.35.9-rc2+k3s1", 181))
	if sig1 == "" || sig1 != sig2 {
		t.Fatalf("same failure, signatures %q and %q", sig1, sig2)
	}
	for _, want := range []string{
		"Stage: Run TestCombination", "Failed: Test: [It] Verifies Local Path Provisioner storage",
		"[FAILED] local-path volume cleanup failed", "Finished: FAILURE",
	} {
		if !strings.Contains(ex1, want) {
			t.Fatalf("excerpt misses %q:\n%s", want, ex1)
		}
	}
	if strings.Contains(ex1, "\x1b[") || strings.Contains(ex1, "[2026-") {
		t.Fatalf("excerpt keeps colors or stamps:\n%s", ex1)
	}

	other := strings.ReplaceAll(ginkgoLog("x", "v1", 1), "local-path volume cleanup failed", "pod not ready in 5m")
	if _, sig3 := extractFailure(other); sig3 == sig1 {
		t.Fatal("a different failure reason got the same signature")
	}
}

// Without Ginkgo output (provisioning, pipeline errors) the first error lines and their stage
// make the signature; a log with nothing recognizable has none.
func TestExtractFailureWithoutGinkgo(t *testing.T) {
	tofu := func(host string) string {
		return strings.Join([]string{
			"[Pipeline] { (Configure and Build)",
			"ok: [" + host + "] => changed=0 failed=0",
			"Error: creating EC2 Instance: InsufficientInstanceCapacity: no capacity in us-east-2c for t3a.medium",
			"  on main.tf line 12",
			"Finished: FAILURE",
		}, "\n")
	}
	ex, sig := extractFailure(tofu("10.0.0.1"))
	if _, sig2 := extractFailure(tofu("10.0.9.9")); sig == "" || sig != sig2 {
		t.Fatalf("signatures %q and %q", sig, sig2)
	}
	if !strings.Contains(ex, "Stage: Configure and Build") || !strings.Contains(ex, "InsufficientInstanceCapacity") ||
		strings.Contains(ex, "failed=0\n---") {
		t.Fatalf("excerpt:\n%s", ex)
	}
	if _, none := extractFailure("[Pipeline] End of Pipeline\nFinished: SUCCESS"); none != "" {
		t.Fatalf("signature for a log without failures: %q", none)
	}
}

// Numbers that carry the cause stay in the signature: a 403 is not a 503, an OOM kill (137) is
// not a plain exit 1. Volatile values (ids, addresses, times, durations) do not count.
func TestSignatureKeepsMeaningfulNumbers(t *testing.T) {
	sig := func(line string) string {
		_, s := extractFailure("[Pipeline] { (Run TestCombination)\n" + line + "\nFinished: FAILURE")
		return s
	}
	for name, pair := range map[string][2]string{
		"http status": {"Error: GET https://h/api: 403 Forbidden", "Error: GET https://h/api: 503 Service Unavailable"},
		"exit status": {"Error: command failed: exit status 1", "Error: command failed: exit status 137"},
		"expected":    {"Error: expected 3 nodes Ready, got 2", "Error: expected 3 nodes Ready, got 0"},
	} {
		if a, b := sig(pair[0]), sig(pair[1]); a == "" || a == b {
			t.Fatalf("%s: %q and %q share signature %q", name, pair[0], pair[1], a)
		}
	}
	same := [2]string{
		"Error: pod web-6858d854cf timed out after 120s at 10.0.0.1:6443 on 2026-09-29 01:00:57",
		"Error: pod web-9a1c2d3e44 timed out after 95s at 10.0.3.7:6443 on 2026-09-30 11:12:13",
	}
	if a, b := sig(same[0]), sig(same[1]); a == "" || a != b {
		t.Fatalf("volatile values split one failure: %q vs %q", a, b)
	}
}

// jenkins' own trailers are not a cause: a log with nothing else gets no signature (not reused).
func TestSignatureIgnoresGenericTrailers(t *testing.T) {
	log := strings.Join([]string{
		"[Pipeline] { (Run TestCombination)",
		"ERROR: script returned exit code 1",
		"Also:   hudson.remoting.ProxyException: hudson.AbortException: script returned exit code 1",
		"Finished: FAILURE",
	}, "\n")
	if _, sig := extractFailure(log); sig != "" {
		t.Fatalf("signature %q from generic trailers only", sig)
	}
}

// Gomega prints the compared values on the lines after [FAILED]: they are part of the signature,
// so the same spec failing with different values is not the same failure.
func TestSignatureKeepsGomegaValues(t *testing.T) {
	gomega := func(actual, took string) string {
		return strings.Join([]string{
			"[Pipeline] { (Run TestCombination)",
			"• [FAILED] [" + took + " seconds]",
			"Test: [It] Verifies exit code",
			"  [FAILED] Expected",
			"      <int>: " + actual,
			"  to equal",
			"      <int>: 0",
			"  In [It] at: /go/src/x/validate.go:40 @ 09/29/26 01:00:57.642",
			"------------------------------",
			"Summarizing 1 Failure:",
			"  [FAIL] Test: [It] Verifies exit code",
			"",
			"Finished: FAILURE",
		}, "\n")
	}
	_, one := extractFailure(gomega("1", "12.345"))
	_, oom := extractFailure(gomega("137", "12.345"))
	_, again := extractFailure(gomega("1", "98.7"))
	if one == "" || one == oom || one != again {
		t.Fatalf("exit 1 %q, exit 137 %q, exit 1 again %q", one, oom, again)
	}
}

// The excerpt is redacted where it is made, so every path (spool, prompt, -triage-build) gets the
// same text; the signature still comes from the raw lines.
func TestExcerptIsRedacted(t *testing.T) {
	log := "[Pipeline] { (Run TestCombination)\n" +
		"Error: login failed, password=hunter2 token 11d3f9e8a7b6c5d4e3f2a1b0c9d8e7f6a5\nFinished: FAILURE"
	f := AnalyzeFailure(log)
	if strings.Contains(f.Excerpt, "hunter2") || strings.Contains(f.Excerpt, "11d3f9e8") ||
		!strings.Contains(f.Excerpt, "password=[redacted]") || f.Signature == "" {
		t.Fatalf("excerpt %q signature %q", f.Excerpt, f.Signature)
	}
}

// Timeout settings are not failures: with every git step printing "# timeout=10" and go test's
// "-timeout=100m", the first failure (and the signature) must still be the real error, here an rpm
// repo whose signature does not match (baler rke2_validate_cluster_rpm #482).
func TestTimeoutSettingsAreNotFailures(t *testing.T) {
	log := strings.Join([]string{
		"[Pipeline] { (Checkout)",
		" > git init /home/jenkins/agent/workspace/rke2-tests/rke2_validate_cluster_rpm # timeout=10",
		" > git fetch --tags --force --progress -- https://github.com/rancher/distros-test-framework.git # timeout=10",
		"[Pipeline] { (Run TestCombination)",
		"+ docker run acceptance-tests sh -c 'go test -timeout=100m -v ./validatecluster/...'",
		"module.master.aws_instance.master (remote-exec): Error: Failed to download metadata for repo " +
			"'rancher-rke2-1.37-stable': repomd.xml GPG signature verification error: Bad PGP signature",
		"module.master.aws_instance.master (remote-exec): Failed to install rke2-server on node ip: 3.140.216.67",
		"ERRO[2026-10-01T14:00:59Z] error getting cluster:",
		"[Pipeline] { (Cleanup)",
		"Finished: FAILURE",
	}, "\n")
	f := AnalyzeFailure(log)
	if !strings.Contains(f.Primary, "Bad PGP signature") || !strings.Contains(f.Excerpt, "Stage: Run TestCombination") {
		t.Fatalf("primary %q, excerpt:\n%s", f.Primary, f.Excerpt)
	}
	other := AnalyzeFailure(strings.Replace(log, "Bad PGP signature", "No space left on device", 1))
	if other.Signature == f.Signature {
		t.Fatal("two different failures share a signature: it comes from the git steps")
	}
	realErr := "Error: timeout: context deadline exceeded (--timeout 5m)"
	if got := timeoutSetting.ReplaceAllString(realErr, ""); !strings.Contains(got, "Error") {
		t.Fatalf("a real error next to a timeout setting was dropped: %q", got)
	}
}
