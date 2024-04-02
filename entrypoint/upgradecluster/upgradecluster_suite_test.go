package upgradecluster

import (
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
	productflag.AddFlags("installVersionOrCommit", "channel", "destroy", "sucUpgradeVersion")

	var err error
	cfg, err = shared.EnvConfig()
	if err != nil {
		return
	}

	os.Exit(m.Run())
}

func TestClusterUpgradeSuite(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Upgrade Cluster Test Suite")
}

var _ = AfterSuite(func() {
	g := GinkgoT()
	if productflag.ServiceFlag.Destroy {
		status, err := factory.DestroyCluster(g)
		Expect(err).NotTo(HaveOccurred())
		Expect(status).To(Equal("cluster destroyed"))
	}
})
