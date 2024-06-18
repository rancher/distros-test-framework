package template

import (
	"fmt"
	"strings"

	"github.com/rancher/distros-test-framework/factory"
	"github.com/rancher/distros-test-framework/pkg/assert"
	"github.com/rancher/distros-test-framework/pkg/testcase"
	"github.com/rancher/distros-test-framework/shared"
)

// upgradeVersion upgrades the product version
func upgradeVersion(template TestTemplate, version string) error {
	cluster := factory.ClusterConfig()
	err := testcase.TestUpgradeClusterManually(cluster, version)
	if err != nil {
		return err
	}

	updateExpectedValue(template)

	return nil
}

// updateExpectedValue updates the expected values getting the values from flag ExpectedValueUpgrade
func updateExpectedValue(template TestTemplate) {
	for i := range template.TestCombination.Run {
		template.TestCombination.Run[i].ExpectedValue = template.TestCombination.Run[i].ExpectedValueUpgrade
	}
}

// executeTestCombination get a template and pass it to `processTestCombination`
//
// to execute test combination on group of IPs
func executeTestCombination(template TestTemplate) error {
	currentVersion, err := currentProductVersion()
	if err != nil {
		return shared.ReturnLogError("failed to get current version: %w", err)
	}

	ips := shared.FetchNodeExternalIPs()
	processErr := processTestCombination(ips, currentVersion, &template)
	if processErr != nil {
		return shared.ReturnLogError("failed to process test combination: %w", processErr)
	}

	if template.TestConfig != nil {
		testCaseWrapper(template)
	}

	return nil
}

// AddTestCases returns the test case based on the name to be used as customflag.
func AddTestCases(cluster *factory.Cluster, names []string) ([]testCase, error) {
	var testCases []testCase

	tcs := map[string]testCase{
		"TestDaemonset":        testcase.TestDaemonset,
		"TestIngress":          testcase.TestIngress,
		"TestDnsAccess":        testcase.TestDnsAccess,
		"TestServiceClusterIP": testcase.TestServiceClusterIp,
		"TestServiceNodePort":  testcase.TestServiceNodePort,
		"TestLocalPathProvisionerStorage": func(applyWorkload, deleteWorkload bool) {
			testcase.TestLocalPathProvisionerStorage(cluster, applyWorkload, deleteWorkload)
		},
		"TestServiceLoadBalancer": testcase.TestServiceLoadBalancer,
		"TestInternodeConnectivityMixedOS": func(applyWorkload, deleteWorkload bool) {
			testcase.TestInternodeConnectivityMixedOS(cluster, applyWorkload, deleteWorkload)
		},
		"TestSonobuoyMixedOS": func(_, deleteWorkload bool) {
			testcase.TestSonobuoyMixedOS(deleteWorkload)
		},
		"TestSelinuxEnabled": func(_, _ bool) {
			testcase.TestSelinux(cluster)
		},
		"TestSelinux": func(_, _ bool) {
			testcase.TestSelinux(cluster)
		},
		"TestSelinuxSpcT": func(_, _ bool) {
			testcase.TestSelinuxSpcT(cluster)
		},
		"TestUninstallPolicy": func(_, _ bool) {
			testcase.TestUninstallPolicy(cluster)
		},
		"TestSelinuxContext": func(_, _ bool) {
			testcase.TestSelinuxContext(cluster)
		},
		"TestIngressRoute": func(applyWorkload, deleteWorkload bool) {
			testcase.TestIngressRoute(cluster, applyWorkload, deleteWorkload, "traefik.io/v1alpha1")
		},
		"TestCertRotate": func(_, _ bool) {
			testcase.TestCertRotate(cluster)
		},
		"TestSecretsEncryption": func(_, _ bool) {
			testcase.TestSecretsEncryption()
		},
		"TestRestartService": func(_, _ bool) {
			testcase.TestRestartService(cluster)
		},
		"TestClusterReset": func(_, _ bool) {
			testcase.TestClusterReset(cluster)
		},
	}

	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			testCases = append(testCases, func(_, _ bool) {})
		} else if test, ok := tcs[name]; ok {
			testCases = append(testCases, test)
		} else {
			return nil, shared.ReturnLogError("invalid test case name")
		}
	}

	return testCases, nil
}

func currentProductVersion() (string, error) {
	_, version, err := shared.Product()
	if err != nil {
		return "", shared.ReturnLogError("failed to get product: %w", err)
	}

	return version, nil
}

func ComponentsBumpResults() {
	product, version, err := shared.Product()
	if err != nil {
		return
	}

	var components []string
	for _, result := range assert.Results {
		if product == "rke2" {
			components = []string{"flannel", "calico", "ingressController", "coredns", "metricsServer", "etcd",
				"containerd", "runc"}
		} else {
			components = []string{"flannel", "coredns", "metricsServer", "etcd", "cniPlugins", "traefik", "local-path",
				"containerd", "klipper", "runc"}
		}
		for _, component := range components {
			if strings.Contains(result.Command, component) {
				fmt.Printf("\n---------------------\nResults from %s on version: %s\n``` \n%v\n ```\n---------------------"+
					"\n\n\n", component, version, result)
			}
		}
		fmt.Printf("\n---------------------\nResults from %s\n``` \n%v\n ```\n---------------------\n\n\n",
			result.Command, result)
	}
}
