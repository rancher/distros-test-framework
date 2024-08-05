package airgap

import (
	"flag"
	"os"
	"testing"

	"github.com/rancher/distros-test-framework/factory"
	"github.com/rancher/distros-test-framework/pkg/customflag"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	cluster *factory.Cluster
	flags   *customflag.FlagConfig
)

func TestMain(m *testing.M) {
	flags = &customflag.ServiceFlag
	flag.Var(&flags.Destroy, "destroy", "Destroy cluster after test")
	flag.StringVar(&flags.RegistryFlag.RegistryUsername, "registryUsername", "", "private registry username")
	flag.StringVar(&flags.RegistryFlag.RegistryPassword, "registryPassword", "", "private registry password")
	flag.Parse()

	cluster = factory.ClusterConfig()

	os.Exit(m.Run())
}

func TestAirgapSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Create Airgap Cluster Test Suite")
}

var _ = AfterSuite(func() {
	if customflag.ServiceFlag.Destroy {
		status, err := factory.DestroyCluster()
		Expect(err).NotTo(HaveOccurred())
		Expect(status).To(Equal("cluster destroyed"))
	}
})
