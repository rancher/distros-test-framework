package testcase

import (
	"fmt"
	"strings"
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
		logPodData(cluster)
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
	dir := "/opt/data"
	cmdPart1 := fmt.Sprintf("test -d '%s' && echo 'directory exists: %s'", dir, dir)
	cmdPart2 := fmt.Sprintf("sudo mkdir -p %s", dir)
	cmd := fmt.Sprintf("%s || %s; sudo chmod +w %s; sudo ls -lrt %s", cmdPart1, cmdPart2, dir, dir)
	if cluster.NodeOS == "slemicro" {
		var output string
		var mkdirErr error
		for _, ip := range append(cluster.ServerIPs, cluster.AgentIPs...) {
			shared.LogLevel("debug", "create /opt/data directory with cmd: %s", cmd)
			output, mkdirErr = shared.RunCommandOnNode(cmd, ip)
			if mkdirErr != nil {
				shared.LogLevel("warn", "error creating /opt/data dir on node ip: %s", ip)
			}
			if output != "" {
				shared.LogLevel("debug", "create and check /opt/data output: %s", output)
			}
		}
	}
}

func logPodData(cluster *shared.Cluster) {
	shared.LogLevel("debug", "logging pod logs and describe pod output for pods with namespace: %s", namespace)
	// Pod Log with namespace: local-path-storage
	filters := map[string]string{
		"namespace": namespace,
	}
	pods, getErr := shared.GetPodsFiltered(filters)
	if getErr != nil {
		shared.LogLevel("error", "possibly no pods found with namespace: %s", namespace)
	}
	for i := range pods {
		if pods[i].NameSpace == "" {
			pods[i].NameSpace = namespace
		}
		shared.LoggerPodLogs(cluster, &pods[i])
		shared.DescribePod(cluster, &pods[i])
	}
	// Pod log for helper-pod-create-pvc
	shared.LogLevel("debug", "logging pod logs and describe pod output for pod with name: helper-pod-create-pvc*")
	filters = map[string]string{
		"namespace": "kube-system",
	}

	ksPods, getPodErr := shared.GetPodsFiltered(filters)
	if getPodErr != nil {
		shared.LogLevel("error", "error getting pods with namespace: kube-system")
	}
	for i := range ksPods {
		if strings.Contains(ksPods[i].Name, "helper-pod-create-pvc") {
			if ksPods[i].NameSpace == "" {
				ksPods[i].NameSpace = "kube-system"
			}
			shared.LoggerPodLogs(cluster, &ksPods[i])
			shared.DescribePod(cluster, &ksPods[i])
		}
	}
	// Log the kubectl get pv,pvc,storageclass
	output, getErr := shared.KubectlCommand(cluster, "node", "get", "pv,pvc,storageclass", "-A")
	if getErr != nil {
		shared.LogLevel("error", "error getting pv,pvc and storageclass info")
	}
	if output != "" {
		shared.LogLevel("debug", "pv,pvc,storageclass info:\n %s", output)
	}
}
