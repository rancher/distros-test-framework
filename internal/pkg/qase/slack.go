package qase

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/rancher/distros-test-framework/internal/resources"
)

const (
	slackAuthURL    = "https://slack.com/api/auth.test"
	slackPostMsgURL = "https://slack.com/api/chat.postMessage"
)

type slackClient struct {
	token     string
	channelID string
	client    *http.Client
	dryRun    bool
}

type slackAuthResponse struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
	User  string `json:"user,omitempty"`
	Team  string `json:"team,omitempty"`
}

type slackPostResponse struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
	TS    string `json:"ts,omitempty"`
}

type slackMessage struct {
	Channel  string       `json:"channel"`
	Text     string       `json:"text,omitempty"`
	Blocks   []slackBlock `json:"blocks,omitempty"`
	ThreadTS string       `json:"thread_ts,omitempty"`
}

type slackBlock struct {
	Type     string           `json:"type"`
	Text     *slackBlockText  `json:"text,omitempty"`
	Fields   []slackBlockText `json:"fields,omitempty"`
	Elements []slackBlockText `json:"elements,omitempty"`
}

type slackBlockText struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// ReportToSlack processes test data and posts results to Slack.
func ReportToSlack(fileName, product, ciArch, baseDir string, runID int32) error {
	slackClient, err := newSlackClient()
	if err != nil {
		resources.LogLevel("warn", "Slack client not configured: %v", err)
		return err
	}

	if !slackClient.dryRun {
		if err := slackClient.validateConnection(); err != nil {
			return fmt.Errorf("slack connection validation failed: %w", err)
		}
	}

	pd, err := processTestData(fileName, product, ciArch)
	if err != nil {
		return fmt.Errorf("error processing test data for Slack: %w", err)
	}

	return postSlackResults(slackClient, pd, product, ciArch, baseDir, runID)
}

// postSlackResults handles the Slack posting logic including thread management and failure details.
func postSlackResults(sc *slackClient, pd *processedTestdata, product, ciArch, baseDir string, runID int32) error {
	failedDirs := getFailedTestDirs(pd, baseDir, product)

	var parentThreadTS string
	isRerunEnv := os.Getenv("IS_RERUN")
	isRerun := isRerunEnv == "true"
	resources.LogLevel("info", "IS_RERUN env value: '%s', isRerun: %v", isRerunEnv, isRerun)
	statePath := filepath.Join(baseDir, "report", ".rerun-state.json")

	if isRerun {
		if data, err := os.ReadFile(statePath); err == nil {
			var state map[string]interface{}
			if json.Unmarshal(data, &state) == nil {
				if ts, ok := state["thread_ts"].(string); ok && ts != "" {
					parentThreadTS = ts
					resources.LogLevel("info", "Rerun detected, will post as reply to thread: %s", parentThreadTS)
				}
			}
		}
	} else {
		resources.LogLevel("info", "Fresh run detected, will create new Slack thread")
	}

	threadTS, err := sc.PostTestResults(pd, product, ciArch, runID, baseDir, parentThreadTS)
	if err != nil {
		return fmt.Errorf("error posting test results to Slack: %w", err)
	}

	// detailed failure information if there are failures.
	resources.LogLevel("info", "Failure details check: failedTests=%d, threadTS=%q, parentThreadTS=%q",
		pd.failedTests, threadTS, parentThreadTS)
	var detailsErr error
	if pd.failedTests > 0 && (threadTS != "" || sc.dryRun) {
		effectiveThreadTS := threadTS
		if parentThreadTS != "" {
			effectiveThreadTS = parentThreadTS
		}

		// A failed details post is not a successful report. Attempt a plain-text
		// fallback, then surface the original error to the caller.
		if err := sc.PostFailureDetails(pd, effectiveThreadTS); err != nil {
			resources.LogLevel("error", "Failed to post failure details: %v", err)
			if fbErr := sc.postFailureFallback(pd, effectiveThreadTS, err); fbErr != nil {
				resources.LogLevel("error", "Failed to post failure fallback: %v", fbErr)
			}
			detailsErr = fmt.Errorf("failure details not posted to Slack thread %s: %w", effectiveThreadTS, err)
		}
	}

	if baseDir != "" && (threadTS != "" || sc.dryRun) {
		persistRerunState(sc, pd, baseDir, product, threadTS, parentThreadTS, failedDirs)
	}

	return detailsErr
}

