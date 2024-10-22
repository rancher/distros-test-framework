package testcase

import (
	"github.com/rancher/distros-test-framework/pkg/customflag"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/gomega"
)

func TestTarball(cluster *shared.Cluster, flags *customflag.FlagConfig) {
	shared.LogLevel("info", "Downloading tarball artifacts...")
	_, err := shared.GetArtifacts(cluster, flags.AirgapFlag.TarballType)
	Expect(err).To(BeNil(), err)

	shared.LogLevel("info", "Copying assets on the airgap nodes...")
	err = shared.CopyAssetsOnNodes(cluster, Tarball, &flags.AirgapFlag.TarballType)
	Expect(err).To(BeNil(), err)

	shared.LogLevel("info", "Installing %v on airgap nodes...", cluster.Config.Product)
	installOnServers(cluster)
	installOnAgents(cluster)
}
