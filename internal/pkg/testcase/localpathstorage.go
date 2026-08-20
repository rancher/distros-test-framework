package testcase

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rancher/distros-test-framework/internal/pkg/assert"
	"github.com/rancher/distros-test-framework/internal/provisioning/driver"
	"github.com/rancher/distros-test-framework/internal/resources"

	. "github.com/onsi/gomega"
)

var namespace = "local-path-storage"

type localPathVolume struct {
	name string
	path string
}

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
	Expect(err).NotTo(HaveOccurred())

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
	Expect(err).NotTo(HaveOccurred(), "failed to read local-path data before pod recreation")

	err = readData(cluster)
	Expect(err).NotTo(HaveOccurred(), "failed to read local-path data after pod recreation")

	if deleteWorkload {
		volume, volumeErr := captureLocalPathVolume(cluster)
		Expect(volumeErr).NotTo(HaveOccurred(), "failed to capture local-path volume before deletion")

		workloadErr = resources.ManageWorkload("delete", "local-path-provisioner.yaml")
		Expect(workloadErr).NotTo(HaveOccurred(), "local-path-provisioner manifest not deleted")

		cleanupErr := waitForLocalPathCleanup(cluster, volume, 2*time.Minute)
		if cleanupErr != nil {
			logLocalPathCleanupData(cluster, volume)
		}
		Expect(cleanupErr).NotTo(HaveOccurred(), "local-path volume cleanup failed")
	}
}

func captureLocalPathVolume(cluster *driver.Cluster) (localPathVolume, error) {
	volumeName, err := resources.KubectlCommand(
		cluster,
		"host",
		"get",
		"pvc",
		"local-path-pvc -n "+namespace+" -o jsonpath='{.spec.volumeName}'",
	)
	if err != nil {
		return localPathVolume{}, fmt.Errorf("failed to get local-path PV name: %w", err)
	}

	volumeName = strings.TrimSpace(volumeName)
	if volumeName == "" {
		return localPathVolume{}, errors.New("local-path PVC is not bound to a PV")
	}

	volumePath, err := resources.KubectlCommand(
		cluster,
		"host",
		"get",
		"pv",
		volumeName+" -o jsonpath='{.spec.local.path}{.spec.hostPath.path}'",
	)
	if err != nil {
		return localPathVolume{}, fmt.Errorf("failed to get backing path for PV %s: %w", volumeName, err)
	}

	volumePath = strings.TrimSpace(volumePath)
	if volumePath == "" {
		return localPathVolume{}, fmt.Errorf("PV %s has no local backing path", volumeName)
	}

	resources.LogLevel("info", "Captured local-path PV %s with backing path %s", volumeName, volumePath)

	return localPathVolume{name: volumeName, path: volumePath}, nil
}

func waitForLocalPathCleanup(cluster *driver.Cluster, volume localPathVolume, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastState string

	for time.Now().Before(deadline) {
		pv, err := resources.KubectlCommand(
			cluster,
			"host",
			"get",
			"pv",
			volume.name+" --ignore-not-found -o name",
		)
		if err != nil {
			lastState = fmt.Sprintf("failed to query PV: %v", err)
			time.Sleep(5 * time.Second)
			continue
		}

		if strings.TrimSpace(pv) != "" {
			lastState = "PV still exists"
			time.Sleep(5 * time.Second)
			continue
		}

		pathErr := localPathRemovedFromNodes(cluster, volume.path)
		if pathErr == nil {
			return nil
		}

		lastState = pathErr.Error()
		time.Sleep(5 * time.Second)
	}

	return fmt.Errorf(
		"PV %s or backing path %s was not deleted within %s: %s",
		volume.name,
		volume.path,
		timeout,
		lastState,
	)
}

func localPathRemovedFromNodes(cluster *driver.Cluster, path string) error {
	nodeIPs := clusterNodeIPs(cluster)
	if len(nodeIPs) == 0 {
		return errors.New("cluster has no node IPs for backing-path verification")
	}

	for _, ip := range nodeIPs {
		_, err := resources.RunCommandOnNode("sudo test ! -e "+shellQuote(path), ip)
		if err != nil {
			return fmt.Errorf("backing path still exists or could not be checked on %s: %w", ip, err)
		}
	}

	return nil
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func clusterNodeIPs(cluster *driver.Cluster) []string {
	serverIPs := append([]string{}, cluster.ServerIPs...)

	return append(serverIPs, cluster.AgentIPs...)
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

// Logs provisioner pods, storage state, SELinux status, and audit denials.
// Cleanup failures also include the PV description and VolumeFailedDelete events.
func logDebugData(cluster *driver.Cluster) {
	// Pod logs and descriptions for the provisioner and its helper pods
	resources.FindPodAndLog(cluster, "local-path-provisioner", "kube-system")
	resources.FindPodAndLog(cluster, "helper-pod-create-pvc", "kube-system")
	resources.FindPodAndLog(cluster, "helper-pod-delete-pvc", "kube-system")

	// Pod Log and describe pod output with namespace: local-path-storage.
	resources.LogAllPodsForNamespace(cluster, namespace)

	// Log the kubectl get pv,pvc,storageclass.
	output, getErr := resources.KubectlCommand(cluster, "node", "get", "pv,pvc,storageclass", "-A")
	if getErr != nil {
		resources.LogLevel("error", "error getting pv,pvc and storageclass info")
	}
	if output != "" {
		resources.LogLevel("debug", "pv,pvc,storageclass info:\n %s", output)
	}

	logSELinuxData(cluster)
}

func logLocalPathCleanupData(cluster *driver.Cluster, volume localPathVolume) {
	logDebugData(cluster)

	output, err := resources.KubectlCommand(cluster, "host", "describe", "pv", volume.name)
	if err != nil {
		resources.LogLevel("error", "error describing local-path PV %s: %v", volume.name, err)
	}
	if output != "" {
		resources.LogLevel("debug", "local-path PV %s:\n%s", volume.name, output)
	}

	output, err = resources.KubectlCommand(
		cluster,
		"host",
		"get",
		"events",
		"-A --field-selector reason=VolumeFailedDelete --sort-by=.lastTimestamp",
	)
	if err != nil {
		resources.LogLevel("error", "error getting VolumeFailedDelete events: %v", err)
	}
	if output != "" {
		resources.LogLevel("debug", "VolumeFailedDelete events:\n%s", output)
	}
}

func logSELinuxData(cluster *driver.Cluster) {
	for _, ip := range clusterNodeIPs(cluster) {
		seStatusOut, statusLogErr := resources.RunCommandOnNode("sestatus", ip)
		if statusLogErr != nil {
			resources.LogLevel("error", "error getting sestatus output from %s", ip)
		}
		if seStatusOut != "" {
			resources.LogLevel("debug", "sestatus on %s:\n%s", ip, seStatusOut)
		}

		resources.LogGrepOutput("/var/log/audit/audit.log", "denied", ip)
	}
}