// persistRerunState records (fresh run) or updates (rerun) the failed test dirs the
// rerun-poller needs for "rerun: failed", warning loudly when nothing could be resolved.
func persistRerunState(sc *slackClient, pd *processedTestdata, baseDir, product, threadTS, parentThreadTS string,
	failedDirs []string,
) {
	if pd.failedTests > 0 && len(failedDirs) == 0 {
		resources.LogLevel("warn", "%d test(s) failed but no failed test directories could be resolved; "+
			"'rerun: failed' will have nothing to rerun (is report/.test-dirs.txt written by the runner?)",
			pd.failedTests)
	}
	if sc.dryRun {
		resources.LogLevel("info", "[DRY RUN] rerun state would record failed_tests=%v", failedDirs)
		return
	}
	if parentThreadTS == "" {
		if err := saveRerunState(baseDir, sc.channelID, threadTS, product, failedDirs); err != nil {
			resources.LogLevel("warn", "Failed to save rerun state: %v", err)
		}
	} else if err := updateFailedTests(baseDir, failedDirs); err != nil {
		resources.LogLevel("warn", "Failed to update failed tests: %v", err)
	}
}

func newSlackClient() (*slackClient, error) {
	token := os.Getenv("SLACK_TOKEN")
	channelID := os.Getenv("SLACK_CHANNEL_ID")
	dryRun := os.Getenv("SLACK_DRY_RUN") == "true"

	if token == "" && !dryRun {
		return nil, errors.New("SLACK_TOKEN environment variable is not set")
	}
	if channelID == "" && !dryRun {
		return nil, errors.New("SLACK_CHANNEL_ID environment variable is not set")
	}

	if dryRun {
		resources.LogLevel("info", "Slack DRY RUN mode enabled - will not post to Slack")
	}

	return &slackClient{
		token:     token,
		channelID: channelID,
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
		dryRun: dryRun,
	}, nil
}

func (s *slackClient) validateConnection() error {
	if s.dryRun {
		resources.LogLevel("info", "[DRY RUN] Would validate Slack connection")
		return nil
	}

	resources.LogLevel("info", "Validating Slack connection...")

	req, err := http.NewRequest(http.MethodGet, slackAuthURL, http.NoBody)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+s.token)

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("network error validating Slack: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	var authResp slackAuthResponse
	if err := json.Unmarshal(body, &authResp); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	if !authResp.OK {
		if authResp.Error == "invalid_auth" {
			return errors.New("slack token validation failed: invalid_auth")
		}
		return fmt.Errorf("slack token validation failed: %s", authResp.Error)
	}

	resources.LogLevel("info", "Connected to Slack as: %s (Team: %s)", authResp.User, authResp.Team)

	return nil
}

func (s *slackClient) PostMessage(text string) error {
	msg := slackMessage{
		Channel: s.channelID,
		Text:    text,
	}
	_, err := s.sendMessage(msg)

	return err
}

func mapTestSuiteToDir(suiteName string, testDirs []string) string {
	lowerSuite := strings.ToLower(suiteName)

	// direct mappings for special cases where dir name does not match suite name.
	specialMappings := map[string]string{
		"upgradevalidation":    "upgradecluster",
		"clustervalidation":    "validatecluster",
		"ciliumwireguard":      "cilium_wireguard",
		"secretsencryptionold": "secretsencryption_old",
		"kinevalidation":       "kine",
		"mixedosbgpvalidation": "mixedosbgp",
		"mixedosvalidation":    "mixedos",
		"mixedosflannel":       "mixedos",
		"calicoebpf":           "calico_ebpf",
		"customcarotation":     "rotateca", // Test_E2ECustomCARotation
		"btrfssnapshot":        "btrfs",
		"startupvalidation":    "startup",
	}

	// check special mappings first.
	for suiteKey, dirName := range specialMappings {
		if strings.Contains(lowerSuite, suiteKey) {
			for _, dir := range testDirs {
				if strings.EqualFold(dir, dirName) {
					return dir
				}
			}
		}
	}

	// fall back to original logic.
	for _, dir := range testDirs {
		lowerDir := strings.ToLower(dir)
		if strings.Contains(lowerSuite, lowerDir) {
			return dir
		}
	}

	return ""
}

