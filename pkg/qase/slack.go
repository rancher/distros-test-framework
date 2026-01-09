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
	"time"

	"github.com/rancher/distros-test-framework/shared"
)

const (
	slackAuthURL    = "https://slack.com/api/auth.test"
	slackPostMsgURL = "https://slack.com/api/chat.postMessage"
)

type SlackClient struct {
	Token     string
	ChannelID string
	client    *http.Client
}

type SlackAuthResponse struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
	User  string `json:"user,omitempty"`
	Team  string `json:"team,omitempty"`
}

type SlackPostResponse struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
	TS    string `json:"ts,omitempty"`
}

type SlackMessage struct {
	Channel  string       `json:"channel"`
	Text     string       `json:"text,omitempty"`
	Blocks   []SlackBlock `json:"blocks,omitempty"`
	ThreadTS string       `json:"thread_ts,omitempty"`
}

type SlackBlock struct {
	Type   string           `json:"type"`
	Text   *SlackBlockText  `json:"text,omitempty"`
	Fields []SlackBlockText `json:"fields,omitempty"`
}

type SlackBlockText struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func NewSlackClient() (*SlackClient, error) {
	token := os.Getenv("SLACK_TOKEN")
	channelID := os.Getenv("SLACK_CHANNEL_ID")

	if token == "" {
		return nil, fmt.Errorf("SLACK_TOKEN environment variable is not set")
	}
	if channelID == "" {
		return nil, fmt.Errorf("SLACK_CHANNEL_ID environment variable is not set")
	}

	return &SlackClient{
		Token:     token,
		ChannelID: channelID,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

func ReportToSlack(fileName, product, baseDir string, runID int32) error {
	slackClient, err := NewSlackClient()
	if err != nil {
		shared.LogLevel("warn", "Slack client not configured: %v", err)
		return err
	}

	if err := slackClient.ValidateConnection(); err != nil {
		return fmt.Errorf("Slack connection validation failed: %w", err)
	}

	pd, err := processTestData(fileName, product)
	if err != nil {
		return fmt.Errorf("error processing test data for Slack: %w", err)
	}

	failedDirs := getFailedTestDirs(pd, baseDir)

	var parentThreadTS string
	isRerun := os.Getenv("IS_RERUN") == "true"
	statePath := filepath.Join(baseDir, "report", ".rerun-state.json")
	
	if isRerun {
		if data, err := os.ReadFile(statePath); err == nil {
			var state map[string]interface{}
			if json.Unmarshal(data, &state) == nil {
				if ts, ok := state["thread_ts"].(string); ok && ts != "" {
					parentThreadTS = ts
					shared.LogLevel("info", "Rerun detected, will post as reply to thread: %s", parentThreadTS)
				}
			}
		}
	} else {
		shared.LogLevel("info", "Fresh run detected, will create new Slack thread")
	}

	threadTS, err := slackClient.PostTestResults(pd, product, runID, baseDir, parentThreadTS)
	if err != nil {
		return fmt.Errorf("error posting test results to Slack: %w", err)
	}

	if baseDir != "" && threadTS != "" {
		if parentThreadTS == "" {
			if err := saveRerunState(baseDir, slackClient.ChannelID, threadTS, product, failedDirs); err != nil {
				shared.LogLevel("warn", "Failed to save rerun state: %v", err)
			}
		} else {
			if err := updateFailedTests(baseDir, failedDirs); err != nil {
				shared.LogLevel("warn", "Failed to update failed tests: %v", err)
			}
		}
	}

	return nil
}

func (s *SlackClient) ValidateConnection() error {
	shared.LogLevel("info", "Validating Slack connection...")

	req, err := http.NewRequest("GET", slackAuthURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+s.Token)

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("network error validating Slack: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	var authResp SlackAuthResponse
	if err := json.Unmarshal(body, &authResp); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	if !authResp.OK {
		if authResp.Error == "invalid_auth" {
			return fmt.Errorf("Slack token validation failed: invalid_auth")
		}
		return fmt.Errorf("Slack token validation failed: %s", authResp.Error)
	}

	shared.LogLevel("info", "Connected to Slack as: %s (Team: %s)", authResp.User, authResp.Team)

	return nil
}

func (s *SlackClient) PostMessage(text string) error {
	msg := SlackMessage{
		Channel: s.ChannelID,
		Text:    text,
	}
	_, err := s.sendMessage(msg)

	return err
}

func mapTestSuiteToDir(suiteName string, testDirs []string) string {
	lowerSuite := strings.ToLower(suiteName)

	// Direct mappings for special cases where dir name does not match suite name
	specialMappings := map[string]string{
		"upgradevalidation":    "upgradecluster",
		"clustervalidation":    "validatecluster",
		"ciliumwireguard":      "cilium_wireguard",
		"secretsencryptionold": "secretsencryption_old",
		"kinevalidation":       "kine",
		"mixedosbgpvalidation": "mixedosbgp",
	}

	// Check special mappings first
	for suiteKey, dirName := range specialMappings {
		if strings.Contains(lowerSuite, suiteKey) {
			for _, dir := range testDirs {
				if strings.ToLower(dir) == dirName {
					return dir
				}
			}
		}
	}

	// Fall back to original logic
	for _, dir := range testDirs {
		lowerDir := strings.ToLower(dir)
		if strings.Contains(lowerSuite, lowerDir) {
			return dir
		}
	}

	return ""
}
func getFailedTestDirs(pd *processedTestdata, baseDir string) []string {
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
	return result
}

func (s *SlackClient) PostTestResults(pd *processedTestdata, product string, runID int32, baseDir string, parentThreadTS string) (string, error) {
	totalTests := pd.passedTests + pd.failedTests + pd.skippedTests
	statusEmoji := ":white_check_mark:"
	statusText := "PASSED"
	if pd.failedTests > 0 {
		statusEmoji = ":x:"
		statusText = "FAILED"
	}

	blocks := []SlackBlock{
		{
			Type: "header",
			Text: &SlackBlockText{
				Type: "plain_text",
				Text: fmt.Sprintf("%s E2E Test Results - %s", strings.ToUpper(product), statusText),
			},
		},
		{
			Type: "section",
			Fields: []SlackBlockText{
				{Type: "mrkdwn", Text: fmt.Sprintf("*Product:*\n%s", strings.ToUpper(product))},
				{Type: "mrkdwn", Text: fmt.Sprintf("*Date:*\n%s", pd.testDate)},
				{Type: "mrkdwn", Text: fmt.Sprintf("*Total Tests:*\n%d", totalTests)},
				{Type: "mrkdwn", Text: fmt.Sprintf("*Duration:*\n%s", pd.totalTestTime)},
			},
		},
		{
			Type: "section",
			Fields: []SlackBlockText{
				{Type: "mrkdwn", Text: fmt.Sprintf(":white_check_mark: *Passed:* %d", pd.passedTests)},
				{Type: "mrkdwn", Text: fmt.Sprintf(":x: *Failed:* %d", pd.failedTests)},
				{Type: "mrkdwn", Text: fmt.Sprintf(":fast_forward: *Skipped:* %d", pd.skippedTests)},
				{Type: "mrkdwn", Text: fmt.Sprintf("%s *Status:* %s", statusEmoji, statusText)},
			},
		},
	}

	if runID > 0 {
		qaseURL := fmt.Sprintf("https://app.qase.io/run/K3SRKE2/dashboard/%d", runID)
		blocks = append(blocks, SlackBlock{
			Type: "section",
			Text: &SlackBlockText{Type: "mrkdwn", Text: fmt.Sprintf(":link: *Qase Run:* <%s|View Test Run #%d>", qaseURL, runID)},
		})
	}

	if parentThreadTS == "" {
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
			rerunText := ":repeat: *To rerun tests*, reply to this thread with:\n"
			rerunText += "`rerun: all` - rerun all tests\n"
			rerunText += "`rerun: failed` - rerun only failed tests\n"
			// Get only the failed test directories for specific rerun option
			failedTestDirs := getFailedTestDirs(pd, baseDir)
			if len(failedTestDirs) > 0 {
				rerunText += "`rerun: " + strings.Join(failedTestDirs, ", ") + "` - rerun specific failed tests\n\n"
			}
			rerunText += "_available tests:_ `" + strings.Join(testNames, "`, `") + "`"
			blocks = append(blocks, SlackBlock{
				Type: "section",
				Text: &SlackBlockText{
					Type: "mrkdwn",
					Text: rerunText,
				},
			})
		}
	}

	if pd.failedTests > 0 {
		blocks = append(blocks, SlackBlock{Type: "divider"})

		var failedTestsList strings.Builder
		failedTestsList.WriteString("*Failed Tests:*\n")
		for _, suite := range pd.testSummary {
			for _, tc := range suite.testCases {
				if tc.status == failStatus {
					failedTestsList.WriteString(fmt.Sprintf("• %s / %s\n", tc.testSuiteName, tc.testCaseName))
				}
			}
		}
		blocks = append(blocks, SlackBlock{
			Type: "section",
			Text: &SlackBlockText{Type: "mrkdwn", Text: failedTestsList.String()},
		})
	}

	blocks = append(blocks, SlackBlock{Type: "divider"})

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
	blocks = append(blocks, SlackBlock{
		Type: "section",
		Text: &SlackBlockText{Type: "mrkdwn", Text: suiteSummary.String()},
	})

	msg := SlackMessage{
		Channel:  s.ChannelID,
		Text:     fmt.Sprintf("%s E2E Test Results: %d passed, %d failed, %d skipped", strings.ToUpper(product), pd.passedTests, pd.failedTests, pd.skippedTests),
		Blocks:   blocks,
		ThreadTS: parentThreadTS,
	}

	return s.sendMessage(msg)
}

func (s *SlackClient) sendMessage(msg SlackMessage) (string, error) {
	payload, err := json.Marshal(msg)
	if err != nil {
		return "", fmt.Errorf("failed to marshal message: %w", err)
	}

	req, err := http.NewRequest("POST", slackPostMsgURL, bytes.NewBuffer(payload))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+s.Token)
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

	var postResp SlackPostResponse
	if err := json.Unmarshal(body, &postResp); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if !postResp.OK {
		return "", fmt.Errorf("failed to post to Slack: %s", postResp.Error)
	}

	shared.LogLevel("info", "Message posted to Slack successfully (ts: %s)", postResp.TS)
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
	if err := os.WriteFile(statePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write state file: %w", err)
	}

	shared.LogLevel("info", "Saved rerun state: thread_ts=%s, failed_tests=%v", threadTS, failedTests)
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
	
	if err := os.WriteFile(statePath, newData, 0644); err != nil {
		return fmt.Errorf("failed to write state file: %w", err)
	}
	
	shared.LogLevel("info", "Updated failed_tests in state: %v", failedTests)
	return nil
}
 