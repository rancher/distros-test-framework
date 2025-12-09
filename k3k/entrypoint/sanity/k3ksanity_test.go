//go:build sanity

package sanity

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/distros-test-framework/internal/pkg/assert"
	"github.com/rancher/distros-test-framework/internal/pkg/testcase"
	"github.com/rancher/distros-test-framework/internal/provisioning/driver"
	"github.com/rancher/distros-test-framework/internal/resources"
	"github.com/rancher/distros-test-framework/k3k/testcases"
)

var _ = Describe("Test:", func() {
	It("Start Up with no issues", func() {
		testcase.TestBuildCluster(cluster)
	})

	It(fmt.Sprintf("TEST: Validate Nodes after host %s cluster deployment", cfg.Product), func() {
		testcase.TestNodeStatusForK3k(
			host,
			assert.NodeAssertReadyStatus(),
			nil,
			host.KubeconfigPath,
		)
	})

	It(fmt.Sprintf("TEST: Validate Pods after host %s cluster deployment:", cfg.Product), func() {
		testcase.TestPodStatusForK3k(
			host,
			assert.PodAssertRestart(),
			assert.PodAssertReady(),
			host.GetKubectlPath(),
			host.KubeconfigPath,
			"kube-system",
			"",
		)
	})

	It(fmt.Sprintf("Prepare setup with %s storage class and Test Install K3k and k3kcli", StorageClassType), func() {
		testcases.TestK3KInstall(host, StorageClassType, false, "", K3kNamespace)
	})

	if K3kTestCases == nil || len(K3kTestCases) == 0 {
		K3kTestCases = setupTestcases()
		resources.LogLevel("info", "Number of testcases created: %d", len(K3kTestCases))
	}

	for index, tc := range K3kTestCases {
		resources.LogLevel("info", "TESTCASE: %d with description: %s", index, tc.Description)
		tc := tc // Capture the loop variable for closure
		if tc.K3kCluster.Namespace == "" {
			tc.K3kCluster.Namespace = tc.K3kCluster.GetNamespace()
			resources.LogLevel("debug", "resetting namespace within testcase object: %s", tc.K3kCluster.Namespace)
		}
		It("TEST: Cluster create type: "+tc.Description, func() {
			tc.K3kCluster.SetKubeconfigPath(host)
			resources.LogLevel("debug", "Working with k3k Namespace: %s to create/delete clustername: %s ", tc.K3kCluster.Namespace, tc.K3kCluster.Name)

			createErr := testcases.TestK3KClusterCreate(tc, host)
			Expect(createErr).NotTo(HaveOccurred(), "create cluster failed: Name: %s Namespace: %s", tc.K3kCluster.Name, tc.K3kCluster.Namespace)
		})
		It("TEST: Validate Nodes for k3k cluster with host kubeconfig:", func() {
			testcase.TestNodeStatusForK3k(
				host,
				assert.NodeAssertReadyStatus(),
				nil,
				host.KubeconfigPath,
			)
		})
		It("TEST: Validate Pods for k3k cluster with host kubeconfig:", func() {
			testcase.TestPodStatusForK3k(
				host,
				assert.PodAssertRestart(),
				assert.PodAssertReady(),
				host.GetKubectlPath(),
				host.KubeconfigPath,
				tc.K3kCluster.Namespace,
				tc.K3kCluster.Name,
			)
		})

		// TODO: TestServerAgentCount, TestDeployment, TestStatefulSet, TestDaemonSet etc validations can be added here

		It("TEST: Validate Pods for k3k cluster with k3k cluster kubeconfig:", func() {
			testcase.TestPodStatusForK3k(
				host,
				assert.PodAssertRestart(),
				assert.PodAssertReady(),
				tc.K3kCluster.GetKubectlPath(host),
				tc.K3kCluster.GetKubeconfigPath(host),
				tc.K3kCluster.Namespace,
				tc.K3kCluster.Name,
			)
		})
		It("TEST: Cluster Delete type: "+tc.Description, func() {
			resources.LogLevel("debug", "Print all resources BEFORE cluster delete")
			resources.PrintGetAllForK3k(host, tc.K3kCluster.Namespace, tc.K3kCluster.GetKubectlPath(host))
			deleteErr := testcases.TestK3KClusterDelete(tc.K3kCluster, host)
			Expect(deleteErr).NotTo(HaveOccurred(), "delete cluster failed: Name: %s Namespace: %s", tc.K3kCluster.Name, tc.K3kCluster.Namespace)
		})
	}

})

var _ = AfterEach(func() {
	if CurrentSpecReport().Failed() {
		fmt.Printf("\nFAILED! %s\n\n", CurrentSpecReport().FullText())
	} else {
		fmt.Printf("\nPASSED! %s\n\n", CurrentSpecReport().FullText())
	}
})