// defaultTestDirs are used when the runner did not write report/.test-dirs.txt;
// without them "rerun: failed" can be persisted with no tests.
var defaultTestDirs = map[string][]string{
	"k3s": {
		"btrfs", "dualstack", "embeddedmirror", "externalip", "multus", "privateregistry",
		"rootless", "rotateca", "s3", "secretsencryption", "secretsencryption_old",
		"splitserver", "startup", "tailscale", "validatecluster", "wasm",
	},
	"rke2": {
		"calico_ebpf", "cilium_wireguard", "ciliumnokp", "clusterloadbalancer", "cni",
		"kine", "kubevip", "mixedos", "mixedosbgp", "multus", "secretsencryption",
		"secretsencryption_old", "splitserver", "upgradecluster", "validatecluster",
	},
}

func getFailedTestDirs(pd *processedTestdata, baseDir, product string) []string {
	var testDirs []string
	testDirsFile := filepath.Join(baseDir, "report", ".test-dirs.txt")
	if data, err := os.ReadFile(testDirsFile); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				testDirs = append(testDirs, line)
			}
		}
	}

	if len(testDirs) == 0 {
		if defaults, ok := defaultTestDirs[strings.ToLower(product)]; ok {
			resources.LogLevel("warn", "%s not found or empty; using built-in %s test directory list",
				testDirsFile, product)
			testDirs = defaults
		}
	}

	if len(testDirs) == 0 {
		return nil
	}

	failedDirs := make(map[string]bool)
	for _, suite := range pd.testSuiteSummary {
		if suite.failedTests > 0 {
			if dir := mapTestSuiteToDir(suite.testSuiteName, testDirs); dir != "" {
				failedDirs[dir] = true
			}
		}
	}

	var result []string
	for dir := range failedDirs {
		result = append(result, dir)
	}
	sort.Strings(result)

	return result
}

