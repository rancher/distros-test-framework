package qase

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func fakeFailures(n int) []*FailureDetails {
	out := make([]*FailureDetails, 0, n)
	for i := range n {
		out = append(out, &FailureDetails{
			TestSuite:     fmt.Sprintf("Suite%d", i),
			TestCase:      "Starts up with no issues",
			ErrorType:     "assertion",
			FailedCommand: "vagrant up --no-tty server-0",
			ErrorMessage:  strings.Repeat("x", 5000), // forces truncation
			Duration:      4.2,
		})
	}

	return out
}

// 28 failures x 4 blocks = 112 blocks: one message would be rejected by Slack
// (invalid_blocks, 50-block limit). Every chunk must stay <= 50 and keep all failures.
func TestChunkFailureBlocksRespectsSlackLimit(t *testing.T) {
	failures := fakeFailures(28)
	chunks := chunkFailureBlocks(failures, slackMaxBlocksPerMessage)

	if len(chunks) < 3 {
		t.Fatalf("expected at least 3 chunks for 112 blocks, got %d", len(chunks))
	}
	sections := 0
	for i, c := range chunks {
		if len(c) > slackMaxBlocksPerMessage {
			t.Errorf("chunk %d has %d blocks (> %d)", i, len(c), slackMaxBlocksPerMessage)
		}
		if c[0].Type != "header" {
			t.Errorf("chunk %d does not start with a header block", i)
		}
		if want := fmt.Sprintf("Failure Details (%d/%d)", i+1, len(chunks)); c[0].Text.Text != want {
			t.Errorf("chunk %d header = %q, want %q", i, c[0].Text.Text, want)
		}
		for _, b := range c {
			if b.Type == "section" && utf8.RuneCountInString(b.Text.Text) > slackMaxSectionText {
				t.Errorf("section text exceeds Slack's %d-character limit", slackMaxSectionText)
			}
			if b.Type == "section" && strings.HasPrefix(b.Text.Text, ":x:") {
				sections++
			}
		}
	}
	if sections != 28 {
		t.Errorf("expected 28 failure sections across chunks, got %d", sections)
	}
}

func TestFailureBlocksTruncateUnicodeWithoutCorruption(t *testing.T) {
	failure := fakeFailures(1)[0]
	failure.FailedCommand = strings.Repeat("á", slackMaxSectionText)
	failure.ErrorMessage = strings.Repeat("界", slackMaxSectionText)
	for _, block := range failureBlocks(1, failure) {
		if block.Text == nil {
			continue
		}
		if !utf8.ValidString(block.Text.Text) {
			t.Fatal("failure block contains invalid UTF-8")
		}
		if utf8.RuneCountInString(block.Text.Text) > slackMaxSectionText {
			t.Fatalf("failure block exceeds %d characters", slackMaxSectionText)
		}
		if strings.Contains(block.Text.Text, "```") && !strings.HasSuffix(block.Text.Text, "```") {
			t.Fatal("truncated code block is missing its closing fence")
		}
	}
}

func TestPostFailureDetailsSendsEveryBoundedChunkToThread(t *testing.T) {
	var messages []slackMessage
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		var msg slackMessage
		if err := json.NewDecoder(req.Body).Decode(&msg); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		messages = append(messages, msg)
		body := io.NopCloser(bytes.NewBufferString(`{"ok":true,"ts":"1.2"}`))

		return &http.Response{StatusCode: http.StatusOK, Body: body, Header: make(http.Header)}, nil
	})}
	sc := &slackClient{channelID: "C123", client: client}
	pd := &processedTestdata{testSummary: []testOverview{{testCases: make([]testDetails, 28)}}}
	for i, failure := range fakeFailures(28) {
		pd.testSummary[0].testCases[i] = testDetails{status: failStatus, failureDetails: failure}
	}

	if err := sc.PostFailureDetails(pd, "thread-123"); err != nil {
		t.Fatalf("PostFailureDetails: %v", err)
	}
	if len(messages) != 3 {
		t.Fatalf("sent %d messages, want 3", len(messages))
	}
	for i, msg := range messages {
		if msg.ThreadTS != "thread-123" || len(msg.Blocks) > slackMaxBlocksPerMessage {
			t.Errorf("message %d: thread=%q blocks=%d", i, msg.ThreadTS, len(msg.Blocks))
		}
	}
}

