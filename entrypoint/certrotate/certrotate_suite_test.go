package certrotate

import (
	"flag"
	"os"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rancher/distros-test-framework/config"
	"github.com/rancher/distros-test-framework/factory"
	"github.com/rancher/distros-test-framework/pkg/customflag"
	"github.com/rancher/distros-test-framework/shared"
)

var cfg *config.ProductConfig

func TestMain(m *testing.M) {
	var err error
	flag.Var(&customflag.ServiceFlag.ClusterConfig.Destroy, "destroy", "Destroy cluster after test")
	flag.Parse()

	configPath, err := shared.EnvDir("entrypoint")
	if err != nil {
		return
	}
	cfg, err = config.AddConfigEnv(configPath)
	if err != nil {
		return
	}

	os.Exit(m.Run())
}

func TestCertRotateSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Certificate Rotate Test Suite")
}

var _ = AfterSuite(func() {
	g := GinkgoT()
	if customflag.ServiceFlag.ClusterConfig.Destroy {
		status, err := factory.DestroyCluster(g)
		Expect(err).NotTo(HaveOccurred())
		Expect(status).To(Equal("cluster destroyed"))
	}
})
