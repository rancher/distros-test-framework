package testcase

import (
	"fmt"
	"time"

	"github.com/rancher/distros-test-framework/pkg/assert"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var lps = "local-path-storage"

func TestLocalPathProvisionerStorage(delete bool) {
	_, err := shared.ManageWorkload("apply", arch, "local-path-provisioner.yaml")
	Expect(err).NotTo(HaveOccurred(), "local-path-provisioner manifest not deployed")

	getPodVolumeTestRunning := "kubectl get pods -n local-path-storage" +
		" --field-selector=status.phase=Running --kubeconfig=" + shared.KubeConfigFile
	err = assert.ValidateOnHost(
		getPodVolumeTestRunning,
		statusRunning,
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
	}, "300s", "10s").Should(Succeed())

	ips := shared.FetchNodeExternalIP()
	for _, ip := range ips {
		_, err = shared.RestartCluster("k3s", ip)
		if err != nil {
			return
		}
	}
	time.Sleep(30 * time.Second)

	_, err = shared.ReadDataPod(lps)
	if err != nil {
		return
	}

	err = readData()
	if err != nil {
		return
	}

	if delete {
		_, err := shared.ManageWorkload("delete", arch, "local-path-provisioner.yaml")
		Expect(err).NotTo(HaveOccurred(), "local-path-provisioner manifest not deleted")
	}

}

func readData() error {
	deletePod := "kubectl delete -n local-path-storage  pod -l app=volume-test --kubeconfig="
	err := assert.ValidateOnHost(deletePod+shared.KubeConfigFile, "deleted")
	if err != nil {
		return err
	}
	time.Sleep(160 * time.Second)

	fmt.Println("Read data from newly create pod")
	_, err = shared.ReadDataPod(lps)
	if err != nil {
		return err
	}

	return nil
}
