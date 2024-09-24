package airgap

import (
	"flag"
	"os"
	"testing"

	"github.com/rancher/distros-test-framework/config"
	"github.com/rancher/distros-test-framework/pkg/customflag"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	flags   *customflag.FlagConfig
	cluster *shared.Cluster
)

func TestMain(m *testing.M) {
	flags = &customflag.ServiceFlag
	flag.Var(&flags.Destroy, "destroy", "Destroy cluster after test")
	flag.StringVar(&flags.AirgapFlag.RegistryUsername, "registryUsername", "testuser", "private registry username")
	flag.StringVar(&flags.AirgapFlag.RegistryPassword, "registryPassword", "testpass765", "private registry password")
	flag.StringVar(&flags.AirgapFlag.TarballType, "tarballType", "", "artifact tarball type")
	flag.Parse()

	_, err := config.AddEnv()
	if err != nil {
		shared.LogLevel("error", "error adding env vars: %w\n", err)
		os.Exit(1)
	}

	// TODO: Implement using kubeconfig for airgap setup
	cluster = shared.ClusterConfig()

	os.Exit(m.Run())
}

func TestAirgapSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Create Airgap Cluster Test Suite")
}

var _ = AfterSuite(func() {
	if customflag.ServiceFlag.Destroy {
		status, err := shared.DestroyCluster()
		Expect(err).NotTo(HaveOccurred())
		Expect(status).To(Equal("cluster destroyed"))
	}
})
