package testcase

import (
	"os"
	"time"

	"github.com/rancher/distros-test-framework/pkg/assert"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/gomega"
)

var lps = "local-path-storage"

func TestLocalPathProvisionerStorage(cluster *shared.Cluster, applyWorkload, deleteWorkload bool) {
	nodeOS := os.Getenv("node_os")
	if nodeOS == "slemicro" {
		var output string
		var mkdirErr error
		for _, ip := range cluster.ServerIPs {
			output, mkdirErr = shared.RunCommandOnNode("test -d '/opt/data' && echo 'directory exists: /opt/data' || sudo mkdir -p /opt/data; ls -lrt /opt", ip)
			if mkdirErr != nil {
				shared.LogLevel("warn", "error creating /opt/data dir on node ip: %s", ip)
			}
			if output != "" {
				shared.LogLevel("debug", "create and check /opt/data output: %s", output)
			}
		}

	}
	var workloadErr error
	if applyWorkload {
		workloadErr = shared.ManageWorkload("apply", "local-path-provisioner.yaml")
		Expect(workloadErr).NotTo(HaveOccurred(), "local-path-provisioner manifest not deployed")
	}

	getPodVolumeTestRunning := "kubectl get pods -n local-path-storage" +
		" --field-selector=status.phase=Running --kubeconfig=" + shared.KubeConfigFile
	err := assert.ValidateOnHost(
		getPodVolumeTestRunning,
		statusRunning,
	)
	Expect(err).NotTo(HaveOccurred(), err)

	_, err = shared.WriteDataPod(cluster, lps)
	Expect(err).NotTo(HaveOccurred(), "error writing data to pod: %v", err)

	Eventually(func(g Gomega) {
		var res string
		shared.LogLevel("info", "Reading data from pod")

		res, err = shared.ReadDataPod(cluster, lps)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(res).Should(ContainSubstring("testing local path"))
		g.Expect(err).NotTo(HaveOccurred())
	}, "300s", "5s").Should(Succeed())

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

func readData(cluster *shared.Cluster) error {
	deletePod := "kubectl delete -n local-path-storage  pod -l app=volume-test --kubeconfig="
	err := assert.ValidateOnHost(deletePod+shared.KubeConfigFile, "deleted")
	if err != nil {
		return err
	}

	shared.LogLevel("info", "Reading data from newly created pod")
	delay := time.After(30 * time.Second)
	<-delay

	_, err = shared.ReadDataPod(cluster, lps)
	if err != nil {
		return err
	}

	return nil
}
