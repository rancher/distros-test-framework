package validatecluster

import (
	"flag"
	"os"
	"testing"

	"github.com/rancher/distros-test-framework/build"
	"github.com/rancher/distros-test-framework/config"
	"github.com/rancher/distros-test-framework/pkg/customflag"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var cfg *config.Product

func TestMain(m *testing.M) {
	var err error
	flag.Var(&customflag.ServiceFlag.ClusterConfig.Destroy, "destroy", "Destroy cluster after test")
	flag.Parse()

	cfg, err = shared.EnvConfig()
	if err != nil {
		return
	}

	os.Exit(m.Run())
}

func TestValidateClusterSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Create Cluster Test Suite")
}

var _ = AfterSuite(func() {
	g := GinkgoT()
	if customflag.ServiceFlag.ClusterConfig.Destroy {
		status, err := build.DestroyCluster(g)
		Expect(err).NotTo(HaveOccurred())
		Expect(status).To(Equal("cluster destroyed"))
	}
})
