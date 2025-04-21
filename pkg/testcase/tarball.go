package testcase

import (
	"github.com/rancher/distros-test-framework/pkg/customflag"
	"github.com/rancher/distros-test-framework/pkg/testcase/support"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/gomega"
)

func TestTarball(cluster *shared.Cluster, flags *customflag.FlagConfig) {
	shared.LogLevel("info", "Downloading tarball artifacts...")
	_, err := support.GetArtifacts(cluster, "linux", flags.AirgapFlag.ImageRegistryUrl, flags.AirgapFlag.TarballType)
	Expect(err).To(BeNil(), err)

	shared.LogLevel("info", "Copying assets on the airgap nodes...")
	err = support.CopyAssetsOnNodes(cluster, support.Tarball, &flags.AirgapFlag.TarballType)
	Expect(err).To(BeNil(), err)

	shared.LogLevel("info", "Installing %v on airgap nodes...", cluster.Config.Product)
	support.InstallOnAirgapServers(cluster, support.Tarball)
	shared.LogLevel("info", "Installation of %v on airgap servers: Completed!", cluster.Config.Product)
	support.InstallOnAirgapAgents(cluster, support.Tarball)
	shared.LogLevel("info", "Installation of %v on airgap agents: Completed!", cluster.Config.Product)
}
