package mixedoscluster

import (
	"flag"
	"os"
	"testing"

	"github.com/rancher/distros-test-framework/factory"
	"github.com/rancher/distros-test-framework/pkg/customflag"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var cluster *factory.Cluster

func TestMain(m *testing.M) {
	flag.StringVar(&customflag.ServiceFlag.ExternalFlag.SonobuoyVersion, "sonobuoyVersion", "0.56.17", "Sonobuoy Version that will be executed on the cluster")
	flag.Var(&customflag.ServiceFlag.ClusterConfig.Destroy, "destroy", "Destroy cluster after test")
	flag.Parse()

	cluster = factory.ClusterConfig()

	if cluster.Config.Product == "k3s" {
		shared.LogLevel("error", "\nproduct not supported: %s", cluster.Config.Product)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

func TestMixedOSClusterCreateSuite(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Create Mixed OS Cluster Test Suite")
}

var _ = AfterSuite(func() {
	if customflag.ServiceFlag.ClusterConfig.Destroy {
		status, err := factory.DestroyCluster()
		Expect(err).NotTo(HaveOccurred())
		Expect(status).To(Equal("cluster destroyed"))
	}
})
