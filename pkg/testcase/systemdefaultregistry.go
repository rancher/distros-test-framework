package testcase

import (
	"github.com/rancher/distros-test-framework/pkg/customflag"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/gomega"
)

func TestSystemDefaultRegistry(cluster *shared.Cluster, flags *customflag.FlagConfig) {
	shared.LogLevel("info", "Setting bastion as system default registry...")
	err := shared.SetupAirgapRegistry(cluster, flags, SystemDefaultRegistry)
	Expect(err).To(BeNil(), err)

	shared.LogLevel("info", "Copying assets on the airgap nodes...")
	err = shared.CopyAssetsOnNodes(cluster, SystemDefaultRegistry, nil)
	Expect(err).To(BeNil(), err)

	shared.LogLevel("info", "Installing %v on airgap nodes...", cluster.Config.Product)
	installOnServers(cluster, SystemDefaultRegistry)
	shared.LogLevel("info", "Installation of %v on airgap servers: Completed!", cluster.Config.Product)
	installOnAgents(cluster, SystemDefaultRegistry)
	shared.LogLevel("info", "Installation of %v on airgap agents: Completed!", cluster.Config.Product)
}
