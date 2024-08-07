package testcase

import (
	"fmt"
	"os"

	. "github.com/onsi/gomega"
	awsClient "github.com/rancher/distros-test-framework/pkg/aws"
	"github.com/rancher/distros-test-framework/shared"
)

func TestRebootInstances(cluster *shared.Cluster) {
	awsDependencies, err := awsClient.AddAWSClient(cluster)
	Expect(err).NotTo(HaveOccurred())

	var instanceIDs []string
	for _, IP := range cluster.ServerIPs {
		instanceID, err := awsDependencies.GetInstanceIDByIP(IP)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		instanceIDs = append(instanceIDs, instanceID)
	}

	Eventually(func(g Gomega) bool {
		for _, instanceID := range instanceIDs {
			instanceState, err := awsDependencies.StopInstance(instanceID)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(instanceState).To(ContainSubstring("stopped"))
		}

		return true
	}, "2500s", "10s").Should(BeTrue(), func() string {
		shared.LogLevel("error", "\nError stopping instance\n")
		return "Instances could not be stopped"
	})
	Eventually(func(g Gomega) bool {
		for _, instanceID := range instanceIDs {
			instanceState, err := awsDependencies.StartInstance(instanceID)
			fmt.Println(instanceID, instanceState)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(instanceState).To(ContainSubstring("running"))
		}

		return true
	}, "2500s", "10s").Should(BeTrue(), func() string {
		shared.LogLevel("error", "\nError starting instance\n")
		return "Instances could not be started"
	})
}
