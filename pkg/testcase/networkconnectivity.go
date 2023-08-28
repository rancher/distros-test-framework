package testcase

import (
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/gomega"
	"github.com/rancher/distros-test-framework/pkg/assert"
	"github.com/rancher/distros-test-framework/shared"
)

// TestInternodeConnectivityMixedOS Deploys services in the cluster and validates communication between linux and windows nodes
func TestInternodeConnectivityMixedOS(deleteWorkload bool) {
	_, err := shared.ManageWorkload("apply",
		"pod_client.yaml", "windows_app_deployment.yaml")
	Expect(err).NotTo(HaveOccurred())

	assert.ValidatePodIPByLabel([]string{"app=client", "app=windows-app"}, []string{"10.42", "10.42"})

	err = testCrossNodeService(
		[]string{"client-curl", "windows-app-svc"},
		[]string{"8080", "3000"},
		[]string{"Welcome to nginx", "Welcome to PSTools"})
	Expect(err).NotTo(HaveOccurred())

	if deleteWorkload {
		_, err := shared.ManageWorkload("delete",
			"pod_client.yaml", "windows_app_deployment.yaml")
		Expect(err).NotTo(HaveOccurred())
	}
}

// testCrossNodeService Perform testing cross node communication via service exec call
//
// services Slice Takes service names as parameters in the array
//
// ports	Slice Takes service ports needed to access the services
//
// expected	Slice Takes the expected substring from the curl response
// func testCrossNodeService(services, ports, expected []string) error {
// 	var cmd string
//
// 	if len(services) != len(ports) && len(ports) != len(expected) {
// 		return fmt.Errorf("slice parameters must have equal length")
// 	}
// 	if len(services) < 2 || len(ports) < 2 || len(expected) < 2 {
// 		return fmt.Errorf("slice parameters must not be less than or equal to 2")
// 	}
//
// 	for i := 0; i < len(services); i++ {
// 		for j := i + 1; j < len(services); j++ {
// 			cmd = fmt.Sprintf("kubectl exec svc/%s --kubeconfig=%s -- curl -m7 %s:%s",
// 				services[i], shared.KubeConfigFile, services[j], ports[j])
// 			Eventually(func() (string, error) {
// 				return shared.RunCommandHost(cmd)
// 			}, "180s", "10s").Should(ContainSubstring(expected[j]))
// 		}
// 	}
//
// 	for i := len(services) - 1; i > 0; i-- {
// 		for j := 1; j <= i; j++ {
// 			cmd = fmt.Sprintf("kubectl exec svc/%s --kubeconfig=%s -- curl -m7 %s:%s",
// 				services[i], shared.KubeConfigFile, services[i-j], ports[i-j])
// 			Eventually(func() (string, error) {
// 				return shared.RunCommandHost(cmd)
// 			}, "180s", "10s").Should(ContainSubstring(expected[i-j]))
// 		}
// 	}
//
// 	return nil
// }

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

	fmt.Println("\nConnecting to services")
	<-delay

	performCheck := func(svc1, svc2, port, expected string) error {
		cmd = fmt.Sprintf("kubectl exec svc/%s --kubeconfig=%s -- curl -m7 %s:%s", svc1,
			shared.KubeConfigFile, svc2, port)

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
