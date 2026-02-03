package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/rancher/distros-test-framework/shared"
)

const (
	lockFile       = "/tmp/e2e-tests.lock"
	runTestsScript = "/root/run_tests.sh"
	pollLockFile   = "/tmp/rerun-poller.lock"
)

type rerunState struct {
	ChannelID       string   `json:"channel_id"`
	ThreadTS        string   `json:"thread_ts"`
	LastProcessedTS string   `json:"last_processed_ts"`
	FailedTests     []string `json:"failed_tests"`
	Product         string   `json:"product"`
	PostedAt        string   `json:"posted_at"`
}

type slackMessage struct {
	TS   string `json:"ts"`
	User string `json:"user"`
	Text string `json:"text"`
}

type slackRepliesResponse struct {
	OK       bool           `json:"ok"`
	Error    string         `json:"error,omitempty"`
	Messages []slackMessage `json:"messages"`
}

func main() {
	shared.LogLevel("info", "========================================")
	shared.LogLevel("info", "Rerun Poller Starting")
	shared.LogLevel("info", "========================================")

	if !acquirePollerLock() {
		os.Exit(0)
	}
	defer releasePollerLock()

	if running, reason := isTestsRunning(); running {
		shared.LogLevel("info", "Tests already running (%s) - skipping this poll cycle", reason)
		return //nolint:gocritic // intentional exit, defer will run on normal return
	}

	state, stateFile, slackToken := initializePoller()
	rerunMsg, rerunTests := findRerunRequest(state, slackToken)

	if rerunMsg == nil {
		shared.LogLevel("info", "No rerun request found in new messages - nothing to do")
		return
	}

	processRerunRequest(state, stateFile, slackToken, rerunMsg, rerunTests)
}

func initializePoller() (state *rerunState, stateFile, slackToken string) {
	slackToken = os.Getenv("SLACK_TOKEN")
	if slackToken == "" {
		shared.LogLevel("error", "SLACK_TOKEN environment variable not set")
		os.Exit(1)
	}
	shared.LogLevel("info", "Slack token loaded (length: %d)", len(slackToken))

	baseDir := os.Getenv("DISTROS_BASE_DIR")
	if baseDir == "" {
		baseDir = "/root/distros-test-framework"
	}
	stateFile = filepath.Join(baseDir, "report", ".rerun-state.json")
	shared.LogLevel("info", "State file: %s", stateFile)

	var err error
	state, err = loadState(stateFile)
	if err != nil {
		if os.IsNotExist(err) {
			shared.LogLevel("info", "No state file - nothing to poll")
		} else {
			shared.LogLevel("error", "Failed to load state: %v", err)
		}
		os.Exit(0)
	}
	shared.LogLevel("info", "Loaded state: thread=%s, channel=%s, last_ts=%s",
		state.ThreadTS, state.ChannelID, state.LastProcessedTS)
	shared.LogLevel("info", "Failed tests from last run: %v", state.FailedTests)

	return state, stateFile, slackToken
}

func findRerunRequest(state *rerunState, slackToken string) (rerunMsg *slackMessage, rerunTests string) {
	shared.LogLevel("info", "Fetching replies from Slack...")
	replies, err := fetchSlackReplies(slackToken, state.ChannelID, state.ThreadTS)
	if err != nil {
		shared.LogLevel("error", "Failed to fetch Slack replies: %v", err)
		os.Exit(1)
	}
	shared.LogLevel("info", "Fetched %d messages from thread", len(replies.Messages))

	for i := len(replies.Messages) - 1; i >= 0; i-- {
		msg := replies.Messages[i]

		if msg.TS == state.ThreadTS {
			continue
		}

		if msg.TS <= state.LastProcessedTS {
			continue
		}

		tests := parseRerunRequest(msg.Text)
		if tests != "" {
			isMember, err := isChannelMember(slackToken, state.ChannelID, msg.User)
			if err != nil {
				shared.LogLevel("warn", "Failed to check channel membership for user %s: %v", msg.User, err)
				continue
			}
			if !isMember {
				shared.LogLevel("warn", "Unauthorized rerun request from user %s (not a channel member)", msg.User)
				continue
			}

			rerunMsg = &msg
			rerunTests = tests
			shared.LogLevel("info", "Found rerun request: ts=%s, user=%s, tests='%s'", msg.TS, msg.User, tests)

			break
		}
		shared.LogLevel("info", "Skipping non-rerun message: ts=%s, text='%s'", msg.TS, msg.Text)
	}

	return rerunMsg, rerunTests
}

