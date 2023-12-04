package restartservice

import (
	"os"
	"testing"

	"github.com/rancher/distros-test-framework/config"
	"github.com/rancher/distros-test-framework/factory"
	"github.com/rancher/distros-test-framework/pkg/customflag"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var cfg *config.ProductConfig

func TestMain(m *testing.M) {
	var err error

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
