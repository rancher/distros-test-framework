package testcase

import (
	"github.com/rancher/distros-test-framework/internal/pkg/customflag"
	"github.com/rancher/distros-test-framework/internal/pkg/testcase/support"
	"github.com/rancher/distros-test-framework/internal/resources"

	. "github.com/onsi/gomega"
)

func TestSystemDefaultRegistry(cluster *resources.Cluster, flags *customflag.FlagConfig) {
	resources.LogLevel("info", "Setting bastion as system default registry...")
	err := support.SetupAirgapRegistry(cluster, flags, support.SystemDefaultRegistry)
	Expect(err).To(BeNil(), err)

	resources.LogLevel("info", "Copying assets on the airgap nodes...")
	err = support.CopyAssetsOnNodes(cluster, support.SystemDefaultRegistry, nil)
	Expect(err).To(BeNil(), err)

	resources.LogLevel("info", "Installing %v on airgap nodes...", cluster.Config.Product)
	support.InstallOnAirgapServers(cluster, support.SystemDefaultRegistry)
	resources.LogLevel("info", "Installation of %v on airgap servers: Completed!", cluster.Config.Product)
	support.InstallOnAirgapAgents(cluster, support.SystemDefaultRegistry)
	resources.LogLevel("info", "Installation of %v on airgap agents: Completed!", cluster.Config.Product)
}
