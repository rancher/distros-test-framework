package support

import (
	"sync"

	"github.com/rancher/distros-test-framework/internal/pkg/aws"
	"github.com/rancher/distros-test-framework/internal/provisioning/driver"
	"github.com/rancher/distros-test-framework/internal/resources"
)

// DeleteEC2Nodes Deletes all the nodes on the cluster based on externalIPs.
func DeleteEC2Nodes(cluster *driver.Cluster) {
	awsClient, err := aws.AddClient(cluster)
	if err != nil {
		resources.LogLevel("error", "error creating aws client: %v", err)
		return
	}

	// A failing kubectl must not skip the tracked-ID cleanup below.
	ips, ipsErr := resources.FetchNodeExternalIPs()
	if ipsErr != nil {
		resources.LogLevel("error", "skipping IP-based node cleanup: %v", ipsErr)
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

	// Replacement nodes live outside the Tofu state and may lack a kubectl
	// ExternalIP — terminate them by the exact IDs recorded at creation.
	if ids := aws.TrackedInstanceIDs(); len(ids) > 0 {
		resources.LogLevel("info", "terminating instances created outside Tofu: %v", ids)
		if delErr := awsClient.DeleteInstances(ids...); delErr != nil {
			resources.LogLevel("error", "error terminating tracked instances: %v", delErr)
		}
	}
}
