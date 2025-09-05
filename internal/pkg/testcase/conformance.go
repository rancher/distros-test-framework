package testcase

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/avast/retry-go"

	"github.com/rancher/distros-test-framework/internal/resources"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// TestSonobuoyMixedOS runs sonobuoy tests for mixed os cluster (linux + windows) node.
func TestSonobuoyMixedOS(deleteWorkload bool, version string) {
	err := resources.InstallSonobuoy("install", version)
	Expect(err).NotTo(HaveOccurred())

	cmd := "sonobuoy run --kubeconfig=" + resources.KubeConfigFile +
		" --plugin my-sonobuoy-plugins/mixed-workload-e2e/mixed-workload-e2e.yaml" +
		" --aggregator-node-selector kubernetes.io/os:linux --wait"
	res, err := resources.RunCommandHost(cmd)
	Expect(err).NotTo(HaveOccurred(), "failed output: "+res)

	cmd = "sonobuoy retrieve --kubeconfig=" + resources.KubeConfigFile
	testResultTar, err := resources.RunCommandHost(cmd)
	Expect(err).NotTo(HaveOccurred(), "failed cmd: "+cmd)

	cmd = "sonobuoy results  " + testResultTar
	res, err = resources.RunCommandHost(cmd)
	Expect(err).NotTo(HaveOccurred(), "failed cmd: "+cmd)
	Expect(res).Should(ContainSubstring("Plugin: mixed-workload-e2e\nStatus: passed\n"))

	if deleteWorkload {
		cmd = "sonobuoy delete --all --wait --kubeconfig=" + resources.KubeConfigFile
		_, err = resources.RunCommandHost(cmd)
		Expect(err).NotTo(HaveOccurred(), "failed cmd: "+cmd)
		err = resources.InstallSonobuoy("delete", version)
		if err != nil {
			GinkgoT().Errorf("error: %v", err)
			return
		}
	}
}

func TestConformance(version string) {
	err := resources.InstallSonobuoy("install", version)
	Expect(err).NotTo(HaveOccurred())

	launchSonobuoyTests()

	statusErr := checkStatus()
	Expect(statusErr).NotTo(HaveOccurred())

	testResultTar, err := retrieveResultsTar()
	Expect(err).NotTo(HaveOccurred())
	resources.LogLevel("info", "%s", "testResultTar: "+testResultTar)

	results := getResults(testResultTar)
	resources.LogLevel("info", "sonobuoy results: %s", results)

	resultsErr := validateResults(results)
	Expect(resultsErr).NotTo(HaveOccurred())

	cleanupTests()
}

func launchSonobuoyTests() {
	resources.LogLevel("info", "checking namespace existence")

	cmds := "kubectl get namespace sonobuoy --kubeconfig=" + resources.KubeConfigFile
	res, _ := resources.RunCommandHost(cmds)
	if strings.Contains(res, "Active") {
		resources.LogLevel("info", "%s", "sonobuoy namespace is active, waiting for it to complete")
		return
	}

	if strings.Contains(res, "Error from server (NotFound): namespaces \"sonobuoy\" not found") {
		cmd := "sonobuoy run --kubeconfig=" + resources.KubeConfigFile +
			" --mode=certified-conformance --kubernetes-version=" + resources.ExtractKubeImageVersion()
		_, err := resources.RunCommandHost(cmd)
		Expect(err).NotTo(HaveOccurred())
	}
}

func checkStatus() error {
	resources.LogLevel("info", "checking status of running tests")

	return retry.Do(
		func() error {
			res, err := resources.RunCommandHost("sonobuoy status --kubeconfig=" + resources.KubeConfigFile)
			if err != nil {
				resources.LogLevel("error", "Error checking sonobuoy status: %v", err)
				return fmt.Errorf("sonobuoy status failed: %v", err)
			}

			resources.LogLevel("info", "Sonobuoy Status at %v:\n%s",
				time.Now().Format(time.Kitchen), res)

			if !strings.Contains(res, "Sonobuoy has completed") {
				return fmt.Errorf("sonobuoy has not completed on time, sonobuoy status:\n%s", res)
			}

			return nil
		},
		retry.Attempts(26),
		retry.Delay(10*time.Minute),
		retry.DelayType(retry.FixedDelay),
		retry.LastErrorOnly(true),
		retry.OnRetry(func(n uint, _ error) {
			resources.LogLevel("debug", "Attempt %d: Sonobuoy status check not finished yet, retrying...", n+1)
		}),
	)
}

func retrieveResultsTar() (string, error) {
	resources.LogLevel("info", "retrieving sonobuoy results tar")

	cmd := "sonobuoy retrieve --kubeconfig=" + resources.KubeConfigFile
	res, err := resources.RunCommandHost(cmd)
	if err != nil {
		return "", fmt.Errorf("failed to retrieve sonobuoy results tar: %w\ncmd: %s", err, cmd)
	}

	tarPath := strings.TrimSpace(res)
	absPath, err := filepath.Abs(tarPath)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path for tar: %w", err)
	}

	if _, statErr := os.Stat(absPath); statErr != nil {
		return "", fmt.Errorf("retrieved tar file does not exist: %w", statErr)
	}

	return absPath, nil
}

