package testcase

import (
	"github.com/rancher/distros-test-framework/pkg/customflag"
	"github.com/rancher/distros-test-framework/pkg/testcase/support"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/gomega"
)

func TestPrivateRegistry(cluster *shared.Cluster, flags *customflag.FlagConfig) {
	shared.LogLevel("info", "Set bastion for private registry...")
	err := support.SetupAirgapRegistry(cluster, flags, support.PrivateRegistry)
	Expect(err).To(BeNil(), err)

	shared.LogLevel("info", "Update and copy registries.yaml on bastion...")
	err = support.UpdateRegistryFile(cluster, flags)
	Expect(err).To(BeNil(), err)

	shared.LogLevel("info", "Copy assets on airgap nodes...")
	err = support.CopyAssetsOnNodes(cluster, support.PrivateRegistry, nil)
	Expect(err).To(BeNil(), err)

	shared.LogLevel("info", "Install %v on airgap nodes...", cluster.Config.Product)
	support.InstallOnAirgapServers(cluster, support.PrivateRegistry)
	shared.LogLevel("info", "%v install on airgap servers: Completed!", cluster.Config.Product)

	support.InstallOnAirgapAgents(cluster, support.PrivateRegistry)
	shared.LogLevel("info", "%v install on airgap agents: Completed!", cluster.Config.Product)

	if support.HasWindowsAgent(cluster) {
		shared.LogLevel("info", "Configure registry for Windows...")
		err = support.ConfigureRegistryWindows(cluster, flags)
		Expect(err).To(BeNil(), err)

		shared.LogLevel("info", "Update and copy registries.yaml for Windows on bastion...")
		err = support.UpdateRegistryFileWindows(cluster, flags)
		Expect(err).To(BeNil(), err)

		shared.LogLevel("info", "Copy assets on Windows airgap nodes...")
		err = support.CopyAssetsOnNodesWindows(cluster, support.PrivateRegistry)
		Expect(err).To(BeNil(), err)

		support.InstallOnAirgapAgentsWindows(cluster, support.PrivateRegistry)
		shared.LogLevel("info", "%v install on airgap Windows agents: Completed!", cluster.Config.Product)
	}
}