func processRerunRequest(state *rerunState, stateFile, slackToken string, rerunMsg *slackMessage, rerunTests string) {
	//  add timestamp BEFORE executing to prevent duplicate triggers.
	oldTS := state.LastProcessedTS
	state.LastProcessedTS = rerunMsg.TS
	if err := saveState(stateFile, state); err != nil {
		shared.LogLevel("error", "CRITICAL: Failed to save state: %v", err)
		shared.LogLevel("error", "Aborting to prevent duplicate triggers")
		os.Exit(1)
	}
	shared.LogLevel("info", "SAFETY: Updated last_processed_ts from %s to %s BEFORE processing", oldTS, rerunMsg.TS)

	// map "failed" to actual test names.
	if rerunTests == "failed" {
		if len(state.FailedTests) == 0 {
			shared.LogLevel("warn", "User requested 'rerun: failed' but no failed tests recorded")
			_ = postToSlack(slackToken, state.ChannelID, state.ThreadTS,
				":warning: No failed tests recorded from the last run.")
			os.Exit(0)
		}
		rerunTests = strings.Join(state.FailedTests, ",")
		shared.LogLevel("info", "Resolved 'failed' to: %s", rerunTests)
	}

	// check tests aren't running now
	if running, reason := isTestsRunning(); running {
		shared.LogLevel("warn", "Tests started running between checks (%s) - aborting", reason)
		os.Exit(0)
	}

	if err := executeRerun(rerunTests, slackToken, state.ChannelID, state.ThreadTS, rerunMsg.User); err != nil {
		shared.LogLevel("error", "Rerun failed: %v", err)
		os.Exit(1)
	}
}

func acquirePollerLock() bool {
	if data, err := os.ReadFile(pollLockFile); err == nil {
		pidStr := strings.TrimSpace(string(data))
		if pid, err := strconv.Atoi(pidStr); err == nil && pid > 0 {
			if processExists(pid) {
				shared.LogLevel("info", "Another poller instance running (PID %d) - exiting", pid)
				return false
			}
		}
		shared.LogLevel("warn", "Removing stale poller lock (PID was %s)", pidStr)
		_ = os.Remove(pollLockFile)
	}

	if err := os.WriteFile(pollLockFile, []byte(strconv.Itoa(os.Getpid())), 0o644); err != nil {
		shared.LogLevel("error", "Failed to create poller lock: %v", err)
		return false
	}
	shared.LogLevel("info", "Acquired poller lock (PID %d)", os.Getpid())

	return true
}

func releasePollerLock() {
	_ = os.Remove(pollLockFile)
	shared.LogLevel("info", "Released poller lock")
}

func isTestsRunning() (running bool, reason string) {
	if data, err := os.ReadFile(lockFile); err == nil {
		pidStr := strings.TrimSpace(string(data))
		if pid, err := strconv.Atoi(pidStr); err == nil && pid > 0 {
			if processExists(pid) {
				return true, fmt.Sprintf("lock file PID %d is alive", pid)
			}
			shared.LogLevel("warn", "Stale test lock file found (PID %d dead), removing", pid)
			_ = os.Remove(lockFile)
		}
	}

	if out, _ := exec.Command("pgrep", "-f", "go test.*_test.go").Output(); len(out) > 0 {
		pids := strings.TrimSpace(string(out))
		return true, "go test processes running: " + pids
	}

	if out, _ := exec.Command("pgrep", "-f", "run_tests.sh").Output(); len(out) > 0 {
		pids := strings.TrimSpace(string(out))
		return true, "run_tests.sh running: " + pids
	}

	if out, _ := exec.Command("pgrep", "-f", "vagrant up").Output(); len(out) > 0 {
		pids := strings.TrimSpace(string(out))
		return true, "vagrant up running: " + pids
	}

	return false, ""
}

func processExists(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))

	return err == nil
}

func loadState(stateFile string) (*rerunState, error) {
	data, err := os.ReadFile(stateFile)
	if err != nil {
		return nil, fmt.Errorf("cannot read state file: %w", err)
	}

	var state rerunState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("cannot parse state file: %w", err)
	}

	if state.ThreadTS == "" {
		return nil, errors.New("state file missing thread_ts")
	}
	if state.ChannelID == "" {
		return nil, errors.New("state file missing channel_id")
	}

	return &state, nil
}

func saveState(stateFile string, state *rerunState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("cannot marshal state: %w", err)
	}

	tmpFile := stateFile + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0o644); err != nil {
		return fmt.Errorf("cannot write temp state: %w", err)
	}
	if err := os.Rename(tmpFile, stateFile); err != nil {
		_ = os.Remove(tmpFile)
		return fmt.Errorf("cannot rename state file: %w", err)
	}

	return nil
}

