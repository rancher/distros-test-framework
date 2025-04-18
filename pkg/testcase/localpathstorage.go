package testcase

import (
	"time"

	"github.com/rancher/distros-test-framework/pkg/assert"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/gomega"
)

var namespace = "local-path-storage"

func TestLocalPathProvisionerStorage(cluster *shared.Cluster, applyWorkload, deleteWorkload bool) {
	createDir(cluster)

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
	if err != nil {
		logDebugData(cluster)
	}
	Expect(err).NotTo(HaveOccurred(), err)

	_, err = shared.WriteDataPod(cluster, namespace)
	Expect(err).NotTo(HaveOccurred(), "error writing data to pod: %v", err)

	Eventually(func(g Gomega) {
		var res string
		shared.LogLevel("info", "Reading data from pod")

		res, err = shared.ReadDataPod(cluster, namespace)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(res).Should(ContainSubstring("testing local path"))
		g.Expect(err).NotTo(HaveOccurred())
	}, "300s", "5s").Should(Succeed())

	_, err = shared.ReadDataPod(cluster, namespace)
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

	_, err = shared.ReadDataPod(cluster, namespace)
	if err != nil {
		return err
	}

	return nil
}

func createDir(cluster *shared.Cluster) {
	shared.LogLevel("debug", "node OS: %s ", cluster.NodeOS)
	if cluster.NodeOS == "slemicro" {
		for _, ip := range append(cluster.ServerIPs, cluster.AgentIPs...) {
			shared.CreateDir("/opt/data", "+w", ip)
		}
	}
}

func logDebugData(cluster *shared.Cluster) {
	// Pod log and describe pod output for 'helper-pod-create-pvc' pod
	shared.FindPodAndLog("helper-pod-create-pvc", "kube-system")

	// Pod Log and describe pod output with namespace: local-path-storage
	shared.LogAllPodsForNamespace(namespace)

	// Log the kubectl get pv,pvc,storageclass
	output, getErr := shared.KubectlCommand(cluster, "node", "get", "pv,pvc,storageclass", "-A")
	if getErr != nil {
		shared.LogLevel("error", "error getting pv,pvc and storageclass info")
	}
	if output != "" {
		shared.LogLevel("debug", "pv,pvc,storageclass info:\n %s", output)
	}

	// Log sestatus output
	cmd := "sestatus"
	seStatusOut, statusLogErr := shared.RunCommandOnNode(cmd, cluster.ServerIPs[0])
	if statusLogErr != nil {
		shared.LogLevel("error", "error getting sestatus output")
	}
	if seStatusOut != "" {
		shared.LogLevel("debug", "sestatus:\n %s", seStatusOut)
	}

	// Grep and Log the audit logs for denied messages
	shared.LogGrepOutput("/var/log/audit/audit.log", "denied", cluster.ServerIPs[0])
}