// setupTestcases creates test cases for k3k clusters based on storage class type
// agents allowed only in virtual mode
// persistenceType can be 'ephemeral' or 'dynamic' - ephemeral set for local-path storageclass and dynamic for longhorn storageclass
func setupTestcases() []driver.K3kClusterOptions {
	resources.LogLevel("info", "Setting up testcases based on StorageClassType: %s", StorageClassType)
	switch StorageClassType {
	case "local-path":
		PersistenceType = "ephemeral"
		resources.LogLevel("info", "Setting up testcases for storageclass: %s with persistence type: %s", StorageClassType, PersistenceType)
		K3kTestCases = []driver.K3kClusterOptions{
			// Ephemeral persistence type test cases
			{Description: "shared + ephemeral + local-path + single server", Mode: "shared", StorageClassType: StorageClassType, PersistenceType: PersistenceType, NoOfServers: 1, NoOfAgents: 0, ServerArgs: "", ServiceCIDR: ServiceCIDR, K3SVersion: K3SVersion, UseValuesYAML: false, ValuesYAMLFile: "", K3kCluster: driver.K3kCluster{Namespace: K3kNamespace, Name: "ksh-fm-lpath", KubeconfigPath: ""}},
			{Description: "virtual + ephemeral + local-path + single server", Mode: "virtual", StorageClassType: StorageClassType, PersistenceType: PersistenceType, NoOfServers: 1, NoOfAgents: 0, ServerArgs: "", ServiceCIDR: ServiceCIDR, K3SVersion: K3SVersion, UseValuesYAML: false, ValuesYAMLFile: "", K3kCluster: driver.K3kCluster{Namespace: K3kNamespace, Name: "kvirt-fm-lpath", KubeconfigPath: ""}},
			{Description: "shared + ephemeral + local-path + multi server", Mode: "shared", StorageClassType: StorageClassType, PersistenceType: PersistenceType, NoOfServers: 3, NoOfAgents: 0, ServerArgs: "", ServiceCIDR: ServiceCIDR, K3SVersion: K3SVersion, UseValuesYAML: false, ValuesYAMLFile: "", K3kCluster: driver.K3kCluster{Namespace: K3kNamespace, Name: "ksh-fm-lpath-ms", KubeconfigPath: ""}},
			{Description: "virtual + ephemeral + local-path + multi server&agent", Mode: "virtual", StorageClassType: StorageClassType, PersistenceType: PersistenceType, NoOfServers: 3, NoOfAgents: 3, ServerArgs: "", ServiceCIDR: ServiceCIDR, K3SVersion: K3SVersion, UseValuesYAML: false, ValuesYAMLFile: "", K3kCluster: driver.K3kCluster{Namespace: K3kNamespace, Name: "kvirt-fm-lpath-ms", KubeconfigPath: ""}},
			{Description: "shared + ephemeral + local-path + yaml", Mode: "shared", StorageClassType: StorageClassType, PersistenceType: PersistenceType, NoOfServers: 1, NoOfAgents: 0, ServerArgs: "", ServiceCIDR: ServiceCIDR, K3SVersion: K3SVersion, UseValuesYAML: true, ValuesYAMLFile: "ephemeral-localpath-cluster.yaml", K3kCluster: driver.K3kCluster{Namespace: K3kNamespace, Name: "ksh-fm-lp-yaml", KubeconfigPath: ""}},
		}
	case "longhorn":
		PersistenceType = "dynamic"
		resources.LogLevel("info", "Setting up testcases for storageclass: %s with persistence type: %s", StorageClassType, PersistenceType)
		K3kTestCases = []driver.K3kClusterOptions{
			{Description: "shared + dynamic + longhorn + single server", Mode: "shared", StorageClassType: StorageClassType, PersistenceType: PersistenceType, NoOfServers: 1, NoOfAgents: 0, ServerArgs: "", ServiceCIDR: ServiceCIDR, K3SVersion: K3SVersion, UseValuesYAML: false, ValuesYAMLFile: "", K3kCluster: driver.K3kCluster{Namespace: K3kNamespace, Name: "ksh-dyn-lh", KubeconfigPath: ""}},
			{Description: "shared + dynamic + longhorn + multi server", Mode: "shared", StorageClassType: StorageClassType, PersistenceType: PersistenceType, NoOfServers: 3, NoOfAgents: 0, ServerArgs: "", ServiceCIDR: ServiceCIDR, K3SVersion: K3SVersion, UseValuesYAML: false, ValuesYAMLFile: "", K3kCluster: driver.K3kCluster{Namespace: K3kNamespace, Name: "ksh-dyn-lh-ms", KubeconfigPath: ""}},
			{Description: "virtual + dynamic + longhorn + single server", Mode: "virtual", StorageClassType: StorageClassType, PersistenceType: PersistenceType, NoOfServers: 1, NoOfAgents: 0, ServerArgs: "", ServiceCIDR: ServiceCIDR, K3SVersion: K3SVersion, UseValuesYAML: false, ValuesYAMLFile: "", K3kCluster: driver.K3kCluster{Namespace: K3kNamespace, Name: "kvirt-dyn-lh", KubeconfigPath: ""}},
			{Description: "virtual + dynamic + longhorn + multi-server-agent", Mode: "virtual", StorageClassType: StorageClassType, PersistenceType: PersistenceType, NoOfServers: 3, NoOfAgents: 3, ServerArgs: "", ServiceCIDR: ServiceCIDR, K3SVersion: K3SVersion, UseValuesYAML: false, ValuesYAMLFile: "", K3kCluster: driver.K3kCluster{Namespace: K3kNamespace, Name: "kvirt-dyn-lh-ms", KubeconfigPath: ""}},
			{Description: "shared + dynamic + longhorn + yaml", Mode: "shared", StorageClassType: StorageClassType, PersistenceType: PersistenceType, NoOfServers: 1, NoOfAgents: 0, ServerArgs: "", ServiceCIDR: ServiceCIDR, K3SVersion: K3SVersion, UseValuesYAML: true, ValuesYAMLFile: "dynamic-longhorn-cluster.yaml", K3kCluster: driver.K3kCluster{Namespace: K3kNamespace, Name: "ksh-dyn-lh-yaml", KubeconfigPath: ""}},
		}
	}
	return K3kTestCases
}
