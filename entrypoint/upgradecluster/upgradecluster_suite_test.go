package upgradecluster

import (
	"flag"
	"os"
	"testing"

	"github.com/rancher/distros-test-framework/config"
	"github.com/rancher/distros-test-framework/factory"
	"github.com/rancher/distros-test-framework/pkg/customflag"
	"github.com/rancher/distros-test-framework/pkg/template"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var cfg *config.ProductConfig

func TestMain(m *testing.M) {
	var err error
	flag.Var(&customflag.ServiceFlag.InstallMode, "installVersionOrCommit", "Upgrade with version or commit")
	flag.Var(&customflag.ServiceFlag.Channel, "channel", "channel to use on install or upgrade")
	flag.Var(&customflag.ServiceFlag.ClusterConfig.Destroy, "destroy", "Destroy cluster after test")
	flag.Var(&customflag.ServiceFlag.SUCUpgradeVersion, "sucUpgradeVersion", "Version for upgrading using SUC")

	flag.Parse()

	cfg, err = config.AddConfigEnv("../../config/.env")
	if err != nil {
		return
	}

	if customflag.ServiceFlag.InstallMode.String() != "" && template.TestMapTemplate.ExpectedValueUpgrade == "" {
		shared.LogLevel("error", "if you are using upgrade, please provide the expected value after upgrade")
		os.Exit(1)
	}

	os.Exit(m.Run())
}

func TestClusterUpgradeSuite(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Upgrade Cluster Test Suite")
}

var _ = AfterSuite(func() {
	g := GinkgoT()
	if customflag.ServiceFlag.ClusterConfig.Destroy {
		status, err := factory.DestroyCluster(g)
		Expect(err).NotTo(HaveOccurred())
		Expect(status).To(Equal("cluster destroyed"))
	}
})
