package testcase

import (
	"fmt"
	"time"

	"github.com/rancher/distros-test-framework/factory"
	"github.com/rancher/distros-test-framework/pkg/assert"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/gomega"
)

var lps = "local-path-storage"

func TestLocalPathProvisionerStorage(cluster *factory.Cluster, applyWorkload, deleteWorkload bool) {
	var workloadErr error
	if applyWorkload {
		workloadErr = shared.ManageWorkload("apply", "local-path-provisioner.yaml")
		Expect(workloadErr).NotTo(HaveOccurred(), "local-path-provisioner manifest not deployed")
	}

	getPodVolumeTestRunning := "kubectl get pods -n local-path-storage" +
		" --field-selector=status.phase=Running --kubeconfig=" + factory.KubeConfigFile
	err := assert.ValidateOnHost(
		getPodVolumeTestRunning,
		statusRunning,
	)
	Expect(err).NotTo(HaveOccurred(), err)

	_, err = shared.WriteDataPod(cluster, lps)
	Expect(err).NotTo(HaveOccurred(), "error writing data to pod: %v", err)

	Eventually(func(g Gomega) {
		var res string
		fmt.Println("Writing and reading data from pod")

		res, err = shared.ReadDataPod(cluster, lps)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(res).Should(ContainSubstring("testing local path"))
		g.Expect(err).NotTo(HaveOccurred())
	}, "300s", "5s").Should(Succeed())

	ips := shared.FetchNodeExternalIPs()
	for _, ip := range ips {
		shared.RestartCluster("k3s", ip)
	}

	_, err = shared.ReadDataPod(cluster, lps)
	if err != nil {
		return
	}

	err = readData(cluster)
	if err != nil {
		return
	}

	if deleteWorkload {
		workloadErr = shared.ManageWorkload("delete", "local-path-provisioner.yaml")
		Expect(workloadErr).NotTo(HaveOccurred(), "local-path-provisioner manifest not deleted")
	}
}

func readData(cluster *factory.Cluster) error {
	deletePod := "kubectl delete -n local-path-storage  pod -l app=volume-test --kubeconfig="
	err := assert.ValidateOnHost(deletePod+factory.KubeConfigFile, "deleted")
	if err != nil {
		return err
	}

	fmt.Println("Reading data from newly created pod")
	delay := time.After(30 * time.Second)
	<-delay

	_, err = shared.ReadDataPod(cluster, lps)
	if err != nil {
		return err
	}

	return nil
}
