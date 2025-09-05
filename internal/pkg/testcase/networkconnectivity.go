package testcase

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rancher/distros-test-framework/internal/pkg/assert"
	"github.com/rancher/distros-test-framework/internal/resources"

	. "github.com/onsi/gomega"
)

// TestInternodeConnectivityMixedOS validates communication between linux and windows nodes.
func TestInternodeConnectivityMixedOS(cluster *resources.Cluster, applyWorkload, deleteWorkload bool) {
	var workloadErr error
	if applyWorkload {
		workloadErr = resources.ManageWorkload("apply",
			"pod_client.yaml", "windows_app_deployment.yaml")
		Expect(workloadErr).NotTo(HaveOccurred(), "workload pod_client and/or windows not deployed")
	}

	checkPodsRunning := "kubectl get pods -n default -l app=client" +
		" --field-selector=status.phase=Running --kubeconfig="
	err := assert.ValidateOnHost(checkPodsRunning+resources.KubeConfigFile, statusRunning)
	Expect(err).NotTo(HaveOccurred(), err)

	checkPodsRunning = "kubectl get pods -n default -l app=windows-app" +
		" --field-selector=status.phase=Running  --kubeconfig="
	err = assert.ValidateOnHost(checkPodsRunning+resources.KubeConfigFile, statusRunning)
	Expect(err).NotTo(HaveOccurred(), err)

	assert.ValidatePodIPByLabel(cluster, []string{"app=client", "app=windows-app"}, []string{"10.42", "10.42"})

	err = testCrossNodeService(
		[]string{"client-curl", "windows-app-svc"},
		[]string{"8080", "3000"},
		[]string{"Welcome to nginx", "Welcome to PSTools"})
	Expect(err).NotTo(HaveOccurred(), "Error testing cross node service: %v", err)

	if deleteWorkload {
		workloadErr = resources.ManageWorkload("delete",
			"pod_client.yaml", "windows_app_deployment.yaml")
		Expect(workloadErr).NotTo(HaveOccurred())
	}
}

// testIPsInCIDRRange Validates Pod IPs and Cluster IPs in CIDR range.
func testIPsInCIDRRange(cluster *resources.Cluster, label, svc string) {
	nodeArgs, err := resources.GetNodeArgsMap(cluster, "server")
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
	timeout := time.After(300 * time.Second)
	ticker := time.NewTicker(30 * time.Second)
	delay := time.After(160 * time.Second)

	if len(services) != len(ports) && len(ports) != len(expected) {
		return errors.New("slice parameters must have equal length")
	}
	if len(services) < 2 || len(ports) < 2 || len(expected) < 2 {
		return errors.New("slice parameters must not be less than or equal to 2")
	}

	resources.LogLevel("info", "Connecting to services")
	<-delay

	performCheck := func(svc1, svc2, port, expected string) error {
		cmd = fmt.Sprintf("kubectl exec svc/%s --kubeconfig=%s -- curl -m7 %s:%s", svc1,
			resources.KubeConfigFile, svc2, port)

		resources.LogLevel("debug", "checking cmd: %v", cmd)
		for {
			select {
			case <-timeout:
				return errors.New("timeout reached")
			case <-ticker.C:
				result, err := resources.RunCommandHost(cmd)
				if err != nil {
					resources.LogLevel("debug", "result: %v\nerror: %v", result, err)
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
