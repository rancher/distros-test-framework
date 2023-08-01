package createcluster

import (
	"flag"
	"fmt"
	"os"
	"testing"

	"github.com/rancher/distros-test-framework/config"
	"github.com/rancher/distros-test-framework/factory"
	"github.com/rancher/distros-test-framework/pkg/customflag"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var cfg config.ProductConfig

func TestMain(m *testing.M) {
	var err error

	flag.Var(&customflag.ServiceFlag.ClusterConfig.Destroy, "destroy", "Destroy cluster after test")
	flag.Var(&customflag.ServiceFlag.ClusterConfig.Arch, "arch", "Architecture type")
	flag.Parse()

	cfg, err = config.LoadConfigEnv("../../config")
	if err != nil {
		fmt.Println(err)
		return
	}

	os.Exit(m.Run())
}

func TestClusterCreateSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Create Cluster Test Suite")
}

var _ = AfterSuite(func() {
	g := GinkgoT()
	if customflag.ServiceFlag.ClusterConfig.Destroy {
		status, err := factory.DestroyCluster(g)
		Expect(err).NotTo(HaveOccurred())
		Expect(status).To(Equal("cluster destroyed"))
	}
})
