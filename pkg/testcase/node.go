package testcase

import (
	"fmt"

	"github.com/rancher/distros-test-framework/factory"
	"github.com/rancher/distros-test-framework/pkg/assert"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// TestNodeStatus test the status of the nodes in the cluster using 2 custom assert functions
func TestNodeStatus(
	nodeAssertReadyStatus assert.NodeAssertFunc,
	nodeAssertVersion assert.NodeAssertFunc,
) {
	cluster := factory.GetCluster(GinkgoT())

	expectedNodeCount := cluster.NumServers + cluster.NumAgents + cluster.NumWinAgents
	Eventually(func(g Gomega) {
		nodes, err := shared.ParseNodes(false)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(len(nodes)).To(Equal(expectedNodeCount),
			"Number of nodes should match the spec")
		for _, node := range nodes {
			if nodeAssertReadyStatus != nil {
				nodeAssertReadyStatus(g, node)
			}
			if nodeAssertVersion != nil {
				nodeAssertVersion(g, node)
			}
		}
	}, "1200s", "60s").Should(Succeed())

	fmt.Println("\nRetrieving Cluster nodes:")
	_, err := shared.ParseNodes(true)
	Expect(err).NotTo(HaveOccurred())
}

// TestInternodeConnectivityMixedOS Deploys services in the cluster and validates communication between linux and windows nodes
func TestInternodeConnectivityMixedOS() {
	_, err := shared.ManageWorkload("apply", "",
		"pod_client.yaml","windows_app_deployment.yaml")
	if err != nil {
		fmt.Errorf("Error applying workload: ", err)
		return
	}
	
	assert.ValidatePodIPByLabel([]string{"app=client","app=windows-app"},[]string{"10.42","10.42"})

	err = testCrossNodeService(
		[]string{"client-curl", "windows-app-svc"}, 
		[]string{"8080", "3000"}, 
		[]string{"Welcome to nginx", "Welcome to PSTools"})
	if err != nil {
		fmt.Errorf("Error checking cross node service: ", err)
		return
	}
}

// testCrossNodeService Perform testing cross node communication via service exec call
//
// services array Takes service names as parameters in the array
//
// ports	array Takes service ports needed to access the services
//
// expected	array Takes the expected substring from the curl response
func testCrossNodeService(services, ports, expected []string) error{
	var cmd string

	if len(services) != len(ports) && len(ports) != len(expected){
		return fmt.Errorf("array parameters must have equal length")
	}
	if len(services) < 2 || len(ports) < 2 || len(expected) < 2{
		return fmt.Errorf("array parameters must not be less than or equal to 2")
	}

	for i := 0; i < len(services); i++ {
		for j := i+1; j < len(services); j++ {
			cmd = fmt.Sprintf("kubectl exec svc/%s --kubeconfig=%s -- curl -m7 %s:%s", 
				services[i], shared.KubeConfigFile, services[j], ports[j])
			Eventually(func() (string, error) {
				return shared.RunCommandHost(cmd)
			}, "300s", "60s").Should(ContainSubstring(expected[j]))
		}
	}

	for i := len(services)-1; i > 0; i-- {
		for j := 1; j <= i; j++ {
			cmd = fmt.Sprintf("kubectl exec svc/%s --kubeconfig=%s -- curl -m7 %s:%s", 
				services[i], shared.KubeConfigFile, services[i-j], ports[i-j])
			Eventually(func() (string, error) {
				return shared.RunCommandHost(cmd)
			}, "300s", "60s").Should(ContainSubstring(expected[i-j]))
		}
	}

	return nil
}
