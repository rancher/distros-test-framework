package testcase

import (
	"fmt"

	"github.com/rancher/distros-test-framework/pkg/assert"
	"github.com/rancher/distros-test-framework/pkg/customflag"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestServiceClusterIp(deployWorkload bool) {
	if deployWorkload {
		fmt.Println("ARCH", customflag.ServiceFlag.ClusterConfig.Arch.String())
		_, err := shared.ManageWorkload(
			"create",
			"clusterip.yaml",
			customflag.ServiceFlag.ClusterConfig.Arch.String(),
		)
		Expect(err).NotTo(HaveOccurred(),
			"Cluster IP manifest not deployed")
	}

	getClusterIp := "kubectl get pods -n test-clusterip -l k8s-app=nginx-app-clusterip " +
		"--field-selector=status.phase=Running --kubeconfig="
	err := assert.ValidateOnHost(getClusterIp+shared.KubeConfigFile, Running)
	if err != nil {
		GinkgoT().Errorf("%v", err)
	}

	clusterip, port, _ := shared.FetchClusterIP("test-clusterip", "nginx-clusterip-svc")
	nodeExternalIP := shared.FetchNodeExternalIP()
	for _, ip := range nodeExternalIP {
		err = assert.ValidateOnNode(ip, "curl -sL --insecure http://"+clusterip+
			":"+port+"/name.html", "test-clusterip")
		if err != nil {
			GinkgoT().Errorf("Error: %v", err)
		}
	}
}

func TestServiceNodePort(deployWorkload bool) {
	if deployWorkload {
		_, err := shared.ManageWorkload(
			"create",
			"nodeport.yaml",
			customflag.ServiceFlag.ClusterConfig.Arch.String(),
		)
		Expect(err).NotTo(HaveOccurred(),
			"NodePort manifest not deployed")
	}

	nodeExternalIP := shared.FetchNodeExternalIP()
	nodeport, err := shared.FetchServiceNodePort("test-nodeport", "nginx-nodeport-svc")
	if err != nil {
		GinkgoT().Errorf("%v", err)
	}

	getNodeport := "kubectl get pods -n test-nodeport -l k8s-app=nginx-app-nodeport " +
		"--field-selector=status.phase=Running --kubeconfig="
	for _, ip := range nodeExternalIP {
		err = assert.ValidateOnHost(
			getNodeport+shared.KubeConfigFile,
			Running,
		)
		if err != nil {
			GinkgoT().Errorf("%v", err)
		}

		err = assert.CheckComponentCmdNode(
		    "curl -sL --insecure http://"+""+ip+":"+nodeport+"/name.html",
		    ip,
		    "test-nodeport")
	}
	if err != nil {
		GinkgoT().Errorf("%v", err)
	}
}

func TestServiceLoadBalancer(deployWorkload bool) {
	if deployWorkload {
		_, err := shared.ManageWorkload(
			"create",
			"loadbalancer.yaml",
			customflag.ServiceFlag.ClusterConfig.Arch.String(),
		)
		Expect(err).NotTo(HaveOccurred(),
			"Loadbalancer manifest not deployed")
	}

	getLoadbalancerSVC := "kubectl get service -n test-loadbalancer nginx-loadbalancer-svc" +
		" --output jsonpath={.spec.ports[0].port} --kubeconfig="
	port, err := shared.RunCommandHost(getLoadbalancerSVC + shared.KubeConfigFile)
	if err != nil {
		GinkgoT().Errorf("%v", err)
	}

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
		if err != nil {
			GinkgoT().Errorf("%v", err)
		}
	}
}
