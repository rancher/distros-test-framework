package testcase

import (
	"strings"

	"github.com/rancher/distros-test-framework/pkg/assert"
	"github.com/rancher/distros-test-framework/pkg/aws"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/gomega"
)

func TestServiceClusterIP(applyWorkload, deleteWorkload bool) {
	var workloadErr error
	if applyWorkload {
		workloadErr = shared.ManageWorkload("apply", "clusterip.yaml")
		Expect(workloadErr).NotTo(HaveOccurred(), "Cluster IP manifest not deployed")
	}
	getClusterIP := "kubectl get pods -n test-clusterip -l k8s-app=nginx-app-clusterip " +
		"--field-selector=status.phase=Running --kubeconfig="
	err := assert.ValidateOnHost(getClusterIP+shared.KubeConfigFile, statusRunning)
	Expect(err).NotTo(HaveOccurred(), err)

	clusterip, port, _ := shared.FetchClusterIPs("test-clusterip", "nginx-clusterip-svc")

	nodeExternalIP := shared.FetchNodeExternalIPs()
	for _, ip := range nodeExternalIP {
		err = assert.ValidateOnNode(ip, "curl -sL --insecure http://"+clusterip+
			":"+port+"/name.html", "test-clusterip")
		Expect(err).NotTo(HaveOccurred(), err)
	}

	if deleteWorkload {
		workloadErr = shared.ManageWorkload("delete", "clusterip.yaml")
		Expect(workloadErr).NotTo(HaveOccurred(), "Cluster IP manifest not deleted")
	}
}

func TestServiceNodePort(applyWorkload, deleteWorkload bool) {
	var workloadErr error
	if applyWorkload {
		workloadErr = shared.ManageWorkload("apply", "nodeport.yaml")
		Expect(workloadErr).NotTo(HaveOccurred(), "nodeport manifest not deployed")
	}

	nodeExternalIP := shared.FetchNodeExternalIPs()
	nodeport, err := shared.FetchServiceNodePort("test-nodeport", "nginx-nodeport-svc")
	Expect(err).NotTo(HaveOccurred(), err)

	getNodeport := "kubectl get pods -n test-nodeport -l k8s-app=nginx-app-nodeport " +
		"--field-selector=status.phase=Running --kubeconfig="
	err = assert.ValidateOnHost(
		getNodeport+shared.KubeConfigFile,
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
		workloadErr = shared.ManageWorkload("delete", "nodeport.yaml")
		Expect(workloadErr).NotTo(HaveOccurred(), "NodePort manifest not deleted")
	}
}

var newNodeIP string

func TestServiceLoadBalancer(cluster *shared.Cluster, awsClient *aws.Client, applyWorkload, deleteWorkload bool) {
	if newNodeIP == "" {
		newNodeName := "distros-qa-test-node-" + cluster.Config.Product
		externalIPs, _, _, _ := awsClient.CreateInstances(newNodeName)
		Expect(externalIPs).NotTo(BeEmpty(), "error creating instance, externalIPs empty")

		newNodeIP = externalIPs[0]

		shared.LogLevel("info", "new node ip: %s", newNodeIP)
	} else {
		shared.LogLevel("info", "new node ip already exists: %s", newNodeIP)
	}

	var workloadErr error
	if applyWorkload {
		workloadErr = shared.ManageWorkload("apply", "loadbalancer.yaml")
		Expect(workloadErr).NotTo(HaveOccurred(), "loadbalancer manifest not deployed")
	}

	getLoadbalancerSVC := "kubectl get service -n test-loadbalancer nginx-loadbalancer-svc" +
		" --output jsonpath={.spec.ports[0].port} --kubeconfig="
	port, err := shared.RunCommandHost(getLoadbalancerSVC + shared.KubeConfigFile)
	Expect(err).NotTo(HaveOccurred(), err)

	getAppLoadBalancer := "kubectl get pods -n test-loadbalancer  " +
		"--field-selector=status.phase=Running --kubeconfig="
	expectedPodName := "test-loadbalancer"
	validNodes, err := shared.GetNodesByRoles("control-plane", "worker")
	Expect(err).NotTo(HaveOccurred(), err)

	err = assert.ValidateOnHost(
		getAppLoadBalancer+shared.KubeConfigFile,
		expectedPodName)
	Expect(err).NotTo(HaveOccurred(), err)

	for _, node := range validNodes {
		err = assert.ValidateOnNode(
			newNodeIP,
			"curl -sL --insecure http://"+node.ExternalIP+":"+port+"/name.html",
			expectedPodName)
		Expect(err).NotTo(HaveOccurred(), err)
	}

	if deleteWorkload {
		workloadErr = shared.ManageWorkload("delete", "loadbalancer.yaml")
		Expect(workloadErr).NotTo(HaveOccurred(), "Loadbalancer manifest not deleted")

		delErr := awsClient.DeleteInstance(newNodeIP)
		Expect(delErr).NotTo(HaveOccurred(), delErr)
	}
}

func testServiceNodePortDualStack(cluster *shared.Cluster, td testData) {
	nodeExternalIP := shared.FetchNodeExternalIPs()
	nodeport, err := shared.FetchServiceNodePort(td.Namespace, td.SVC)
	Expect(err).NotTo(HaveOccurred(), err)

	for _, ip := range nodeExternalIP {
		if strings.Contains(ip, ":") {
			ip = shared.EncloseSqBraces(ip)
		}
		err = assert.CheckComponentCmdNode(
			"curl -sL --insecure http://"+ip+":"+nodeport+"/name.html",
			cluster.BastionConfig.PublicIPv4Addr,
			td.Expected)
		Expect(err).NotTo(HaveOccurred(), err)
	}
}

func testServiceClusterIPs(td testData) {
	clusterIPs, port, err := shared.FetchClusterIPs(td.Namespace, td.SVC)
	clusterIPSlice := strings.Split(clusterIPs, " ")
	Expect(err).NotTo(HaveOccurred(), err)
	nodeExternalIPs := shared.FetchNodeExternalIPs()

	for _, clusterIP := range clusterIPSlice {
		if strings.Contains(clusterIP, ":") {
			clusterIP = shared.EncloseSqBraces(clusterIP)
		}
		err := assert.ValidateOnNode(nodeExternalIPs[0],
			"curl -sL --insecure http://"+clusterIP+":"+port, td.Expected)
		Expect(err).NotTo(HaveOccurred(), err)
	}
}
