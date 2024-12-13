package testcase

import (
	"fmt"
	"strings"
	"time"

	"github.com/rancher/distros-test-framework/pkg/customflag"
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
		sonobuoyVersion := customflag.ServiceFlag.External.SonobuoyVersion
		err = shared.SonobuoyMixedOS("delete", sonobuoyVersion)
		if err != nil {
			GinkgoT().Errorf("error: %v", err)
			return
		}
	}
}

func ConformanceTest(cluster *shared.Cluster) {
	verifyClusterNodes(cluster)
	installConformanceBinary()
	// launchSonobuoyTests("certified-conformance")
	launchSonobuoyTests("quick")
	testResultTar := checkStatusGetResults()
	parseResults(testResultTar)
	cleanupTests()
}

func launchSonobuoyTests(testMode string) {
	//not doing anything different yet if the status is running from the previous attempts
	cmds := "kubectl get namespace sonobuoy --kubeconfig=" + shared.KubeConfigFile
	res, _ := shared.RunCommandHost(cmds)
	// Expect(err).NotTo(HaveOccurred())
	Expect(res).Should(ContainSubstring("NotFound"))
	// if kubectl get namespace sonobuoy returns sonobuoy Active then we can run the tests
	// else return out of this function and proceed to checkStatusGetResults()
	if strings.Contains(res, "Active") {
		fmt.Println("sonobuoy namespace already exists")
		return
	}
	if res == "Error from server (NotFound): namespaces \"sonobuoy\" not found" {
		cmd := "sonobuoy run --kubeconfig=" + shared.KubeConfigFile +
			" --mode=" + testMode + " --kubernetes-version=" + shared.ExtractKubeImageVersion() +
			" --aggregator-node-selector kubernetes.io/os:linux"
		res, err := shared.RunCommandHost(cmd)
		Expect(err).NotTo(HaveOccurred())
		// Expect(res).Should(ContainSubstring("Running"))
		fmt.Println(res)
	}
}

func verifyClusterNodes(cluster *shared.Cluster) bool {
	if cluster.NumAgents < 1 && cluster.NumServers < 1 {
		fmt.Println("cluster does not meet the minimum requirements to run conformance tests")
		return false
	} else {
		return true
	}
}

func rerunFailedTests(cluster *shared.Cluster, testResultTar string) bool {
	cmd := "sonobuoy run --rerun-failed=" + testResultTar + " --kubeconfig=" + shared.KubeConfigFile
	res, err := shared.RunCommandHost(cmd)
	Expect(err).NotTo(HaveOccurred(), "failed cmd: "+cmd)
	Expect(res).Should(ContainSubstring("passed"))
	passedIndividually := false
	return passedIndividually
}

func installConformanceBinary() {
	sonobuoyVersion := customflag.ServiceFlag.External.SonobuoyVersion
	err := shared.SonobuoyMixedOS("install", sonobuoyVersion)
	Expect(err).NotTo(HaveOccurred())
}

func parseResults(testResultTar string) {
	// so we use this to get the failed tests - might not want to use the expect function here but build an array for it
	cmd := "sonobuoy results " + testResultTar + " | awk '/Failed tests:/ { flag = 1; next } flag { print } /^$/ { flag = 0 }'"
	res, err := shared.RunCommandHost(cmd)
	Expect(err).NotTo(HaveOccurred(), "failed cmd: "+cmd)
	Expect(res).Should(BeEmpty())
	fmt.Println("the command output emitted from sonobuoy results tarfile awk checking failed: ", res)
	failedTests := strings.Split(res, "\n")
	fmt.Println("failed tests: ", failedTests)
	cmd = "sonobuoy results  " + testResultTar
	res, err = shared.RunCommandHost(cmd)
	Expect(err).NotTo(HaveOccurred(), "failed cmd: "+cmd)
	Expect(res).Should(ContainSubstring("shmooot"))
	cleanupTests()

}

func cleanupTests() {
	cmd := "sonobuoy delete --all --wait --kubeconfig=" + shared.KubeConfigFile
	res, err := shared.RunCommandHost(cmd)
	Expect(err).NotTo(HaveOccurred(), "failed cmd: "+cmd)
	Expect(res).Should(ContainSubstring("deleted"))
}

func exportResultsToS3() {
	// export results to s3
	// if destroy is false keep results in s3 bucket
	// send results to s3 bucket with deletion rules
}

func checkStatusGetResults() string {
	// sonobuoy's output is becoming unreliable for status checks observe remaining count incorrect at 404
	// 	sono status
	//          PLUGIN     STATUS   RESULT   COUNT                                PROGRESS
	//             e2e   complete   passed       1   Passed:  0, Failed:  0, Remaining:404
	//    systemd-logs   complete   passed       2
	// Sonobuoy has completed. Use `sonobuoy retrieve` to get results

	// checkStatusGetResults() //needs to return a status running or complete
	// if status is complete then retrieve results
	cmd := "sonobuoy status --kubeconfig=" + shared.KubeConfigFile
	res, err := shared.RunCommandHost(cmd)
	// Expect(err).NotTo(HaveOccurred())
	fmt.Println(err)
	fmt.Println(res)
	status := false
	for !status {
		if Expect(res).Should(ContainSubstring("Sonobuoy has completed")) {
			cmd = "sonobuoy retrieve --kubeconfig=" + shared.KubeConfigFile
			res, err = shared.RunCommandHost(cmd)
			Expect(err).NotTo(HaveOccurred(), "failed cmd: "+cmd)
			// Expect(res).Should(ContainSubstring("e2e	complete"))
			status = true
			fmt.Println(res)
			fmt.Println(err)
			return res
		}
		time.Sleep(10 * time.Minute)
	}

	return "no file found"
}
