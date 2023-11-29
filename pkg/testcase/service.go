package testcase

import (
	"github.com/rancher/distros-test-framework/pkg/assert"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/gomega"
)

func TestServiceClusterIp(applyWorkload, deleteWorkload bool) {
	var workloadErr error
	if applyWorkload {
		workloadErr = shared.ManageWorkload("apply", "clusterip.yaml")
		Expect(workloadErr).NotTo(HaveOccurred(), "Cluster IP manifest not deployed")
	}
	getClusterIP := "kubectl get pods -n test-clusterip -l k8s-app=nginx-app-clusterip " +
		"--field-selector=status.phase=Running --kubeconfig="
	err := assert.ValidateOnHost(getClusterIP+shared.KubeConfigFile, statusRunning)
	Expect(err).NotTo(HaveOccurred(), err)

	clusterip, port, _ := shared.FetchClusterIP("test-clusterip", "nginx-clusterip-svc")
	nodeExternalIP := shared.FetchNodeExternalIP()
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

	nodeExternalIP := shared.FetchNodeExternalIP()
	nodeport, err := shared.FetchServiceNodePort("test-nodeport", "nginx-nodeport-svc")
	Expect(err).NotTo(HaveOccurred(), err)

	getNodeport := "kubectl get pods -n test-nodeport -l k8s-app=nginx-app-nodeport " +
		"--field-selector=status.phase=Running --kubeconfig="
	for _, ip := range nodeExternalIP {
		err = assert.ValidateOnHost(
			getNodeport+shared.KubeConfigFile,
			statusRunning,
		)
		Expect(err).NotTo(HaveOccurred(), err)

		err = assert.CheckComponentCmdNode(
			"curl -sL --insecure http://"+""+ip+":"+nodeport+"/name.html",
			ip,
			"test-nodeport")
	}
	Expect(err).NotTo(HaveOccurred(), err)

	if deleteWorkload {
		workloadErr = shared.ManageWorkload("delete", "nodeport.yaml")
		Expect(workloadErr).NotTo(HaveOccurred(), "NodePort manifest not deleted")
	}
}

func TestServiceLoadBalancer(applyWorkload, deleteWorkload bool) {
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
	loadBalancer := "test-loadbalancer"
	nodeExternalIP := shared.FetchNodeExternalIP()
	for _, ip := range nodeExternalIP {
		err = assert.ValidateOnHost(
			getAppLoadBalancer+shared.KubeConfigFile,
			loadBalancer,
			"curl -sL --insecure http://"+ip+":"+port+"/name.html",
			loadBalancer,
		)
		Expect(err).NotTo(HaveOccurred(), err)
	}

	if deleteWorkload {
		workloadErr = shared.ManageWorkload("delete", "loadbalancer.yaml")
		Expect(workloadErr).NotTo(HaveOccurred(), "Loadbalancer manifest not deleted")
	}
}
