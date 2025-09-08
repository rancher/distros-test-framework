package testcase

import (
	"time"

	"github.com/rancher/distros-test-framework/internal/pkg/assert"
	"github.com/rancher/distros-test-framework/internal/provisioning/driver"
	"github.com/rancher/distros-test-framework/internal/resources"

	. "github.com/onsi/gomega"
)

var namespace = "local-path-storage"

func TestLocalPathProvisionerStorage(cluster *driver.Cluster, applyWorkload, deleteWorkload bool) {
	createDir(cluster)

	var workloadErr error
	if applyWorkload {
		workloadErr = resources.ManageWorkload("apply", "local-path-provisioner.yaml")
		Expect(workloadErr).NotTo(HaveOccurred(), "local-path-provisioner manifest not deployed")
	}

	getPodVolumeTestRunning := "kubectl get pods -n local-path-storage" +
		" --field-selector=status.phase=Running --kubeconfig=" + resources.KubeConfigFile
	err := assert.ValidateOnHost(
		getPodVolumeTestRunning,
		statusRunning,
	)
	if err != nil {
		logDebugData(cluster)
	}
	Expect(err).NotTo(HaveOccurred(), err)

	_, err = resources.WriteDataPod(cluster, namespace)
	Expect(err).NotTo(HaveOccurred(), "error writing data to pod: %v", err)

	Eventually(func(g Gomega) {
		var res string
		resources.LogLevel("info", "Reading data from pod")

		res, err = resources.ReadDataPod(cluster, namespace)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(res).Should(ContainSubstring("testing local path"))
		g.Expect(err).NotTo(HaveOccurred())
	}, "300s", "5s").Should(Succeed())

	_, err = resources.ReadDataPod(cluster, namespace)
	if err != nil {
		return
	}

	err = readData(cluster)
	if err != nil {
		return
	}

	if deleteWorkload {
		workloadErr = resources.ManageWorkload("delete", "local-path-provisioner.yaml")
		Expect(workloadErr).NotTo(HaveOccurred(), "local-path-provisioner manifest not deleted")
	}
}

func readData(cluster *driver.Cluster) error {
	deletePod := "kubectl delete -n local-path-storage  pod -l app=volume-test --kubeconfig="
	err := assert.ValidateOnHost(deletePod+resources.KubeConfigFile, "deleted")
	if err != nil {
		return err
	}

	resources.LogLevel("info", "Reading data from newly created pod")
	delay := time.After(30 * time.Second)
	<-delay

	_, err = resources.ReadDataPod(cluster, namespace)
	if err != nil {
		return err
	}

	return nil
}

func createDir(cluster *driver.Cluster) {
	resources.LogLevel("debug", "node OS: %s ", cluster.NodeOS)
	if cluster.NodeOS == "slemicro" {
		for _, ip := range append(cluster.ServerIPs, cluster.AgentIPs...) {
			resources.CreateDir("/opt/data", "+w", ip)
		}
	}
}

// Logs the following debug data:
// 1. pod log and describe pod output for 'helper-pod-create-pvc' pod.
// 2. pod log and describe pod output for all pods in local-path-storage namespace
// 3. kubectl get pv,pvc,storageclass output
// 4. sestatus output
// 5. grep audit logs for denied calls and log the same.
func logDebugData(cluster *driver.Cluster) {
	// Pod log and describe pod output for 'helper-pod-create-pvc' pod
	resources.FindPodAndLog(cluster, "helper-pod-create-pvc", "kube-system")

	// Pod Log and describe pod output with namespace: local-path-storage
	resources.LogAllPodsForNamespace(cluster, namespace)

	// Log the kubectl get pv,pvc,storageclass
	output, getErr := resources.KubectlCommand(cluster, "node", "get", "pv,pvc,storageclass", "-A")
	if getErr != nil {
		resources.LogLevel("error", "error getting pv,pvc and storageclass info")
	}
	if output != "" {
		resources.LogLevel("debug", "pv,pvc,storageclass info:\n %s", output)
	}

	// Log sestatus output
	cmd := "sestatus"
	seStatusOut, statusLogErr := resources.RunCommandOnNode(cmd, cluster.ServerIPs[0])
	if statusLogErr != nil {
		resources.LogLevel("error", "error getting sestatus output")
	}
	if seStatusOut != "" {
		resources.LogLevel("debug", "sestatus:\n %s", seStatusOut)
	}

	// Grep and Log the audit logs for denied messages
	resources.LogGrepOutput("/var/log/audit/audit.log", "denied", cluster.ServerIPs[0])
}
