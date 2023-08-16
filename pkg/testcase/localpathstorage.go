package testcase

import (
	"fmt"

	"github.com/rancher/distros-test-framework/pkg/assert"
	"github.com/rancher/distros-test-framework/pkg/customflag"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var lps = "local-path-storage"

func TestLocalPathProvisionerStorage(deployWorkload bool) {
	if deployWorkload {
		_, err := shared.ManageWorkload(
			"create",
			"local-path-provisioner.yaml",
			customflag.ServiceFlag.ClusterConfig.Arch.String(),
		)
		Expect(err).NotTo(HaveOccurred(),
			"local-path-provisioner manifest not deployed")
	}

	getPodVolumeTestRunning := "kubectl get pods -n local-path-storage" +
		" --field-selector=status.phase=Running --kubeconfig=" + shared.KubeConfigFile
	err := assert.ValidateOnHost(
		getPodVolumeTestRunning,
		Running,
	)
	if err != nil {
		GinkgoT().Errorf("%v", err)
	}

	_, err = shared.WriteDataPod(lps)
	if err != nil {
		GinkgoT().Errorf("error writing data to pod: %v", err)
		return
	}

	Eventually(func(g Gomega) {
		fmt.Println("Writing and reading data from pod")
		res, err := shared.ReadDataPod(lps)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(res).Should(ContainSubstring("testing local path"))
		g.Expect(err).NotTo(HaveOccurred())
	}, "420s", "2s").Should(Succeed())

	ips := shared.FetchNodeExternalIP()
	for _, ip := range ips {
		shared.RestartCluster("k3s", ip)
	}

	_, err = shared.ReadDataPod(lps)
	if err != nil {
		return
	}

	err = readData()
	if err != nil {
		return
	}
}

func readData() error {
	deletePod := "kubectl delete -n local-path-storage  pod -l app=volume-test --kubeconfig="
	err := assert.ValidateOnHost(deletePod+shared.KubeConfigFile, "deleted")
	if err != nil {
		return err
	}

	fmt.Println("Read data from newly create pod")
	_, err = shared.ReadDataPod(lps)
	if err != nil {
		return err
	}

	return nil
}
