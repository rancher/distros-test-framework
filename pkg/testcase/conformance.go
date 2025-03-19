package testcase

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// TestSonobuoyMixedOS runs sonobuoy tests for mixed os cluster (linux + windows) node.
func TestSonobuoyMixedOS(deleteWorkload bool) {
	installConformanceBinary()

	cmd := "sonobuoy run --kubeconfig=" + shared.KubeConfigFile +
		" --plugin my-sonobuoy-plugins/mixed-workload-e2e/mixed-workload-e2e.yaml" +
		" --aggregator-node-selector kubernetes.io/os:linux --wait"
	res, err := shared.RunCommandHost(cmd)
	Expect(err).NotTo(HaveOccurred(), "failed output: "+res)

	cmd = "sonobuoy retrieve --kubeconfig=" + shared.KubeConfigFile
	testResultTar, err := shared.RunCommandHost(cmd)
	Expect(err).NotTo(HaveOccurred(), "failed cmd: "+cmd)

	cmd = "sonobuoy results  " + testResultTar
	res, err = shared.RunCommandHost(cmd)
	Expect(err).NotTo(HaveOccurred(), "failed cmd: "+cmd)
	Expect(res).Should(ContainSubstring("Plugin: mixed-workload-e2e\nStatus: passed\n"))

	if deleteWorkload {
		cmd = "sonobuoy delete --all --wait --kubeconfig=" + shared.KubeConfigFile
		_, err = shared.RunCommandHost(cmd)
		Expect(err).NotTo(HaveOccurred(), "failed cmd: "+cmd)
		err = shared.InstallSonobuoy("delete")
		if err != nil {
			GinkgoT().Errorf("error: %v", err)
			return
		}
	}
}

func ConformanceTest() {
	installConformanceBinary()
	launchSonobuoyTests()

	checkStatus()
	testResultTar := getResults()
	shared.LogLevel("info", "%s", "testResultTar: "+testResultTar)

	if hasFailures(testResultTar) {
		rerunFailedTests(testResultTar)
		testResultTar = getResults()
	}

	shared.LogLevel("info", "%s", "testResultTar: "+testResultTar)

	parseResults(testResultTar)

	cleanupTests()
}

func installConformanceBinary() {
	shared.LogLevel("info", "installing sonobuoy binary")

	err := shared.InstallSonobuoy("install")
	Expect(err).NotTo(HaveOccurred())
}

func launchSonobuoyTests() {
	shared.LogLevel("info", "checking namespace existence")

	cmds := "kubectl get namespace sonobuoy --kubeconfig=" + shared.KubeConfigFile
	res, _ := shared.RunCommandHost(cmds)
	if strings.Contains(res, "Active") {
		shared.LogLevel("info", "%s", "sonobuoy namespace is active, waiting for it to complete")
		return
	}

	if strings.Contains(res, "Error from server (NotFound): namespaces \"sonobuoy\" not found") {
		cmd := "sonobuoy run --kubeconfig=" + shared.KubeConfigFile +
			" --mode=certified-conformance --kubernetes-version=" + shared.ExtractKubeImageVersion()
		_, err := shared.RunCommandHost(cmd)
		Expect(err).NotTo(HaveOccurred())
	}
}

func checkStatus() {
	shared.LogLevel("info", "checking status of running tests")

	resultChan := make(chan string, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 260*time.Minute)
	defer cancel()

	cmd := "sonobuoy status --kubeconfig=" + shared.KubeConfigFile

	go printStatusWorker(ctx, cmd, resultChan)

	go checkCompletionWorker(ctx, cmd, resultChan)

	finalResult := <-resultChan
	if strings.Contains(finalResult, "Sonobuoy has completed") {
		shared.LogLevel("info", "Sonobuoy Status: %s", finalResult)
	} else {
		shared.LogLevel("error", "check status failed: %s", finalResult)
	}
}

func getResults() string {
	shared.LogLevel("info", "getting sonobuoy results")

	cmd := "sonobuoy retrieve --kubeconfig=" + shared.KubeConfigFile
	res, err := shared.RunCommandHost(cmd)
	Expect(err).NotTo(HaveOccurred())

	return res
}

