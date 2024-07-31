package testcase

import (
	"fmt"
	"strings"
	"time"

	"github.com/rancher/distros-test-framework/factory"
	"github.com/rancher/distros-test-framework/pkg/assert"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/gomega"
	// . "github.com/onsi/gomega/gstruct"
)

// TestInternodeConnectivityMixedOS validates communication between linux and windows nodes.
func TestInternodeConnectivityMixedOS(cluster *factory.Cluster, applyWorkload, deleteWorkload bool) {
	var workloadErr error
	if applyWorkload {
		workloadErr = shared.ManageWorkload("apply",
			"pod_client.yaml", "windows_app_deployment.yaml")
		Expect(workloadErr).NotTo(HaveOccurred(), "workload pod_client and/or windows not deployed")
	}

	assert.ValidatePodIPByLabel(cluster, []string{"app=client", "app=windows-app"}, []string{"10.42", "10.42"})

	err := testCrossNodeService(
		[]string{"client-curl", "windows-app-svc"},
		[]string{"8080", "3000"},
		[]string{"Welcome to nginx", "Welcome to PSTools"})
	Expect(err).NotTo(HaveOccurred())

	if deleteWorkload {
		workloadErr = shared.ManageWorkload("delete",
			"pod_client.yaml", "windows_app_deployment.yaml")
		Expect(workloadErr).NotTo(HaveOccurred())
	}
}

// testIPsInCIDRRange Validates Pod IPs and Cluster IPs in CIDR range.
func testIPsInCIDRRange(cluster *factory.Cluster, label, svc string) {
	nodeArgs, err := shared.GetNodeArgsMap(cluster, "server")
	Expect(err).NotTo(HaveOccurred(), err)

	clusterCIDR := strings.Split(nodeArgs["cluster-cidr"], ",")
	serviceCIDR := strings.Split(nodeArgs["service-cidr"], ",")

	assert.ValidatePodIPsByLabel(label, clusterCIDR)
	assert.ValidateClusterIPsBySVC(svc, serviceCIDR)
}

// testCrossNodeService Perform testing cross node communication via service exec call.
//
// services Slice Takes service names as parameters in the array.
//
// ports	Slice Takes service ports needed to access the services.
//
// expected	Slice Takes the expected substring from the curl response.
func testCrossNodeService(services, ports, expected []string) error {
	var cmd string
	timeout := time.After(220 * time.Second)
	ticker := time.NewTicker(10 * time.Second)
	delay := time.After(160 * time.Second)

	if len(services) != len(ports) && len(ports) != len(expected) {
		return fmt.Errorf("slice parameters must have equal length")
	}
	if len(services) < 2 || len(ports) < 2 || len(expected) < 2 {
		return fmt.Errorf("slice parameters must not be less than or equal to 2")
	}

	shared.LogLevel("info", "Connecting to services")
	<-delay

	performCheck := func(svc1, svc2, port, expected string) error {
		cmd = fmt.Sprintf("kubectl exec svc/%s --kubeconfig=%s -- curl -m7 %s:%s", svc1,
			factory.KubeConfigFile, svc2, port)

		for {
			select {
			case <-timeout:
				return fmt.Errorf("timeout reached")
			case <-ticker.C:
				result, err := shared.RunCommandHost(cmd)
				if err != nil {
					return err
				}
				if strings.Contains(result, expected) {
					return nil
				}
			}
		}
	}

	for i := 0; i < len(services); i++ {
		for j := i + 1; j < len(services); j++ {
			if err := performCheck(services[i], services[j], ports[j], expected[j]); err != nil {
				return err
			}
		}
	}

	for i := len(services) - 1; i > 0; i-- {
		for j := 1; j <= i; j++ {
			if err := performCheck(services[i], services[i-j], ports[i-j], expected[i-j]); err != nil {
				return err
			}
		}
	}

	return nil
}

func TestEndpointReadiness(cluster *factory.Cluster) {
	//do more checks on the filesystem to ensure the certs are all created and in the correct location before this
	commands := []string{
		"sudo curl -sk http://127.0.0.1:10248/healthz",  //kubelet
		"sudo curl -sk http://127.0.0.1:10249/healthz",  //kube-proxy
		"sudo curl -sk https://127.0.0.1:10257/healthz", //kube-controller
		"sudo curl -sk https://127.0.0.1:10258/healthz", //cloud-controller
		"sudo curl -sk https://127.0.0.1:10259/healthz", //kube-scheduler
		"sudo curl -sk  " + fmt.Sprintf("--cert /var/lib/rancher/%s/server/tls/client-ca.crt", cluster.Config.Product) + fmt.Sprintf(" --key  /var/lib/rancher/%s/server/tls/client-ca.key", cluster.Config.Product) + " https://127.0.0.1:6443/healthz",
		// {Command: "sudo curl -sk http://127.0.0.1:10256/healthz"}, //SearchString: "lastUpdated" or "nodeEligible: true" //check with devs for this versus second kube-proxy port
		// "sudo curl -sk " + fmt.Sprintf("--cert /var/lib/rancher/%s/server/tls/etcd/server-client.crt", cluster.Config.Product) + fmt.Sprintf(" --key /var/lib/rancher/%s/server/tls/etcd/server-client.key", cluster.Config.Product) + " https://127.0.0.1:2379/livez?verbose",
	}
	var err error
	for _, serverIP := range cluster.ServerIPs {
		for _, endpoint := range commands {
			fmt.Printf("Running command %s against server %s", commands, serverIP)
			err = assert.CheckComponentCmdNode(
				endpoint,
				serverIP,
				"ok")
		}
	}
	Expect(err).NotTo(HaveOccurred(), err)
}

func Testk8sAPIReady(cluster *factory.Cluster) {
	for _, serverIP := range cluster.ServerIPs {
		err := assert.CheckComponentCmdNode(
			"kubectl get --raw='/readyz?verbose'",
			serverIP,
			"readyz check passed",
		)
		Expect(err).NotTo(HaveOccurred(), err)
	}
}

func Testk8sAPILive(cluster *factory.Cluster) {
	for _, serverIP := range cluster.ServerIPs {
		err := assert.CheckComponentCmdNode(
			"kubectl get --raw='/livez?verbose'",
			serverIP,
			"livez check passed",
		)
		Expect(err).NotTo(HaveOccurred(), err)
	}
}
