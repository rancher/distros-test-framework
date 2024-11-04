//go:build upgradereplacement

package upgradecluster

import (
	"fmt"
	"sync"

	"github.com/rancher/distros-test-framework/pkg/assert"
	"github.com/rancher/distros-test-framework/pkg/aws"
	"github.com/rancher/distros-test-framework/pkg/testcase"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Upgrade Node Replacement Test:", Ordered, func() {
	It("Start Up with no issues", func() {
		testcase.TestBuildCluster(cluster)
	})

	It("Validate Node", func() {
		testcase.TestNodeStatus(
			cluster,
			assert.NodeAssertReadyStatus(),
			nil)
	})

	It("Validate Pod", func() {
		testcase.TestPodStatus(
			cluster,
			assert.PodAssertRestart(),
			assert.PodAssertReady())
	})

	It("Verifies ClusterIP Service pre-upgrade", func() {
		testcase.TestServiceClusterIP(true, false)
	})

	if cluster.Config.Product == "k3s" {
		It("Verifies LoadBalancer Service pre-upgrade", func() {
			testcase.TestServiceLoadBalancer(true, false)
		})
	}

	It("Verifies Ingress pre-upgrade", func() {
		testcase.TestIngress(true, false)
	})

	It("Upgrade by Node replacement", func() {
		testcase.TestUpgradeReplaceNode(cluster, flags)
	})

	It("Checks Node Status after upgrade and validate version", func() {
		testcase.TestNodeStatus(
			cluster,
			assert.NodeAssertReadyStatus(),
			assert.NodeAssertVersionTypeUpgrade(flags))
	})

	It("Checks Pod Status after upgrade", func() {
		testcase.TestPodStatus(
			cluster,
			assert.PodAssertRestart(),
			assert.PodAssertReady())
	})

	It("Verifies ClusterIP Service after upgrade", func() {
		testcase.TestServiceClusterIP(false, true)
	})

	It("Verifies NodePort Service after upgrade applying and deleting workload", func() {
		testcase.TestServiceNodePort(true, true)
	})

	It("Verifies Ingress after upgrade", func() {
		testcase.TestIngress(false, true)
	})

	if cluster.Config.Product == "k3s" {
		It("Verifies LoadBalancer Service after upgrade", func() {
			testcase.TestServiceLoadBalancer(false, true)
		})
	}

	AfterAll(func() {
		if flags.Destroy {
			deleteAWSResources()
		}
	})

})

func deleteAWSResources() {
	ips := shared.FetchNodeExternalIPs()
	awsClient, err = aws.AddClient(cluster)
	if err != nil {
		shared.LogLevel("error", "error creating aws client: %w\n", err)
	}
	var wg sync.WaitGroup
	for _, ip := range ips {
		wg.Add(1)
		go func(ip string) {
			defer wg.Done()
			nodeDelErr := awsClient.DeleteInstance(ip)
			if nodeDelErr != nil {
				shared.LogLevel("error", "on deleting node with ip: %v, got error %w", ip, nodeDelErr)
				return
			}
		}(ip)
	}
	wg.Wait()
}

var _ = AfterEach(func() {
	if CurrentSpecReport().Failed() {
		fmt.Printf("\nFAILED! %s\n\n", CurrentSpecReport().FullText())
	} else {
		fmt.Printf("\nPASSED! %s\n\n", CurrentSpecReport().FullText())
	}
})
