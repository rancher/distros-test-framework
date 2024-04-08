package restartservice

import (
	"os"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rancher/distros-test-framework/config"
	"github.com/rancher/distros-test-framework/factory"
	"github.com/rancher/distros-test-framework/pkg/customflag"
)

var (
	cfg     *config.Product
	cluster *factory.Cluster
)

func TestMain(m *testing.M) {
	var err error

	cfg, err = config.AddEnv()
	if err != nil {
		return
	}

	cluster = factory.ClusterConfig(GinkgoT())

	os.Exit(m.Run())
}

func TestRestartServiceSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Restart Service Test Suite")
}

var _ = AfterSuite(func() {
	g := GinkgoT()
	if customflag.ServiceFlag.ClusterConfig.Destroy {
		status, err := factory.DestroyCluster(g)
		Expect(err).NotTo(HaveOccurred())
		Expect(status).To(Equal("cluster destroyed"))
	}
})
