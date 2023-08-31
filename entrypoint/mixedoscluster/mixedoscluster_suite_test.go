package mixedoscluster

import (
	"flag"
	"os"
	"testing"

	"github.com/rancher/distros-test-framework/config"
	"github.com/rancher/distros-test-framework/factory"
	"github.com/rancher/distros-test-framework/pkg/customflag"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var sonobuoyVersion string
var cfg *config.ProductConfig

func TestMain(m *testing.M) {
	var err error
	flag.StringVar(&sonobuoyVersion, "sonobuoyVersion", "", "Sonobuoy Version that will be executed on the cluster")
	flag.Var(&customflag.ServiceFlag.ClusterConfig.Destroy, "destroy", "Destroy cluster after test")
	flag.Parse()

	cfg, err = config.AddConfigEnv("../../config")
	if err != nil {
		return
	}

	if cfg.Product == "k3s" {
		shared.LogLevel("error", "\nproduct not supported: %s", cfg.Product)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

func TestMixedOSClusterCreateSuite(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Create Mixed OS Cluster Test Suite")
}

var _ = AfterSuite(func() {
	g := GinkgoT()
	if customflag.ServiceFlag.ClusterConfig.Destroy {
		status, err := factory.DestroyCluster(g)
		Expect(err).NotTo(HaveOccurred())
		Expect(status).To(Equal("cluster destroyed"))
	}
})
