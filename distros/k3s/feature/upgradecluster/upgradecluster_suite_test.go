package upgradecluster

import (
	"flag"
	"os"
	"testing"

	"github.com/rancher/distros-test-framework/cmd"
	"github.com/rancher/distros-test-framework/lib/cluster"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestMain(m *testing.M) {
	flag.Var(&cmd.ServiceFlag.ClusterConfig.Product, "product", "Distro to create cluster and run the tests")
	flag.Var(&cmd.ServiceFlag.InstallUpgrade, "installVersionOrCommit", "Upgrade to run with type=value,"+
		"INSTALL_K3S_VERSION=v1.26.2+k3s1 or INSTALL_K3S_COMMIT=1823dsad7129873192873129asd")
	flag.StringVar(&cmd.ServiceFlag.InstallType.Channel, "channel", "", "channel to use on install or upgrade")
	flag.Var(&cmd.ServiceFlag.ClusterConfig.Destroy, "destroy", "Destroy cluster after test")
	flag.Var(&cmd.ServiceFlag.ClusterConfig.Arch, "arch", "Architecture type")

	flag.Parse()

	os.Exit(m.Run())
}

func TestClusterUpgradeSuite(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Upgrade Cluster Test Suite")
}

var _ = AfterSuite(func() {
	g := GinkgoT()
	if cmd.ServiceFlag.ClusterConfig.Destroy {
		status, err := activity.DestroyCluster(g, cmd.ServiceFlag.ClusterConfig.Product.String())
		Expect(err).NotTo(HaveOccurred())
		Expect(status).To(Equal("cluster destroyed"))
	}
})
