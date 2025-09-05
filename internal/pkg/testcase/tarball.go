package testcase

import (
	"github.com/rancher/distros-test-framework/internal/pkg/customflag"
	"github.com/rancher/distros-test-framework/internal/pkg/testcase/support"
	"github.com/rancher/distros-test-framework/internal/resources"

	. "github.com/onsi/gomega"
)

func TestTarball(cluster *resources.Cluster, flags *customflag.FlagConfig) {
	resources.LogLevel("info", "Downloading tarball artifacts...")
	_, err := support.GetArtifacts(cluster, "linux", flags.AirgapFlag.ImageRegistryUrl, flags.AirgapFlag.TarballType)
	Expect(err).To(BeNil(), err)

	resources.LogLevel("info", "Copying assets on the airgap nodes...")
	err = support.CopyAssetsOnNodes(cluster, support.Tarball, &flags.AirgapFlag.TarballType)
	Expect(err).To(BeNil(), err)

	resources.LogLevel("info", "Installing %v on airgap nodes...", cluster.Config.Product)
	support.InstallOnAirgapServers(cluster, support.Tarball)
	resources.LogLevel("info", "Installation of %v on airgap servers: Completed!", cluster.Config.Product)
	support.InstallOnAirgapAgents(cluster, support.Tarball)
	resources.LogLevel("info", "Installation of %v on airgap agents: Completed!", cluster.Config.Product)

	if support.HasWindowsAgent(cluster) {
		resources.LogLevel("info", "Downloading %v artifacts for Windows...", cluster.Config.Product)
		_, err = support.GetArtifacts(cluster, "windows", flags.AirgapFlag.ImageRegistryUrl, flags.AirgapFlag.TarballType)
		Expect(err).To(BeNil(), err)

		resources.LogLevel("info", "Copy assets on Windows airgap nodes...")
		err = support.CopyAssetsOnNodesWindows(cluster, support.Tarball)
		Expect(err).To(BeNil(), err)

		support.InstallOnAirgapAgentsWindows(cluster, support.Tarball)
		resources.LogLevel("info", "%v install on airgap Windows agents: Completed!", cluster.Config.Product)
	}
}
