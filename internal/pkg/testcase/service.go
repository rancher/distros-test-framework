package testcase

import (
	"strings"

	"github.com/rancher/distros-test-framework/internal/pkg/assert"
	"github.com/rancher/distros-test-framework/internal/provisioning/driver"
	"github.com/rancher/distros-test-framework/internal/resources"

	. "github.com/onsi/gomega"
)

func TestServiceClusterIP(applyWorkload, deleteWorkload bool) {
	var workloadErr error
	if applyWorkload {
		workloadErr = resources.ManageWorkload("apply", "clusterip.yaml")
		Expect(workloadErr).NotTo(HaveOccurred(), "Cluster IP manifest not deployed")
	}
	getClusterIP := "kubectl get pods -n test-clusterip -l k8s-app=nginx-app-clusterip " +
		"--field-selector=status.phase=Running --kubeconfig="
	err := assert.ValidateOnHost(getClusterIP+resources.KubeConfigFile, statusRunning)
	Expect(err).NotTo(HaveOccurred(), err)

	clusterip, port, _ := resources.FetchClusterIPs("test-clusterip", "nginx-clusterip-svc")

	nodeExternalIP := resources.FetchNodeExternalIPs()
	for _, ip := range nodeExternalIP {
		err = assert.ValidateOnNode(ip, "curl -sL --insecure http://"+clusterip+
			":"+port+"/name.html", "test-clusterip")
		Expect(err).NotTo(HaveOccurred(), err)
	}

	if deleteWorkload {
		workloadErr = resources.ManageWorkload("delete", "clusterip.yaml")
		Expect(workloadErr).NotTo(HaveOccurred(), "Cluster IP manifest not deleted")
	}
}

func TestServiceNodePort(applyWorkload, deleteWorkload bool) {
	var workloadErr error
	if applyWorkload {
		workloadErr = resources.ManageWorkload("apply", "nodeport.yaml")
		Expect(workloadErr).NotTo(HaveOccurred(), "nodeport manifest not deployed")
	}

	nodeExternalIP := resources.FetchNodeExternalIPs()
	nodeport, err := resources.FetchServiceNodePort("test-nodeport", "nginx-nodeport-svc")
	Expect(err).NotTo(HaveOccurred(), err)

	getNodeport := "kubectl get pods -n test-nodeport -l k8s-app=nginx-app-nodeport " +
		"--field-selector=status.phase=Running --kubeconfig="
	err = assert.ValidateOnHost(
		getNodeport+resources.KubeConfigFile,
		statusRunning,
	)
	Expect(err).NotTo(HaveOccurred(), err)

	expectedPodName := "test-nodeport"
	for _, ip := range nodeExternalIP {
		err = assert.ValidateOnHost(
			"curl -sL --insecure http://"+""+ip+":"+nodeport+"/name.html",
			expectedPodName)
	}
	Expect(err).NotTo(HaveOccurred(), err)

	if deleteWorkload {
		workloadErr = resources.ManageWorkload("delete", "nodeport.yaml")
		Expect(workloadErr).NotTo(HaveOccurred(), "NodePort manifest not deleted")
	}
}

func TestServiceLoadBalancer(applyWorkload, deleteWorkload bool) {
	var workloadErr error
	if applyWorkload {
		workloadErr = resources.ManageWorkload("apply", "loadbalancer.yaml")
		Expect(workloadErr).NotTo(HaveOccurred(), "loadbalancer manifest not deployed")
	}

	getLoadbalancerSVC := "kubectl get service -n test-loadbalancer nginx-loadbalancer-svc" +
		" --output jsonpath={.spec.ports[0].port} --kubeconfig="
	port, err := resources.RunCommandHost(getLoadbalancerSVC + resources.KubeConfigFile)
	Expect(err).NotTo(HaveOccurred(), err)

	getAppLoadBalancer := "kubectl get pods -n test-loadbalancer  " +
		"--field-selector=status.phase=Running --kubeconfig="
	expectedPodName := "test-loadbalancer"
	validNodes, err := resources.GetNodesByRoles("control-plane", "worker")
	Expect(err).NotTo(HaveOccurred(), err)

	err = assert.ValidateOnHost(
		getAppLoadBalancer+resources.KubeConfigFile,
		expectedPodName)
	Expect(err).NotTo(HaveOccurred(), err)

	for _, node := range validNodes {
		err = assert.ValidateOnHost(
			"curl -sL --insecure http://"+node.ExternalIP+":"+port+"/name.html",
			expectedPodName)
		Expect(err).NotTo(HaveOccurred(), err)
	}

	if deleteWorkload {
		workloadErr = resources.ManageWorkload("delete", "loadbalancer.yaml")
		Expect(workloadErr).NotTo(HaveOccurred(), "Loadbalancer manifest not deleted")
	}
}

func testServiceNodePortDualStack(cluster *driver.Cluster, td testData) {
	nodeExternalIP := resources.FetchNodeExternalIPs()
	nodeport, err := resources.FetchServiceNodePort(td.Namespace, td.SVC)
	Expect(err).NotTo(HaveOccurred(), err)

	for _, ip := range nodeExternalIP {
		if strings.Contains(ip, ":") {
			ip = resources.EncloseSqBraces(ip)
		}
		err = assert.CheckComponentCmdNode(
			"curl -sL --insecure http://"+ip+":"+nodeport+"/name.html",
			cluster.Bastion.PublicIPv4Addr,
			td.Expected)
		Expect(err).NotTo(HaveOccurred(), err)
	}
}

func testServiceClusterIPs(td testData) {
	clusterIPs, port, err := resources.FetchClusterIPs(td.Namespace, td.SVC)
	clusterIPSlice := strings.Split(clusterIPs, " ")
	Expect(err).NotTo(HaveOccurred(), err)
	nodeExternalIPs := resources.FetchNodeExternalIPs()

	for _, clusterIP := range clusterIPSlice {
		if strings.Contains(clusterIP, ":") {
			clusterIP = resources.EncloseSqBraces(clusterIP)
		}
		err := assert.ValidateOnNode(nodeExternalIPs[0],
			"curl -sL --insecure http://"+clusterIP+":"+port, td.Expected)
		Expect(err).NotTo(HaveOccurred(), err)
	}
}