//nolint:funlen // yeah complex Slack block message.
func (s *slackClient) PostTestResults(
	pd *processedTestdata,
	product string,
	ciArch string,
	runID int32,
	baseDir string,
	parentThreadTS string,
) (string, error) {
	totalTests := pd.passedTests + pd.failedTests + pd.skippedTests
	statusEmoji := ":white_check_mark:"
	statusText := "PASSED"
	if pd.failedTests > 0 {
		statusEmoji = ":x:"
		statusText = "FAILED"
	}

	// Build title based on architecture.
	// For arm64, specify "ARM Docker" to make it clear these are arm64 docker-only tests.
	var title string
	if ciArch == "arm64" {
		title = fmt.Sprintf("%s E2E ARM Docker Test Results - %s", strings.ToUpper(product), statusText)
	} else {
		title = fmt.Sprintf("%s E2E Test Results - %s", strings.ToUpper(product), statusText)
	}

	blocks := []slackBlock{
		{
			Type: "header",
			Text: &slackBlockText{
				Type: "plain_text",
				Text: title,
			},
		},
		{
			Type: "section",
			Fields: []slackBlockText{
				{Type: "mrkdwn", Text: "*Product:*\n" + strings.ToUpper(product)},
				{Type: "mrkdwn", Text: "*Date:*\n" + pd.testDate},
				{Type: "mrkdwn", Text: fmt.Sprintf("*Total Tests:*\n%d", totalTests)},
				{Type: "mrkdwn", Text: "*Duration:*\n" + pd.totalTestTime},
			},
		},
		{
			Type: "section",
			Fields: []slackBlockText{
				{Type: "mrkdwn", Text: fmt.Sprintf(":white_check_mark: *Passed:* %d", pd.passedTests)},
				{Type: "mrkdwn", Text: fmt.Sprintf(":x: *Failed:* %d", pd.failedTests)},
				{Type: "mrkdwn", Text: fmt.Sprintf(":fast_forward: *Skipped:* %d", pd.skippedTests)},
				{Type: "mrkdwn", Text: fmt.Sprintf("%s *Status:* %s", statusEmoji, statusText)},
			},
		},
	}

	if runID > 0 {
		qaseURL := fmt.Sprintf("https://app.qase.io/run/K3SRKE2/dashboard/%d", runID)
		qaseText := fmt.Sprintf(":link: *Qase Run:* <%s|View Test Run #%d>", qaseURL, runID)
		blocks = append(blocks, slackBlock{
			Type: "section",
			Text: &slackBlockText{Type: "mrkdwn", Text: qaseText},
		})
	}

	if parentThreadTS == "" {
		var actionText string
		if strings.EqualFold(product, "k3s") {
			// k3s uses GitHub Actions for CI - always show link to e2e workflow.
			actionText = ":github: *CI:* " +
				"<https://github.com/k3s-io/k3s/actions/workflows/e2e.yaml|View E2E Workflow>"
		} else {
			// rke2 uses rerun-poller for reruns - only show if test-dirs file exists
			var testNames []string
			testDirsFile := filepath.Join(baseDir, "report", ".test-dirs.txt")
			if data, err := os.ReadFile(testDirsFile); err == nil {
				for _, line := range strings.Split(string(data), "\n") {
					line = strings.TrimSpace(line)
					if line != "" {
						testNames = append(testNames, line)
					}
				}
			}
			if len(testNames) > 0 {
				actionText = ":repeat: *To rerun failed tests*, reply with: `rerun: failed`"
			}
		}
		if actionText != "" {
			blocks = append(blocks, slackBlock{
				Type: "section",
				Text: &slackBlockText{
					Type: "mrkdwn",
					Text: actionText,
				},
			})
		}
	}

	if pd.failedTests > 0 {
		blocks = append(blocks, slackBlock{Type: "divider"})

		var failedTestsList strings.Builder
		failedTestsList.WriteString("*Failed Tests:*\n")
		for _, suite := range pd.testSummary {
			for _, tc := range suite.testCases {
				if tc.status == failStatus {
					failedTestsList.WriteString(fmt.Sprintf("• %s / %s\n", tc.testSuiteName, tc.testCaseName))
				}
			}
		}
		blocks = append(blocks, slackBlock{
			Type: "section",
			Text: &slackBlockText{Type: "mrkdwn", Text: failedTestsList.String()},
		})
	}

	blocks = append(blocks, slackBlock{Type: "divider"})

	var suiteSummary strings.Builder
	suiteSummary.WriteString("*Test Suite Summary:*\n")
	for _, suite := range pd.testSuiteSummary {
		suiteEmoji := ":white_check_mark:"
		if suite.failedTests > 0 {
			suiteEmoji = ":x:"
		}
		suiteSummary.WriteString(fmt.Sprintf("%s %s\n",
			suiteEmoji, suite.testSuiteName))
	}
	blocks = append(blocks, slackBlock{
		Type: "section",
		Text: &slackBlockText{Type: "mrkdwn", Text: suiteSummary.String()},
	})

	summaryText := fmt.Sprintf("%s E2E Test Results: %d passed, %d failed, %d skipped",
		strings.ToUpper(product), pd.passedTests, pd.failedTests, pd.skippedTests)
	msg := slackMessage{
		Channel:  s.channelID,
		Text:     summaryText,
		Blocks:   blocks,
		ThreadTS: parentThreadTS,
	}

	return s.sendMessage(msg)
}

// Slack limits chat.postMessage to 50 blocks and each section to 3000 characters.
// Oversized failure details are rejected without posting anything to the thread.
const (
	slackMaxBlocksPerMessage = 50
	slackMaxSectionText      = 3000
)

