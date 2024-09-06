package testcase

import (
	"sync"

	"github.com/rancher/distros-test-framework/pkg/aws"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestRebootInstances(cluster *shared.Cluster) {
	awsDependencies, err := aws.Add(cluster)
	Expect(err).NotTo(HaveOccurred())

	// reboot server instances.
	for _, IP := range cluster.ServerIPs {
		serverInstanceID, getErr := awsDependencies.GetInstanceIDByIP(IP)
		Expect(getErr).NotTo(HaveOccurred())
		rebootInstance(awsDependencies, serverInstanceID)
	}

	// reboot agent instances.
	for _, IP := range cluster.AgentIPs {
		agentInstanceID, getErr := awsDependencies.GetInstanceIDByIP(IP)
		Expect(getErr).NotTo(HaveOccurred())
		rebootInstance(awsDependencies, agentInstanceID)
	}
}

// rebootInstance reboots an instance by stopping and starting it.
func rebootInstance(awsDependencies *aws.Client, instanceID string) {
	var wg sync.WaitGroup

	wg.Add(1)
	go func(instanceID string) {
		defer wg.Done()
		defer GinkgoRecover()

		stopErr := awsDependencies.StopInstance(instanceID)
		if stopErr != nil {
			Expect(stopErr).NotTo(HaveOccurred())
		}

		startErr := awsDependencies.StartInstance(instanceID)
		if startErr != nil {
			Expect(startErr).NotTo(HaveOccurred())
		}
	}(instanceID)
	wg.Wait()
}
