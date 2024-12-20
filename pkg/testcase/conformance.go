package testcase

import (
	"fmt"
	"strings"

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
		err = shared.InstallSonobuoy("delete", sonobuoyVersion)
		if err != nil {
			GinkgoT().Errorf("error: %v", err)
			return
		}
	}
}

func ConformanceTest(cluster *shared.Cluster) {
	verifyClusterNodes(cluster)
	installConformanceBinary()
	launchSonobuoyTests("certified-conformance")
	// launchSonobuoyTests("quick")
	testResultTar := checkStatusGetResults(cluster)
	fmt.Println("testResultTar: ", testResultTar)
	// need to do cilium force failures to test
	rerunFailedTests(testResultTar)
	parseResults(testResultTar)
	cleanupTests()
}

func verifyClusterNodes(cluster *shared.Cluster) bool {
	if cluster.NumAgents < 1 && cluster.NumServers < 1 {
		fmt.Println("cluster does not meet the minimum requirements to run conformance tests")
		return false
	}
	return true
}

func installConformanceBinary() {
	sonobuoyVersion := customflag.ServiceFlag.External.SonobuoyVersion
	err := shared.InstallSonobuoy("install", sonobuoyVersion)
	Expect(err).NotTo(HaveOccurred())
}

func launchSonobuoyTests(testMode string) {
	// not doing anything different yet if the status is running from the previous attempts
	cmds := "kubectl get namespace sonobuoy --kubeconfig=" + shared.KubeConfigFile
	res, _ := shared.RunCommandHost(cmds)
	if strings.Contains(res, "Active") {
		fmt.Println("sonobuoy namespace already exists, it is either still running, failed, or completed")
		return
	}
	if strings.Contains(res, "Error from server (NotFound): namespaces \"sonobuoy\" not found") {
		cmd := "sonobuoy run --kubeconfig=" + shared.KubeConfigFile +
			" --mode=" + testMode + " --kubernetes-version=" + shared.ExtractKubeImageVersion()
		_, err := shared.RunCommandHost(cmd)
		Expect(err).NotTo(HaveOccurred())
	}
}

func checkStatusGetResults(cluster *shared.Cluster) string {
	// sonobuoy's output is becoming unreliable for status checks observe remaining count incorrect at 404
	// 	sono status
	//          PLUGIN     STATUS   RESULT   COUNT                                PROGRESS
	//             e2e   complete   passed       1   Passed:  0, Failed:  0, Remaining:404
	//    systemd-logs   complete   passed       2
	// Sonobuoy has completed. Use `sonobuoy retrieve` to get results
	cmd := "sonobuoy status --kubeconfig=" + shared.KubeConfigFile
	Eventually(func() string {
		res, err := shared.RunCommandHost(cmd)
		Expect(err).NotTo(HaveOccurred())
		return res
	}, "170m", "45s").Should(ContainSubstring("Sonobuoy has completed"))

	cmd = "sonobuoy retrieve --kubeconfig=" + shared.KubeConfigFile
	res, err := shared.RunCommandHost(cmd)
	Expect(err).NotTo(HaveOccurred())
	_, err = shared.RunCommandOnNode(cmd, cluster.ServerIPs[0])
	Expect(err).NotTo(HaveOccurred())
	return res
}

func rerunFailedTests(testResultTar string) {
	cmd := "sonobuoy run --rerun-failed=" + testResultTar + " --kubeconfig=" + shared.KubeConfigFile
	res, err := shared.RunCommandHost(cmd)
	Expect(err).To(HaveOccurred(), "failed cmd: "+cmd)
	Expect(res).Should(ContainSubstring("no tests failed for plugin"))
}

func parseResults(testResultTar string) {
	cmd := "sonobuoy results  " + testResultTar
	res, err := shared.RunCommandHost(cmd)
	Expect(err).NotTo(HaveOccurred(), "failed cmd: "+cmd)
	Expect(res).Should(ContainSubstring("Status: passed"))
	fmt.Println("sonobuoy results: ", res)
}

// func exportResultsToS3() {}
// export results to s3
// if destroy is false keep results in s3 bucket
// send results to s3 bucket with deletion rules}

func cleanupTests() {
	cmd := "sonobuoy delete --all --wait --kubeconfig=" + shared.KubeConfigFile
	res, err := shared.RunCommandHost(cmd)
	Expect(err).NotTo(HaveOccurred(), "failed cmd: "+cmd)
	Expect(res).Should(ContainSubstring("deleted"))
}