func fetchSlackReplies(token, channelID, threadTS string) (*slackRepliesResponse, error) {
	url := fmt.Sprintf("https://slack.com/api/conversations.replies?channel=%s&ts=%s", channelID, threadTS)

	req, err := http.NewRequest(http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("cannot create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("cannot read response: %w", err)
	}

	var result slackRepliesResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("cannot parse response: %w", err)
	}

	if !result.OK {
		return nil, fmt.Errorf("slack API error: %s", result.Error)
	}

	return &result, nil
}

// isChannelMember checks if a user is a member of the specified channel.
// Handles pagination for channels with more than 1000 members.
func isChannelMember(token, channelID, userID string) (bool, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	cursor := ""

	for {
		url := fmt.Sprintf("https://slack.com/api/conversations.members?channel=%s&limit=1000", channelID)
		if cursor != "" {
			url += "&cursor=" + cursor
		}

		req, err := http.NewRequest(http.MethodGet, url, http.NoBody)
		if err != nil {
			return false, fmt.Errorf("cannot create request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := client.Do(req)
		if err != nil {
			return false, fmt.Errorf("request failed: %w", err)
		}

		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			return false, fmt.Errorf("cannot read response: %w", err)
		}

		var result struct {
			OK               bool     `json:"ok"`
			Error            string   `json:"error,omitempty"`
			Members          []string `json:"members"`
			ResponseMetadata struct {
				NextCursor string `json:"next_cursor"`
			} `json:"response_metadata"`
		}
		if err := json.Unmarshal(body, &result); err != nil {
			return false, fmt.Errorf("cannot parse response: %w", err)
		}

		if !result.OK {
			return false, fmt.Errorf("slack API error: %s", result.Error)
		}

		for _, member := range result.Members {
			if member == userID {
				return true, nil
			}
		}

		// Check if there are more pages.
		if result.ResponseMetadata.NextCursor == "" {
			break
		}
		cursor = result.ResponseMetadata.NextCursor
	}

	return false, nil
}

func postToSlack(token, channelID, threadTS, text string) error {
	payload := map[string]string{
		"channel":   channelID,
		"thread_ts": threadTS,
		"text":      text,
	}
	data, _ := json.Marshal(payload)

	req, err := http.NewRequest(http.MethodPost, "https://slack.com/api/chat.postMessage", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("cannot create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("cannot read response: %w", err)
	}

	var result struct {
		OK    bool   `json:"ok"`
		Error string `json:"error,omitempty"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("cannot parse response: %w", err)
	}

	if !result.OK {
		return fmt.Errorf("slack API error: %s", result.Error)
	}

	return nil
}

func parseRerunRequest(text string) string {
	text = strings.TrimSpace(text)
	lower := strings.ToLower(text)

	if !strings.HasPrefix(lower, "rerun:") {
		return ""
	}

	tests := strings.TrimSpace(text[6:])
	tests = strings.ToLower(tests)

	if tests == "failed" {
		return tests
	}

	return ""
}

func executeRerun(tests, slackToken, channelID, threadTS, user string) error {
	shared.LogLevel("info", "========================================")
	shared.LogLevel("info", "STARTING RERUN")
	shared.LogLevel("info", "  Tests: %s", tests)
	shared.LogLevel("info", "  Requested by: %s", user)
	shared.LogLevel("info", "  Thread: %s", threadTS)
	shared.LogLevel("info", "========================================")

	msg := fmt.Sprintf(":arrows_counterclockwise: Starting rerun of: `%s`\nRequested by: <@%s>", tests, user)
	if err := postToSlack(slackToken, channelID, threadTS, msg); err != nil {
		shared.LogLevel("warn", "Failed to post start message to Slack: %v", err)
	}

	if _, err := os.Stat(runTestsScript); os.IsNotExist(err) {
		return fmt.Errorf("run_tests.sh not found at %s", runTestsScript)
	}

	cmd := exec.Command(runTestsScript, "-test", tests)
	cmd.Env = append(os.Environ(), "IS_RERUN=true")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	startTime := time.Now()
	err := cmd.Run()
	duration := time.Since(startTime)

	if err != nil {
		exitCode := 1
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
		shared.LogLevel("error", "Rerun FAILED after %v (exit code %d)", duration, exitCode)

		msg := fmt.Sprintf(":x: Rerun failed for: `%s`\nExit code: %d\nDuration: %v",
			tests, exitCode, duration.Round(time.Second))
		_ = postToSlack(slackToken, channelID, threadTS, msg)

		return fmt.Errorf("tests failed with exit code %d", exitCode)
	}

	shared.LogLevel("info", "Rerun COMPLETED successfully in %v", duration.Round(time.Second))

	return nil
}
