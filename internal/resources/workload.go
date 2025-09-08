package resources

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/avast/retry-go"
)

var KubeConfigFile string

// ManageWorkload applies or deletes a workload based on the action: apply or delete.
func ManageWorkload(action string, workloads ...string) error {
	if action != "apply" && action != "delete" {
		return ReturnLogError("invalid action: %s. Must be 'apply' or 'delete'", action)
	}

	arch := os.Getenv("ARCH")
	if arch == "" {
		arch = "amd64"
	}

	resourceDir := BasePath() + "/workloads/" + arch
	files, readErr := os.ReadDir(resourceDir)
	if readErr != nil {
		return ReturnLogError("Unable to read resource manifest file for: %s\n with error:%w", resourceDir, readErr)
	}

	for _, workload := range workloads {
		if !fileExists(files, workload) {
			return ReturnLogError("workload %s not found", workload)
		}

		workloadErr := handleWorkload(action, resourceDir, workload)
		if workloadErr != nil {
			return workloadErr
		}
	}

	return nil
}

// ApplyWorkloadURL applies a workload from a URL.
func ApplyWorkloadURL(url string) error {
	applyWorkloadErr := applyWorkload("apply", url)
	if applyWorkloadErr != nil {
		return ReturnLogError("failed to apply workload: %s\n", applyWorkloadErr)
	}

	return nil
}

func handleWorkload(action, resourceDir, workload string) error {
	filename := filepath.Join(resourceDir, workload)

	switch action {
	case "apply":
		return applyWorkload(workload, filename)
	case "delete":
		return deleteWorkload(workload, filename)
	default:
		return ReturnLogError("invalid action: %s. Must be 'apply' or 'delete'", action)
	}
}

func applyWorkload(workload, filename string) error {
	LogLevel("info", "Applying %s", workload)
	cmd := "kubectl apply -f " + filename + " --kubeconfig=" + KubeConfigFile
	out, err := RunCommandHost(cmd)
	if err != nil || out == "" {
		if strings.Contains(out, "Invalid value") {
			return fmt.Errorf("failed to apply workload %s: %s", workload, out)
		}
		return ReturnLogError("failed to run kubectl apply: %w", err)
	}
	LogLevel("info", "Workload applied: %v", filename)
	LogLevel("debug", "Workload apply response: \n%v", out)

	out, err = RunCommandHost("kubectl get all -A " + " --kubeconfig=" + KubeConfigFile)
	if err != nil {
		return ReturnLogError("failed to run kubectl get all: %w\n", err)
	}

	if ok := !strings.Contains(out, "Creating") && strings.Contains(out, workload); ok {
		return ReturnLogError("failed to apply workload %s", workload)
	}

	return nil
}

// deleteWorkload deletes a workload and asserts that the workload is deleted.
func deleteWorkload(workload, filename string) error {
	LogLevel("info", "Removing %s", workload)

	cmd := "kubectl delete -f " + filename + " --kubeconfig=" + KubeConfigFile
	_, err := RunCommandHost(cmd)
	if err != nil {
		return err
	}

	// Wait for the workload to be deleted.
	err = retry.Do(
		func() error {
			res, err := RunCommandHost("kubectl get all -A " + " --kubeconfig=" + KubeConfigFile)
			if err != nil {
				return ReturnLogError("failed to run kubectl get all: %w\n", err)
			}

			if strings.Contains(res, workload) {
				return ReturnLogError("workload still exists: %s", workload)
			}

			return nil
		},
		retry.Attempts(10),
		retry.Delay(30*time.Second),
		retry.DelayType(retry.FixedDelay),
		retry.OnRetry(func(n uint, _ error) {
			LogLevel("info", "Waiting for workload to be deleted. Attempt: %d", n+1)
		}),
	)
	if err != nil {
		return ReturnLogError("workload delete timed out")
	}

	LogLevel("info", "Workload deleted: %v", filename)

	return nil
}

// ManageSonobuoy manages sonobuoy installation or deletion.
func ManageSonobuoy(action string) error {
	if action != "install" && action != "delete" {
		return ReturnLogError("invalid action: %s. Must be 'install' or 'delete'", action)
	}

	scriptsDir := BasePath() + "/scripts/install_sonobuoy.sh"

	if err := os.Chmod(scriptsDir, 0o755); err != nil {
		return ReturnLogError("failed to change script permissions: %w", err)
	}

	cmd := exec.Command(scriptsDir, action)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return ReturnLogError("failed to execute %s action sonobuoy: %w\nOutput: %s", action, err, output)
	}

	LogLevel("info", "Sonobuoy %s completed successfully\nOutput: %s", action, output)

	return nil
}
