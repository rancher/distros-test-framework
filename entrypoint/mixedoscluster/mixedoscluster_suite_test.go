package mixedoscluster

import (
	"flag"
	"os"
	"testing"

	"github.com/rancher/distros-test-framework/config"
	"github.com/rancher/distros-test-framework/factory"
	"github.com/rancher/distros-test-framework/pkg/productflag"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var cfg *config.Product

func TestMain(m *testing.M) {
	flag.Var(&productflag.ServiceFlag.Destroy, "destroy", "Destroy cluster after test")
	flag.Parse()

	var err error
	cfg, err = shared.EnvConfig()
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
	if productflag.ServiceFlag.Destroy {
		status, err := factory.DestroyCluster(g)
		Expect(err).NotTo(HaveOccurred())
		Expect(status).To(Equal("cluster destroyed"))
	}
})