func truncateSlackText(text string, maxRunes int, suffix string) string {
	if utf8.RuneCountInString(text) <= maxRunes {
		return text
	}

	suffixRunes := []rune(suffix)
	if len(suffixRunes) >= maxRunes {
		return string(suffixRunes[:maxRunes])
	}
	runes := []rune(text)

	return string(runes[:maxRunes-len(suffixRunes)]) + suffix
}

func failureCodeSection(label, content string) string {
	prefix, closing := label+"\n```", "```"
	contentLimit := slackMaxSectionText - utf8.RuneCountInString(prefix+closing)
	content = truncateSlackText(content, contentLimit, "\n... (truncated)")

	return prefix + content + closing
}

// failureBlocks renders one failure as its Slack blocks (2 to 4 blocks).
func failureBlocks(idx int, failure *FailureDetails) []slackBlock {
	blocks := []slackBlock{{Type: "divider"}}

	testInfo := fmt.Sprintf(":x: *%d. %s*\n", idx, failure.TestSuite)
	testInfo += fmt.Sprintf("*Subtest:* %s\n", failure.TestCase)
	testInfo += fmt.Sprintf("*Duration:* %.2fs", failure.Duration)
	if failure.TimeoutDuration != "" {
		testInfo += fmt.Sprintf(" (timed out after %s)", failure.TimeoutDuration)
	}
	testInfo += "\n*Error Type:* " + failure.ErrorType
	testInfo = truncateSlackText(testInfo, slackMaxSectionText, "\n... (truncated)")

	blocks = append(blocks, slackBlock{
		Type: "section",
		Text: &slackBlockText{Type: "mrkdwn", Text: testInfo},
	})

	if failure.FailedCommand != "" {
		cmd := failureCodeSection("*Failed Command:*", failure.FailedCommand)
		blocks = append(blocks, slackBlock{
			Type: "section",
			Text: &slackBlockText{Type: "mrkdwn", Text: cmd},
		})
	}

	if failure.ErrorMessage != "" {
		errMsg := failureCodeSection("*Error Output:*", failure.ErrorMessage)
		blocks = append(blocks, slackBlock{
			Type: "section",
			Text: &slackBlockText{Type: "mrkdwn", Text: errMsg},
		})
	}

	return blocks
}

// chunkFailureBlocks splits the failure details into messages that each stay
// under Slack's block limit. Every chunk starts with a header block.
func chunkFailureBlocks(failures []*FailureDetails, maxBlocks int) [][]slackBlock {
	header := func(part, total int) slackBlock {
		text := "Failure Details"
		if total > 1 {
			text = fmt.Sprintf("Failure Details (%d/%d)", part, total)
		}

		return slackBlock{Type: "header", Text: &slackBlockText{Type: "plain_text", Text: text}}
	}

	// first pass: group failures so that header + blocks <= maxBlocks.
	var groups [][]slackBlock
	var current []slackBlock
	for i, f := range failures {
		fb := failureBlocks(i+1, f)
		if len(current) > 0 && 1+len(current)+len(fb) > maxBlocks {
			groups = append(groups, current)
			current = nil
		}
		current = append(current, fb...)
	}
	if len(current) > 0 {
		groups = append(groups, current)
	}

	// second pass: prepend the numbered header now that the total is known.
	chunks := make([][]slackBlock, 0, len(groups))
	for i, g := range groups {
		chunks = append(chunks, append([]slackBlock{header(i+1, len(groups))}, g...))
	}

	return chunks
}

// PostFailureDetails posts detailed failure information as thread replies,
// split into as many messages as Slack's block limit requires.
func (s *slackClient) PostFailureDetails(pd *processedTestdata, threadTS string) error {
	failures := pd.GetFailedTestDetails()
	resources.LogLevel("info", "PostFailureDetails: found %d failure details", len(failures))
	if len(failures) == 0 {
		return nil
	}

	chunks := chunkFailureBlocks(failures, slackMaxBlocksPerMessage)
	resources.LogLevel("info", "Posting detailed failure information for %d failed tests in %d message(s)",
		len(failures), len(chunks))

	for i, blocks := range chunks {
		fallbackText := "Failure Details"
		if len(chunks) > 1 {
			fallbackText = fmt.Sprintf("Failure Details (%d/%d)", i+1, len(chunks))
		}
		msg := slackMessage{
			Channel:  s.channelID,
			Text:     fallbackText,
			Blocks:   blocks,
			ThreadTS: threadTS,
		}
		if _, err := s.sendMessage(msg); err != nil {
			return fmt.Errorf("failure details message %d/%d (%d blocks): %w", i+1, len(chunks), len(blocks), err)
		}
	}

	return nil
}

