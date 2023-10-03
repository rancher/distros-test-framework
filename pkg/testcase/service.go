package testcase

import (
	"strings"

	"github.com/rancher/distros-test-framework/pkg/assert"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/gomega"
)

func TestServiceClusterIp(deleteWorkload bool) {
	_, err := shared.ManageWorkload("apply", "clusterip.yaml")
	Expect(err).NotTo(HaveOccurred(), "Cluster IP manifest not deployed")

	getClusterIP := "kubectl get pods -n test-clusterip -l k8s-app=nginx-app-clusterip " +
		"--field-selector=status.phase=Running --kubeconfig="
	err = assert.ValidateOnHost(getClusterIP+shared.KubeConfigFile, statusRunning)
	Expect(err).NotTo(HaveOccurred(), err)

	clusterip, port, _ := shared.FetchClusterIP("test-clusterip", "nginx-clusterip-svc")
	nodeExternalIP := shared.FetchNodeExternalIP()
	for _, ip := range nodeExternalIP {
		err = assert.ValidateOnNode(ip, "curl -sL --insecure http://"+clusterip+
			":"+port+"/name.html", "test-clusterip")
		Expect(err).NotTo(HaveOccurred(), err)
	}

	if deleteWorkload {
		_, err := shared.ManageWorkload("delete", "clusterip.yaml")
		Expect(err).NotTo(HaveOccurred(), "Cluster IP manifest not deleted")
	}
}

func TestServiceNodePort(deleteWorkload bool) {
	_, err := shared.ManageWorkload("apply", "nodeport.yaml")
	Expect(err).NotTo(HaveOccurred(), "NodePort manifest not deployed")

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
		_, err := shared.ManageWorkload("delete", "nodeport.yaml")
		Expect(err).NotTo(HaveOccurred(), "NodePort manifest not deleted")
	}
}

func TestServiceLoadBalancer(deleteWorkload bool) {
	_, err := shared.ManageWorkload("apply", "loadbalancer.yaml")
	Expect(err).NotTo(HaveOccurred(), "Loadbalancer manifest not deployed")

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
		_, err := shared.ManageWorkload("delete", "loadbalancer.yaml")
		Expect(err).NotTo(HaveOccurred(), "Loadbalancer manifest not deleted")
	}
}

func TestServiceNodePortDualStack(testinfo map[string]string) {
	nodeExternalIP := shared.FetchNodeExternalIP()
	nodeport, err := shared.FetchServiceNodePort(testinfo["namespace"], testinfo["svc"])
	Expect(err).NotTo(HaveOccurred(), err)
	
	for _, ip := range nodeExternalIP {
		if strings.Contains(ip,":") {
			ip = shared.EncloseInBrackets(ip)
		}
		err = assert.CheckComponentCmdNode(
			"curl -sL --insecure http://"+ip+":"+nodeport+"/name.html",
			testinfo["expected"],
			shared.BastionIP)
		Expect(err).NotTo(HaveOccurred(), err)
	}
}

func TestServiceClusterIPDualStack(testInfo map[string]string) {		
	res, port, _ := shared.FetchClusterIPs(testInfo["namespace"], testInfo["svc"])
	clusterIPs := strings.Split(res, " ")
	nodeExternalIPs := shared.FetchNodeExternalIP()
	
	for _, clusterIP := range clusterIPs {
		if strings.Contains(clusterIP,":") {
			clusterIP = shared.EncloseInBrackets(clusterIP)
		}
		err := assert.ValidateOnNode(nodeExternalIPs[0], 
			"curl -sL --insecure http://"+ clusterIP +":"+ port, testInfo["expected"])
		Expect(err).NotTo(HaveOccurred(), err)
	}
}


