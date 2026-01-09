package main

import (
	"bytes"
	"encoding/json"
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
)

const (
	LockFile       = "/tmp/e2e-tests.lock"
	RunTestsScript = "/root/run_tests.sh"
	PollLockFile   = "/tmp/rerun-poller.lock"
)

type RerunState struct {
	ChannelID       string   `json:"channel_id"`
	ThreadTS        string   `json:"thread_ts"`
	LastProcessedTS string   `json:"last_processed_ts"`
	FailedTests     []string `json:"failed_tests"`
	Product         string   `json:"product"`
	PostedAt        string   `json:"posted_at"`
}

type SlackMessage struct {
	TS   string `json:"ts"`
	User string `json:"user"`
	Text string `json:"text"`
}

type SlackRepliesResponse struct {
	OK       bool           `json:"ok"`
	Error    string         `json:"error,omitempty"`
	Messages []SlackMessage `json:"messages"`
}

func log(level, msg string, args ...interface{}) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	formatted := fmt.Sprintf(msg, args...)
	fmt.Printf("%s [rerun-poller] [%s] %s\n", timestamp, level, formatted)
}

func logInfo(msg string, args ...interface{})  { log("INFO", msg, args...) }
func logWarn(msg string, args ...interface{})  { log("WARN", msg, args...) }
func logError(msg string, args ...interface{}) { log("ERROR", msg, args...) }

func acquirePollerLock() bool {
	if data, err := os.ReadFile(PollLockFile); err == nil {
		pidStr := strings.TrimSpace(string(data))
		if pid, err := strconv.Atoi(pidStr); err == nil && pid > 0 {
			if processExists(pid) {
				logInfo("Another poller instance running (PID %d) - exiting", pid)
				return false
			}
		}
		logWarn("Removing stale poller lock (PID was %s)", pidStr)
		os.Remove(PollLockFile)
	}

	if err := os.WriteFile(PollLockFile, []byte(strconv.Itoa(os.Getpid())), 0644); err != nil {
		logError("Failed to create poller lock: %v", err)
		return false
	}
	logInfo("Acquired poller lock (PID %d)", os.Getpid())
	return true
}

func releasePollerLock() {
	os.Remove(PollLockFile)
	logInfo("Released poller lock")
}

func isTestsRunning() (bool, string) {
	if data, err := os.ReadFile(LockFile); err == nil {
		pidStr := strings.TrimSpace(string(data))
		if pid, err := strconv.Atoi(pidStr); err == nil && pid > 0 {
			if processExists(pid) {
				return true, fmt.Sprintf("lock file PID %d is alive", pid)
			}
			logWarn("Stale test lock file found (PID %d dead), removing", pid)
			os.Remove(LockFile)
		}
	}

	if out, _ := exec.Command("pgrep", "-f", "go test.*_test.go").Output(); len(out) > 0 {
		pids := strings.TrimSpace(string(out))
		return true, fmt.Sprintf("go test processes running: %s", pids)
	}

	if out, _ := exec.Command("pgrep", "-f", "run_tests.sh").Output(); len(out) > 0 {
		pids := strings.TrimSpace(string(out))
		return true, fmt.Sprintf("run_tests.sh running: %s", pids)
	}

	if out, _ := exec.Command("pgrep", "-f", "vagrant up").Output(); len(out) > 0 {
		pids := strings.TrimSpace(string(out))
		return true, fmt.Sprintf("vagrant up running: %s", pids)
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

func loadState(stateFile string) (*RerunState, error) {
	data, err := os.ReadFile(stateFile)
	if err != nil {
		return nil, fmt.Errorf("cannot read state file: %w", err)
	}

	var state RerunState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("cannot parse state file: %w", err)
	}

	if state.ThreadTS == "" {
		return nil, fmt.Errorf("state file missing thread_ts")
	}
	if state.ChannelID == "" {
		return nil, fmt.Errorf("state file missing channel_id")
	}

	return &state, nil
}

func saveState(stateFile string, state *RerunState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("cannot marshal state: %w", err)
	}

	tmpFile := stateFile + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return fmt.Errorf("cannot write temp state: %w", err)
	}
	if err := os.Rename(tmpFile, stateFile); err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("cannot rename state file: %w", err)
	}

	return nil
}

func fetchSlackReplies(token, channelID, threadTS string) (*SlackRepliesResponse, error) {
	url := fmt.Sprintf("https://slack.com/api/conversations.replies?channel=%s&ts=%s", channelID, threadTS)

	req, err := http.NewRequest("GET", url, nil)
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

	var result SlackRepliesResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("cannot parse response: %w", err)
	}

	if !result.OK {
		return nil, fmt.Errorf("Slack API error: %s", result.Error)
	}

	return &result, nil
}

func postToSlack(token, channelID, threadTS, text string) error {
	payload := map[string]string{
		"channel":   channelID,
		"thread_ts": threadTS,
		"text":      text,
	}
	data, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", "https://slack.com/api/chat.postMessage", bytes.NewReader(data))
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

	if tests == "all" || tests == "failed" {
		return tests
	}

	parts := strings.Split(tests, ",")
	var cleaned []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			cleaned = append(cleaned, p)
		}
	}

	if len(cleaned) == 0 {
		return ""
	}

	return strings.Join(cleaned, ",")
}

