package testcase

import (
	awsclient "github.com/rancher/distros-test-framework/pkg/aws"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/gomega"
)

func TestRebootInstances(cluster *shared.Cluster) {
	awsDependencies, err := awsclient.AddAWSClient(cluster)
	Expect(err).NotTo(HaveOccurred())

	var instanceIDs []string
	for _, IP := range cluster.ServerIPs {
		instanceID, getErr := awsDependencies.GetInstanceIDByIP(IP)
		Expect(getErr).NotTo(HaveOccurred())
		instanceIDs = append(instanceIDs, instanceID)
	}

	numWorkers := len(cluster.ServerIPs)
	jobs := make(chan string, len(instanceIDs))
	results := make(chan error, numWorkers)

	for i := 0; i < numWorkers; i++ {
		go func() {
			for instanceID := range jobs {

				stopErr := awsDependencies.StopInstance(instanceID)
				if stopErr != nil {
					results <- stopErr
					continue
				}

				startErr := awsDependencies.StartInstance(instanceID)
				if startErr != nil {
					results <- startErr
					continue
				}

				releaseEIPsErr := awsDependencies.ReleaseElasticIps(instanceID)
				if releaseEIPsErr != nil {
					results <- releaseEIPsErr
					continue
				}

				results <- nil
			}
		}()
	}

	for _, instanceID := range instanceIDs {
		jobs <- instanceID
	}
	close(jobs)

	for i := 0; i < len(instanceIDs); i++ {
		err := <-results
		Expect(err).NotTo(HaveOccurred())
	}
}

//
//
// var wg sync.WaitGroup
// for _, instanceID := range instanceIDs {
// 	instanceID := instanceID
// 	wg.Add(1)
// 	go func(instanceID string) {
// 		defer wg.Done()
// 		defer GinkgoRecover()
//
// 		stopErr := awsDependencies.StopInstance(instanceID)
// 		Expect(stopErr).NotTo(HaveOccurred())
// 	}(instanceID)
// }
// wg.Wait()
//
// for _, instanceID := range instanceIDs {
// 	instanceID := instanceID
// 	wg.Add(1)
// 	go func(instanceID string) {
// 		defer wg.Done()
// 		defer GinkgoRecover()
//
// 		startErr := awsDependencies.StartInstance(instanceID)
// 		Expect(startErr).NotTo(HaveOccurred())
// 	}(instanceID)
// }
// wg.Wait()
//
// for _, instanceID := range instanceIDs {
// 	instanceID := instanceID
// 	wg.Add(1)
// 	go func(instanceID string) {
// 		defer wg.Done()
// 		defer GinkgoRecover()
//
// 		releaseEIPsErr := awsDependencies.ReleaseElasticIps(instanceID)
// 		Expect(releaseEIPsErr).NotTo(HaveOccurred())
// 	}(instanceID)
// }
// wg.Wait()
