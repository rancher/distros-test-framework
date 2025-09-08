package testcase

import (
	. "github.com/onsi/gomega"
	"github.com/rancher/distros-test-framework/internal/pkg/aws"
	"github.com/rancher/distros-test-framework/internal/pkg/testcase/support"
	"github.com/rancher/distros-test-framework/internal/provisioning/driver"
	"github.com/rancher/distros-test-framework/internal/resources"
)

func TestIPv6Only(cluster *driver.Cluster, awsClient *aws.Client) {
	resources.LogLevel("info", "Setting up %s cluster on ipv6 only nodes...", cluster.Config.Product)
	err := support.ConfigureIPv6OnlyNodes(cluster, awsClient)
	Expect(err).NotTo(HaveOccurred(), err)

	resources.LogLevel("info", "Installing %s on ipv6 only server nodes...", cluster.Config.Product)
	support.InstallOnIPv6Servers(cluster)
	resources.LogLevel("info", "Installing %s on ipv6 only agent nodes...", cluster.Config.Product)
	support.InstallOnIPv6Agents(cluster)
	resources.LogLevel("info", "Installation of %s on ipv6 only nodes: Completed!", cluster.Config.Product)
}
