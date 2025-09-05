package support

import (
	"sync"

	"github.com/rancher/distros-test-framework/internal/resources"

	"github.com/rancher/distros-test-framework/internal/pkg/aws"
)

// DeleteEC2Nodes Deletes all the nodes on the cluster based on externalIPs.
func DeleteEC2Nodes(cluster *resources.Cluster) {
	ips := resources.FetchNodeExternalIPs()
	awsClient, err := aws.AddClient(cluster)
	if err != nil {
		resources.LogLevel("error", "error creating aws client: %w\n", err)
	}
	var wg sync.WaitGroup
	for _, ip := range ips {
		wg.Add(1)
		go func(ip string) {
			defer wg.Done()
			nodeDelErr := awsClient.DeleteInstance(ip)
			if nodeDelErr != nil {
				resources.LogLevel("error", "on deleting node with ip: %v, got error %w", ip, nodeDelErr)
				return
			}
		}(ip)
	}
	wg.Wait()
}
