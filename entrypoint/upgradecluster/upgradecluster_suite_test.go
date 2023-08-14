package upgradecluster

import (
	"flag"
	"fmt"
	"os"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/distros-test-framework/config"
	"github.com/rancher/distros-test-framework/factory"
	"github.com/rancher/distros-test-framework/pkg/customflag"
)

var cfg config.ProductConfig

func TestMain(m *testing.M) {
	var err error
	flag.Var(&customflag.ServiceFlag.InstallUpgrade, "installVersionOrCommit",
		"Upgrade with version or commit")
	flag.StringVar(&customflag.ServiceFlag.InstallType.Channel, "channel", "",
		"channel to use on install or upgrade")
	flag.Var(&customflag.ServiceFlag.ClusterConfig.Destroy, "destroy",
		"Destroy cluster after test")
	flag.Var(&customflag.ServiceFlag.UpgradeVersionSUC, "upgradeVersion", "Upgrade SUC model")

	flag.Parse()

	cfg, err = config.LoadConfigEnv("../../config")
	if err != nil {
		fmt.Println(err)
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
	if customflag.ServiceFlag.ClusterConfig.Destroy {
		status, err := factory.DestroyCluster(g)
		Expect(err).NotTo(HaveOccurred())
		Expect(status).To(Equal("cluster destroyed"))
	}
})
