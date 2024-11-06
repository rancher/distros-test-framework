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
	sonobuoyVersion := customflag.ServiceFlag.External.SonobuoyVersion
	err := shared.SonobuoyMixedOS("install", sonobuoyVersion)
	Expect(err).NotTo(HaveOccurred())

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
		err = shared.SonobuoyMixedOS("delete", sonobuoyVersion)
		if err != nil {
			GinkgoT().Errorf("error: %v", err)
			return
		}
	}
}

func ConformanceTest(cluster *shared.Cluster) {
	sonobuoyVersion := customflag.ServiceFlag.External.SonobuoyVersion
	err := shared.SonobuoyMixedOS("install", sonobuoyVersion)
	Expect(err).NotTo(HaveOccurred())

	cmd := "sonobuoy run --kubeconfig=" + shared.KubeConfigFile +
		" --mode=certified-conformance --kubernetes-version=" + shared.ExtractKubeImageVersion() +
		" --aggregator-node-selector kubernetes.io/os:linux --wait"
	res, err := shared.RunCommandHost(cmd)
	Expect(err).NotTo(HaveOccurred(), "failed output: "+res)

	cmd = "sonobuoy retrieve --kubeconfig=" + shared.KubeConfigFile
	testResultTar, err := shared.RunCommandHost(cmd)
	Expect(err).NotTo(HaveOccurred(), "failed cmd: "+cmd)

	cmd = "sonobuoy results " + testResultTar + " | awk '/Failed tests:/ { flag = 1; next } flag { print } /^$/ { flag = 0 }'"
	res, err = shared.RunCommandHost(cmd)
	Expect(err).NotTo(HaveOccurred(), "failed cmd: "+cmd)
	failedTests := strings.Split(res, "\n")

	cmd = "sonobuoy results  " + testResultTar
	res, err = shared.RunCommandHost(cmd)
	Expect(err).NotTo(HaveOccurred(), "failed cmd: "+cmd)

	// if len(failedTests) != []nil {
	// 	cmd = "sonobuoy delete --all --wait --kubeconfig=" + shared.KubeConfigFile
	// 	_, err = shared.RunCommandHost(cmd)
	// 	Expect(err).NotTo(HaveOccurred(), "failed cmd: "+cmd)
	// }
	if len(failedTests) != 0 {
		cmd = "sonobuoy delete --all --wait --kubeconfig=" + shared.KubeConfigFile
		res, err = shared.RunCommandHost(cmd)
	}
	cmd = "sonobuoy delete --all --wait --kubeconfig=" + shared.KubeConfigFile
	res, err = shared.RunCommandHost(cmd)
	fmt.Println("serverVersion: ", strings.Split(sonobuoyVersion, "\n"))
}

// todo retry logic array processing
// hydrophone run failed in parallel?
