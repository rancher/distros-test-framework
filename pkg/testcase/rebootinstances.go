package testcase

import (
	"sync"

	"github.com/rancher/distros-test-framework/pkg/aws"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestRebootInstances(cluster *shared.Cluster) {
	ec2Client, err := aws.AddClient(cluster)
	Expect(err).NotTo(HaveOccurred())

	// reboot server instances.
	for _, IP := range cluster.ServerIPs {
		serverInstanceID, getErr := ec2Client.GetInstanceIDByIP(IP)
		Expect(getErr).NotTo(HaveOccurred())
		rebootInstance(ec2Client, serverInstanceID)
	}

	// reboot agent instances.
	for _, IP := range cluster.AgentIPs {
		agentInstanceID, getErr := ec2Client.GetInstanceIDByIP(IP)
		Expect(getErr).NotTo(HaveOccurred())
		rebootInstance(ec2Client, agentInstanceID)
	}
}

// rebootInstance reboots an instance by stopping and starting it.
func rebootInstance(ec2Client *aws.Client, instanceID string) {
	var wg sync.WaitGroup

	wg.Add(1)
	go func(instanceID string) {
		defer wg.Done()
		defer GinkgoRecover()

		stopErr := ec2Client.StopInstance(instanceID)
		if stopErr != nil {
			Expect(stopErr).NotTo(HaveOccurred())
		}

		startErr := ec2Client.StartInstance(instanceID)
		if startErr != nil {
			Expect(startErr).NotTo(HaveOccurred())
		}
	}(instanceID)
	wg.Wait()
}
