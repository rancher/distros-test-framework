package restartservice

import (
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

	cfg, err = shared.EnvConfig()
	if err != nil {
		return
	}

	os.Exit(m.Run())
}

func TestRestartServiceSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Restart Service Test Suite")
}

var _ = AfterSuite(func() {
	g := GinkgoT()
	if customflag.ServiceFlag.ClusterConfig.Destroy {
		status, err := build.DestroyCluster(g)
		Expect(err).NotTo(HaveOccurred())
		Expect(status).To(Equal("cluster destroyed"))
	}
})