func executeRerun(tests, slackToken, channelID, threadTS, user string) error {
	logInfo("========================================")
	logInfo("STARTING RERUN")
	logInfo("  Tests: %s", tests)
	logInfo("  Requested by: %s", user)
	logInfo("  Thread: %s", threadTS)
	logInfo("========================================")

	msg := fmt.Sprintf(":arrows_counterclockwise: Starting rerun of: `%s`\nRequested by: <@%s>", tests, user)
	if err := postToSlack(slackToken, channelID, threadTS, msg); err != nil {
		logWarn("Failed to post start message to Slack: %v", err)
	}

	if _, err := os.Stat(RunTestsScript); os.IsNotExist(err) {
		return fmt.Errorf("run_tests.sh not found at %s", RunTestsScript)
	}

	cmd := exec.Command(RunTestsScript, "-test", tests)
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
		logError("Rerun FAILED after %v (exit code %d)", duration, exitCode)

		msg := fmt.Sprintf(":x: Rerun failed for: `%s`\nExit code: %d\nDuration: %v", tests, exitCode, duration.Round(time.Second))
		postToSlack(slackToken, channelID, threadTS, msg)
		return fmt.Errorf("tests failed with exit code %d", exitCode)
	}

	logInfo("Rerun COMPLETED successfully in %v", duration.Round(time.Second))
	return nil
}

func main() {
	logInfo("========================================")
	logInfo("Rerun Poller Starting")
	logInfo("========================================")

	if !acquirePollerLock() {
		os.Exit(0)
	}
	defer releasePollerLock()

	if running, reason := isTestsRunning(); running {
		logInfo("Tests already running (%s) - skipping this poll cycle", reason)
		os.Exit(0)
	}

	slackToken := os.Getenv("SLACK_TOKEN")
	if slackToken == "" {
		logError("SLACK_TOKEN environment variable not set")
		os.Exit(1)
	}
	logInfo("Slack token loaded (length: %d)", len(slackToken))

	baseDir := os.Getenv("DISTROS_BASE_DIR")
	if baseDir == "" {
		baseDir = "/root/distros-test-framework"
	}
	stateFile := filepath.Join(baseDir, "report", ".rerun-state.json")
	logInfo("State file: %s", stateFile)

	state, err := loadState(stateFile)
	if err != nil {
		if os.IsNotExist(err) {
			logInfo("No state file - nothing to poll")
		} else {
			logError("Failed to load state: %v", err)
		}
		os.Exit(0)
	}
	logInfo("Loaded state: thread=%s, channel=%s, last_ts=%s", state.ThreadTS, state.ChannelID, state.LastProcessedTS)
	logInfo("Failed tests from last run: %v", state.FailedTests)

	logInfo("Fetching replies from Slack...")
	replies, err := fetchSlackReplies(slackToken, state.ChannelID, state.ThreadTS)
	if err != nil {
		logError("Failed to fetch Slack replies: %v", err)
		os.Exit(1)
	}
	logInfo("Fetched %d messages from thread", len(replies.Messages))

	// CHANGED: Scan ALL new messages looking for a valid rerun request
	// Only process if we find "rerun: <tests>" - ignore everything else
	var rerunMsg *SlackMessage
	var rerunTests string

	for i := len(replies.Messages) - 1; i >= 0; i-- {
		msg := replies.Messages[i]
		
		// Skip the original thread message
		if msg.TS == state.ThreadTS {
			continue
		}
		
		// Skip already processed messages
		if msg.TS <= state.LastProcessedTS {
			continue
		}

		// Check if this is a valid rerun request
		tests := parseRerunRequest(msg.Text)
		if tests != "" {
			rerunMsg = &msg
			rerunTests = tests
			logInfo("Found rerun request: ts=%s, user=%s, tests='%s'", msg.TS, msg.User, tests)
			break
		} else {
			logInfo("Skipping non-rerun message: ts=%s, text='%s'", msg.TS, msg.Text)
		}
	}

	if rerunMsg == nil {
		logInfo("No rerun request found in new messages - nothing to do")
		os.Exit(0)
	}

	// SAFETY: Update timestamp BEFORE executing to prevent duplicate triggers
	oldTS := state.LastProcessedTS
	state.LastProcessedTS = rerunMsg.TS
	if err := saveState(stateFile, state); err != nil {
		logError("CRITICAL: Failed to save state: %v", err)
		logError("Aborting to prevent duplicate triggers")
		os.Exit(1)
	}
	logInfo("SAFETY: Updated last_processed_ts from %s to %s BEFORE processing", oldTS, rerunMsg.TS)

	// Resolve "failed" to actual test names
	if rerunTests == "failed" {
		if len(state.FailedTests) == 0 {
			logWarn("User requested 'rerun: failed' but no failed tests recorded")
			postToSlack(slackToken, state.ChannelID, state.ThreadTS,
				":warning: No failed tests recorded from the last run. Use `rerun: all` or specify test names.")
			os.Exit(0)
		}
		rerunTests = strings.Join(state.FailedTests, ",")
		logInfo("Resolved 'failed' to: %s", rerunTests)
	}

	// Double-check tests aren't running now
	if running, reason := isTestsRunning(); running {
		logWarn("Tests started running between checks (%s) - aborting", reason)
		os.Exit(0)
	}

	if err := executeRerun(rerunTests, slackToken, state.ChannelID, state.ThreadTS, rerunMsg.User); err != nil {
		logError("Rerun failed: %v", err)
		os.Exit(1)
	}

	logInfo("========================================")
	logInfo("Rerun Poller Finished Successfully")
	logInfo("========================================")
}
