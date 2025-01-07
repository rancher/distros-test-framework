package testcase

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/rancher/distros-test-framework/pkg/assert"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/gomega"
)

// TestInternodeConnectivityMixedOS validates communication between linux and windows nodes.
func TestInternodeConnectivityMixedOS(cluster *shared.Cluster, applyWorkload, deleteWorkload bool) {
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
func testIPsInCIDRRange(cluster *shared.Cluster, label, svc string) {
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
		return errors.New("slice parameters must have equal length")
	}
	if len(services) < 2 || len(ports) < 2 || len(expected) < 2 {
		return errors.New("slice parameters must not be less than or equal to 2")
	}

	shared.LogLevel("info", "Connecting to services")
	<-delay

	performCheck := func(svc1, svc2, port, expected string) error {
		cmd = fmt.Sprintf("kubectl exec svc/%s --kubeconfig=%s -- curl -m7 %s:%s", svc1,
			shared.KubeConfigFile, svc2, port)

		for {
			select {
			case <-timeout:
				return errors.New("timeout reached")
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

func TestEndpointReadiness(cluster *shared.Cluster) {
	var err error
	var wg sync.WaitGroup
	var listenPort = fmt.Sprintf("--cert /var/lib/rancher/%s/server/tls/client-ca.crt",
		cluster.Config.Product) + fmt.Sprintf(" --key  /var/lib/rancher/%s/server/tls/client-ca.key",
		cluster.Config.Product)

	endpoints := []string{
		"sudo curl -sk http://127.0.0.1:10248/healthz",  // kubelet
		"sudo curl -sk http://127.0.0.1:10249/healthz",  // kube-proxy
		"sudo curl -sk https://127.0.0.1:10257/healthz", // kube-controller
		"sudo curl -sk https://127.0.0.1:10258/healthz", // cloud-controller
		"sudo curl -sk https://127.0.0.1:10259/healthz", // kube-scheduler
		"sudo curl -sk  " + listenPort + " https://127.0.0.1:6443/healthz",
	}

	controlPlaneNodes, err := shared.GetNodesByRoles("control-plane")
	if err != nil {
		// handle the error, e.g. log or return an error
		fmt.Println("Error getting nodes by roles:", err)
		return
	}

	for _, serverIP := range controlPlaneNodes {
		for _, endpoint := range endpoints {
			wg.Add(1)
			go func(serverIP, endpoint string) {
				defer wg.Done()
				shared.LogLevel("info", "Checking endpoint %s on server %s", endpoint, serverIP)
				err = assert.CheckComponentCmdNode(endpoint, serverIP, "ok")
				if err != nil {
					shared.LogLevel("error", "Error checking endpoint %s on server %s: %v\n", endpoint, serverIP, err)
				}
			}(serverIP.ExternalIP, endpoint)
		}
	}
	wg.Wait()
	Expect(err).NotTo(HaveOccurred(), err)
}

func Testk8sAPIReady(cluster *shared.Cluster) {
	for _, serverIP := range cluster.ServerIPs {
		err := assert.CheckComponentCmdNode(
			"kubectl get --raw='http://127.0.0.1/readyz?verbose'",
			serverIP,
			"readyz check passed",
		)
		Expect(err).NotTo(HaveOccurred(), err)
	}
}

func Testk8sAPILive(cluster *shared.Cluster) {
	for _, serverIP := range cluster.ServerIPs {
		err := assert.CheckComponentCmdNode(
			"kubectl get --raw='http://127.0.0.1/livez?verbose'",
			serverIP,
			"livez check passed",
		)
		Expect(err).NotTo(HaveOccurred(), err)
	}
}
