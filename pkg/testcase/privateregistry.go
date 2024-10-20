package testcase

import (
	"github.com/rancher/distros-test-framework/pkg/customflag"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/gomega"
)

func TestPrivateRegistry(cluster *shared.Cluster, flags *customflag.FlagConfig) {
	shared.LogLevel("info", "Setting bastion as private registry...")
	err := shared.SetupAirgapRegistry(cluster, flags, PrivateRegistry)
	Expect(err).To(BeNil(), err)

	shared.LogLevel("info", "Updating and copying registries.yaml on bastion...")
	err = shared.UpdateRegistryFile(cluster, flags)
	Expect(err).To(BeNil(), err)

	shared.LogLevel("info", "Copying assets on the airgap nodes...")
	err = shared.CopyAssetsOnNodes(cluster, PrivateRegistry)
	Expect(err).To(BeNil(), err)

	shared.LogLevel("info", "Installing %v on airgap nodes...", cluster.Config.Product)
	installOnServers(cluster)
	installOnAgents(cluster)
}
