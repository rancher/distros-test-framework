package resources

import (
	"fmt"
	"strings"
	"time"

	"github.com/rancher/distros-test-framework/internal/logging"
)

// CheckProcessCompletion monitors any process until it completes or times out.
// It accepts a process pattern like `.*{ps}|{ps}.*` or the name of the process to check.
// It uses the RunCommandOnNodeWithRetry() to repeatedly check if the process is still running.
func CheckProcessCompletion(nodeIP, processPattern string, attempts int, delay time.Duration) error {
	retryCfg := &RetryCfg{
		Attempts:                   attempts,
		Delay:                      delay,
		DelayMultiplier:            1.0,
		RetryableExitCodes:         []int{0, 1},
		RetryableErrorSubString:    []string{"connection", "timeout", "temporary"},
		NonRetryableErrorSubString: []string{},
	}
	// First check if process is already finished or not running.
	checkCmd := fmt.Sprintf("pgrep -f '%s' 2>/dev/null || echo 'not_found'", processPattern)
	result, err := RunCommandOnNode(checkCmd, nodeIP)
	if err != nil {
		LogLevel("debug", "Initial process check failed: %v", err)
	}
	result = strings.TrimSpace(result)
	if result == "not_found" || result == "" {
		LogLevel("info", "Process matching '%s' is not currently running on node %s", processPattern, nodeIP)

		return nil
	}

	LogLevel("info", "Process '%s' is running on node %s (PIDs: %s), waiting for completion",
		processPattern, nodeIP, result)
	initialPIDs := strings.Fields(result)
	checkProcesscmd := fmt.Sprintf(`
		stillRunning=false
		for pid in %s; do
			if kill -0 "$pid" 2>/dev/null; then
				echo "Process PID $pid still running"
				stillRunning=true
				break
			fi
		done
		if [ "$stillRunning" = "true" ]; then
			exit 1 
		else
			echo "All processes have completed"
			exit 0
		fi
	`, strings.Join(initialPIDs, " "))

	res, err := RunCommandOnNodeWithRetry(checkProcesscmd, nodeIP, retryCfg)
	if err != nil {
		return fmt.Errorf("timeout waiting for process '%s' to complete: %w", processPattern, err)
	}

	res = strings.TrimSpace(res)
	if !strings.Contains(res, "All processes have completed") {
		return fmt.Errorf("unexpected result while checking process completion: %s", res)
	}
	LogLevel("info", "All processes matching '%s' have completed on node %s", processPattern, nodeIP)

	return nil
}

 