package testcase

import (
	. "github.com/onsi/gomega"
	"github.com/rancher/distros-test-framework/pkg/aws"
	"github.com/rancher/distros-test-framework/pkg/testcase/support"
	"github.com/rancher/distros-test-framework/shared"
)

func TestIPv6Only(cluster *shared.Cluster, awsClient *aws.Client) {
	shared.LogLevel("info", "Setting up %s cluster on ipv6 only nodes...", cluster.Config.Product)
	err := support.ConfigureIPv6OnlyNodes(cluster, awsClient)
	Expect(err).NotTo(HaveOccurred(), err)

	shared.LogLevel("info", "Installing %s on ipv6 only nodes...", cluster.Config.Product)
	support.InstallOnIPv6Servers(cluster)
	support.InstallOnIPv6Agents(cluster)
}
