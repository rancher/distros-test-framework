package aws

import (
	"sync"

	"github.com/rancher/distros-test-framework/shared"
)

func DeleteEC2Instances(cluster *shared.Cluster) {
	ips := shared.FetchNodeExternalIPs()
	awsClient, err := AddClient(cluster)
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