// postFailureFallback attempts a compact plain-text diagnosis when Slack
// rejects the block message.
func (s *slackClient) postFailureFallback(pd *processedTestdata, threadTS string, cause error) error {
	failures := pd.GetFailedTestDetails()
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf(":warning: Detailed failure blocks could not be posted (%v). Summary:\n", cause))
	for i, f := range failures {
		line := strings.TrimSpace(strings.SplitN(f.ErrorMessage, "\n", 2)[0])
		line = truncateSlackText(line, 160, "...")
		sb.WriteString(fmt.Sprintf("%d. %s / %s — %s: %s\n", i+1, f.TestSuite, f.TestCase, f.ErrorType, line))
		if sb.Len() > 3500 {
			sb.WriteString(fmt.Sprintf("... (%d more)\n", len(failures)-i-1))
			break
		}
	}

	_, err := s.sendMessage(slackMessage{Channel: s.channelID, Text: sb.String(), ThreadTS: threadTS})

	return err
}

func (s *slackClient) sendMessage(msg slackMessage) (string, error) {
	if s.dryRun {
		payload, _ := json.MarshalIndent(msg, "", "  ")
		resources.LogLevel("info", "[DRY RUN] Would send Slack message:\n%s", string(payload))
		return "dry-run-thread-ts", nil
	}

	payload, err := json.Marshal(msg)
	if err != nil {
		return "", fmt.Errorf("failed to marshal message: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, slackPostMsgURL, bytes.NewBuffer(payload))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+s.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("network error posting to Slack: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	var postResp slackPostResponse
	if err := json.Unmarshal(body, &postResp); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if !postResp.OK {
		return "", fmt.Errorf("failed to post to Slack: %s", postResp.Error)
	}

	resources.LogLevel("info", "Message posted to Slack successfully (ts: %s)", postResp.TS)

	return postResp.TS, nil
}

func saveRerunState(baseDir, channelID, threadTS, product string, failedTests []string) error {
	state := map[string]interface{}{
		"thread_ts":         threadTS,
		"channel_id":        channelID,
		"last_processed_ts": threadTS,
		"product":           product,
		"posted_at":         time.Now().Format(time.RFC3339),
		"failed_tests":      failedTests,
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	statePath := filepath.Join(baseDir, "report", ".rerun-state.json")
	tmpPath := statePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o600); err != nil {
		return fmt.Errorf("failed to write temp state file: %w", err)
	}
	if err := os.Rename(tmpPath, statePath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to rename state file: %w", err)
	}

	resources.LogLevel("info", "Saved rerun state: thread_ts=%s, failed_tests=%v", threadTS, failedTests)

	return nil
}

func updateFailedTests(baseDir string, failedTests []string) error {
	statePath := filepath.Join(baseDir, "report", ".rerun-state.json")

	data, err := os.ReadFile(statePath)
	if err != nil {
		return fmt.Errorf("failed to read state file: %w", err)
	}

	var state map[string]interface{}
	if err := json.Unmarshal(data, &state); err != nil {
		return fmt.Errorf("failed to parse state file: %w", err)
	}

	state["failed_tests"] = failedTests
	state["posted_at"] = time.Now().Format(time.RFC3339)

	newData, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	tmpPath := statePath + ".tmp"
	if err := os.WriteFile(tmpPath, newData, 0o600); err != nil {
		return fmt.Errorf("failed to write temp state file: %w", err)
	}
	if err := os.Rename(tmpPath, statePath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to rename state file: %w", err)
	}

	resources.LogLevel("info", "Updated failed_tests in state: %v", failedTests)

	return nil
}