func TestPostSlackResultsFallsBackAndReturnsDetailsError(t *testing.T) {
	responses := []string{
		`{"ok":true,"ts":"parent-123"}`,
		`{"ok":false,"error":"invalid_blocks"}`,
		`{"ok":true,"ts":"fallback-123"}`,
	}
	var messages []slackMessage
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		var msg slackMessage
		if err := json.NewDecoder(req.Body).Decode(&msg); err != nil {
			return nil, fmt.Errorf("decode request: %w", err)
		}
		messages = append(messages, msg)
		if len(messages) > len(responses) {
			return nil, fmt.Errorf("unexpected Slack request %d", len(messages))
		}
		body := io.NopCloser(bytes.NewBufferString(responses[len(messages)-1]))

		return &http.Response{StatusCode: http.StatusOK, Body: body, Header: make(http.Header)}, nil
	})}
	sc := &slackClient{channelID: "C123", client: client}
	failure := fakeFailures(1)[0]
	failure.TestSuite = "Test_E2EBtrfsSnapshot"
	pd := &processedTestdata{
		failedTests: 1,
		testSummary: []testOverview{{testCases: []testDetails{{
			status: failStatus, failureDetails: failure,
		}}}},
		testSuiteSummary: []testSuiteDetails{{
			testSuiteName: "Test_E2EBtrfsSnapshot", failedTests: 1,
		}},
	}
	base := t.TempDir()
	if err := os.Mkdir(filepath.Join(base, "report"), 0o755); err != nil {
		t.Fatal(err)
	}

	err := postSlackResults(sc, pd, "k3s", "amd64", base, 0)
	if err == nil || !strings.Contains(err.Error(), "invalid_blocks") {
		t.Fatalf("error = %v, want surfaced invalid_blocks failure", err)
	}
	if len(messages) != 3 {
		t.Fatalf("sent %d messages, want summary + details + fallback", len(messages))
	}
	if len(messages[2].Blocks) != 0 || messages[2].ThreadTS != "parent-123" ||
		!strings.Contains(messages[2].Text, "Test_E2EBtrfsSnapshot") {
		t.Errorf("fallback message was not a plain-text diagnosis in the parent thread: %+v", messages[2])
	}
	state, err := os.ReadFile(filepath.Join(base, "report", ".rerun-state.json"))
	if err != nil {
		t.Fatalf("read rerun state: %v", err)
	}
	if !strings.Contains(string(state), `"btrfs"`) {
		t.Fatalf("rerun state does not contain mapped failed test: %s", state)
	}
}

func TestChunkFailureBlocksSingleMessageKeepsPlainHeader(t *testing.T) {
	chunks := chunkFailureBlocks(fakeFailures(3), slackMaxBlocksPerMessage)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if chunks[0][0].Text.Text != "Failure Details" {
		t.Errorf("single chunk header = %q", chunks[0][0].Text.Text)
	}
}

func TestChunkFailureBlocksEmpty(t *testing.T) {
	if got := chunkFailureBlocks(nil, slackMaxBlocksPerMessage); len(got) != 0 {
		t.Errorf("expected no chunks for no failures, got %d", len(got))
	}
}

// Without report/.test-dirs.txt the old code returned nil and the rerun state was
// saved with failed_tests=[]; now the built-in per-product list is used.
func TestGetFailedTestDirsFallsBackToDefaults(t *testing.T) {
	base := t.TempDir()
	// real suite names as they appear in the go test -json log.
	pd := &processedTestdata{
		failedTests: 4,
		testSuiteSummary: []testSuiteDetails{
			{testSuiteName: "Test_E2EBtrfsSnapshot", failedTests: 1},
			{testSuiteName: "Test_E2EClusterValidation", failedTests: 1},
			{testSuiteName: "Test_E2ECustomCARotation", failedTests: 1},
			{testSuiteName: "Test_E2ESecretsEncryptionOld", failedTests: 1},
			{testSuiteName: "Test_E2ERootless", failedTests: 0},
		},
	}

	got := getFailedTestDirs(pd, base, "k3s")
	want := map[string]bool{"btrfs": true, "validatecluster": true, "rotateca": true, "secretsencryption_old": true}
	if len(got) != len(want) {
		t.Fatalf("got %v, want dirs %v", got, want)
	}
	for _, d := range got {
		if !want[d] {
			t.Errorf("unexpected dir %q in %v", d, got)
		}
	}
	if strings.Join(got, ",") != "btrfs,rotateca,secretsencryption_old,validatecluster" {
		t.Errorf("failed dirs are not deterministic: %v", got)
	}

	// an explicit file still wins over the defaults.
	if err := os.MkdirAll(filepath.Join(base, "report"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "report", ".test-dirs.txt"), []byte("btrfs\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got = getFailedTestDirs(pd, base, "k3s")
	if len(got) != 1 || got[0] != "btrfs" {
		t.Errorf("with explicit file got %v, want [btrfs]", got)
	}

	if got := getFailedTestDirs(pd, t.TempDir(), "unknown-product"); got != nil {
		t.Errorf("unknown product without file should give nil, got %v", got)
	}
}