func getResults(testResultTar string) string {
	cmd := "sonobuoy results " + testResultTar
	res, err := resources.RunCommandHost(cmd)
	Expect(err).NotTo(HaveOccurred(), "failed cmd: %s\nwith output: %s\nerror: %v", cmd, res, err)

	return res
}

// validateResults validates the results of the sonobuoy tests.
// if all passed dont rerun the tests.
// If there are failures, check if the failures are expected with the cilium CNI plugin,if so, skip rerun.
// if not, rerun the tests and check the results again.
func validateResults(results string) error {
	if pluginsPass := strings.Contains(results, "Plugin: systemd-logs\nStatus: passed") &&
		strings.Contains(results, "Plugin: e2e\nStatus: passed"); pluginsPass {
		resources.LogLevel("info", "all plugins passed")

		return nil
	}

	failures, failErr := extractFailedTests(results)
	if failErr != nil {
		return fmt.Errorf("failed to extract failed tests: %w", failErr)
	}

	shouldRerun, err := skipRerun(results, failures)
	if err != nil {
		return fmt.Errorf("failed to run results: %w", err)
	}

	if !shouldRerun {
		return nil
	}

	return execRerun()
}

func execRerun() error {
	newTar, err := retrieveResultsTar()
	if err != nil {
		return fmt.Errorf("failed to retrieve results tarball: %w", err)
	}
	if newTar == "" {
		return errors.New("failed to retrieve results tarball")
	}
	resources.LogLevel("info", "new results tarball: %s", newTar)

	_ = getResults(newTar)

	cleanupTests()

	rerunErr := rerunFailedTests(newTar)
	if rerunErr != nil {
		return fmt.Errorf("rerun failed: %w", rerunErr)
	}

	statusErr := checkStatus()
	Expect(statusErr).NotTo(HaveOccurred())

	resources.LogLevel("info", "getting new results after rerun")
	newResults := getResults(newTar)

	newFailures, failErr := extractFailedTests(newResults)
	if failErr != nil {
		return fmt.Errorf("failed to extract failed tests after rerun: %w", failErr)
	}
	if len(newFailures) > 0 {
		return fmt.Errorf("tests still failing after rerun: %v", newFailures)
	}

	Expect(newResults).ShouldNot(ContainSubstring("Status: failed"), "failed tests: %s", newResults)
	Expect(newResults).ShouldNot(ContainSubstring("Failed tests:"), "failed tests: %s", newResults)

	pluginsPass := strings.Contains(newResults, "Plugin: systemd-logs\nStatus: passed") &&
		strings.Contains(newResults, "Plugin: e2e\nStatus: passed")
	Expect(pluginsPass).Should(BeTrue())

	return nil
}

func skipRerun(results string, failures []string) (bool, error) {
	if len(failures) == 0 {
		if !strings.Contains(results, "Status: failed") {
			resources.LogLevel("info", "no explicit failures detected")

			return false, nil
		}
		resources.LogLevel("warn", "status failed but no specific test failures found, proceeding with rerun")

		return true, nil
	}

	resources.LogLevel("warn", "found %d test failures", len(failures))

	serverFlags := os.Getenv("server_flags")
	if strings.Contains(serverFlags, "cilium") && len(failures) > 0 {
		resources.LogLevel("info", "checking cilium for expected failures")

		nonNetworkFailures := false
		for _, failure := range failures {
			if !strings.Contains(failure, "[sig-network]") {
				nonNetworkFailures = true
				resources.LogLevel("warn", "Found non-network failure: %s", failure)
			}
		}

		if !nonNetworkFailures {
			resources.LogLevel("info", "Cilium CNI detected, "+
				"all failures are in sig-network namespace, skipping rerun")

			return false, nil
		}

		resources.LogLevel("warn", "found non-network failures with Cilium CNI, proceeding with rerun")
	}

	return true, nil
}

func extractFailedTests(results string) ([]string, error) {
	var failures []string

	fails := strings.Index(results, "Failed tests:")
	if fails == -1 {
		return nil, errors.New("no failed tests found")
	}

	failedTests := strings.Split(results[fails:], "\n")
	for i := 1; i < len(failedTests); i++ {
		line := strings.TrimSpace(failedTests[i])
		if line == "" {
			break
		}
		failures = append(failures, line)
	}

	return failures, nil
}

func rerunFailedTests(testResultTar string) error {
	cmd := "sonobuoy run --rerun-failed=" + testResultTar + "  --kubeconfig=" + resources.KubeConfigFile +
		" --kubernetes-version=" + resources.ExtractKubeImageVersion()

	resources.LogLevel("info ", "rerunning failed tests with cmd: %s", cmd)

	_, err := resources.RunCommandHost(cmd)
	Expect(err).NotTo(HaveOccurred(), "failed cmd: %s\nerror: %v", cmd, err)

	return nil
}

func cleanupTests() {
	resources.LogLevel("info", "cleaning up cluster conformance tests and deleting sonobuoy namespace")

	cmd := "sonobuoy delete --all --wait --kubeconfig=" + resources.KubeConfigFile
	res, err := resources.RunCommandHost(cmd)
	Expect(err).NotTo(HaveOccurred(), "failed cmd: "+cmd)
	Expect(res).Should(ContainSubstring("deleted"))
}
