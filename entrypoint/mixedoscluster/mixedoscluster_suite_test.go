package mixedoscluster

import (
	"flag"
	"fmt"
	"os"
	"testing"

	"github.com/rancher/distros-test-framework/config"
	"github.com/rancher/distros-test-framework/pkg/customflag"
	"github.com/rancher/distros-test-framework/factory"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var sonobuoyVersion string
var cfg config.ProductConfig

func TestMain(m *testing.M) {
	var err error
	flag.StringVar(&sonobuoyVersion, "sonobuoyVersion", "", "Sonobuoy Version that will be executed on the cluster")
	flag.Var(&customflag.ServiceFlag.ClusterConfig.Destroy, "destroy", "Destroy cluster after test")
	flag.Parse()

	cfg, err = config.LoadConfigEnv("../../config")
	if err != nil {
		fmt.Println(err)
		return
	}

	if cfg.Product == "rke2" {
		os.Exit(m.Run())
	} else {
		fmt.Println("Unsupported product to execute tests: " + cfg.Product)
		os.Exit(1)
	}
	
}

func TestMixedOSClusterCreateSuite(t *testing.T) {
	RegisterFailHandler(Fail)

	if cfg.Product == "rke2" {
		RunSpecs(t, "Create Mixed OS Cluster Test Suite")
	} else {
		fmt.Println("Unsupported product to execute test suite", cfg.Product)
	}
	
}

var _ = AfterSuite(func() {
	g := GinkgoT()
	if customflag.ServiceFlag.ClusterConfig.Destroy {
		status, err := factory.DestroyCluster(g)
		Expect(err).NotTo(HaveOccurred())
		Expect(status).To(Equal("cluster destroyed"))
	}
})