func hasFailures(testResultTar string) bool {
	shared.LogLevel("info", "checking for failed tests")

	cmd := exec.Command("sonobuoy", "results", testResultTar, "--mode=failed")
	failedTests, err := cmd.Output()
	if err != nil {
		shared.LogLevel("error", "failed to run sonobuoy results: %v", err)
		return false
	}

	fails := len(failedTests) != 0 && !strings.Contains(string(failedTests), "No failed tests found")
	if fails {
		shared.LogLevel("info", "failed tests: %s", string(failedTests))

		return true
	}

	return false
}

func rerunFailedTests(testResultTar string) {
	ciliumExpectedFailures := `
		[sig-network] Services should serve endpoints on same port and different protocols
	 	Services should be able to switch session affinity for service with type clusterIP
		Services should have session affinity work for service with type clusterIP`

	if strings.Contains(os.Getenv("cni"), "cilium") {
		shared.LogLevel("info", "Cilium has known issues with conformance tests, skipping re-run")
		shared.LogLevel("info", "ciliumExpectedFailures: %s", ciliumExpectedFailures)

		return
	}

	cmd := "sonobuoy run --rerun-failed=" + testResultTar + " --kubeconfig=" + shared.KubeConfigFile +
		" --kubernetes-version=" + shared.ExtractKubeImageVersion()
	res, err := shared.RunCommandHost(cmd)
	Expect(err).To(HaveOccurred(), "failed cmd: "+cmd)
	Expect(res).Should(ContainSubstring("no tests failed for plugin"))
}

func parseResults(testResultTar string) {
	shared.LogLevel("info", "parsing sonobuoy results")

	cmd := "sonobuoy results  " + testResultTar
	res, err := shared.RunCommandHost(cmd)
	Expect(err).NotTo(HaveOccurred(), "failed cmd: "+cmd)
	Expect(res).Should(ContainSubstring("Status: passed"))
	shared.LogLevel("info", "%s", "sonobuoy results: "+res)
}

func cleanupTests() {
	shared.LogLevel("info", "cleaning up cluster conformance tests and deleting sonobuoy namespace")

	cmd := "sonobuoy delete --all --wait --kubeconfig=" + shared.KubeConfigFile
	res, err := shared.RunCommandHost(cmd)
	Expect(err).NotTo(HaveOccurred(), "failed cmd: "+cmd)
	Expect(res).Should(ContainSubstring("deleted"))
}

func printStatusWorker(ctx context.Context, cmd string, resultChan chan string) {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	retries := 0
	const maxRetries = 3

	for {
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				shared.LogLevel("info", "Sonobuoy status timed out")
				resultChan <- "Timed out waiting for sonobuoy"
			}

			return
		case <-ticker.C:
			res, err := shared.RunCommandHost(cmd)
			if err != nil {
				retries++
				shared.LogLevel("error", "checking sonobuoy status, will (attempt %d/%d): %v",
					retries, maxRetries, err)

				if retries >= maxRetries {
					resultChan <- fmt.Sprintf("checking sonobuoy status: %v", err)
					return
				}

				continue
			}

			retries = 0
			shared.LogLevel("info", "Sonobuoy Status at %v:\n%s", time.Now().Format(time.Kitchen), res)
		}
	}
}

func checkCompletionWorker(ctx context.Context, cmd string, resultChan chan string) {
	retries := 0
	const maxRetries = 3

	for {
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				shared.LogLevel("info", "Sonobuoy status timed out")
				resultChan <- "Timed out waiting for sonobuoy"
			}

			return
		default:
			res, err := shared.RunCommandHost(cmd)
			if err != nil {
				retries++
				shared.LogLevel("error", "checking sonobuoy status, will (attempt %d/%d): %v",
					retries, maxRetries, err)

				if retries >= maxRetries {
					resultChan <- fmt.Sprintf("checking sonobuoy status: %v", err)
					return
				}

				continue
			}

			retries = 0
			if strings.Contains(res, "Sonobuoy has completed") {
				resultChan <- res
				return
			}
		}
	}
}
