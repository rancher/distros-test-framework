package selinux

import (
	"flag"
	"os"
	"testing"

	"github.com/rancher/distros-test-framework/config"
	"github.com/rancher/distros-test-framework/factory"
	"github.com/rancher/distros-test-framework/pkg/customflag"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var cfg *config.Product

func TestMain(m *testing.M) {
	var err error

	flag.Var(&customflag.ServiceFlag.InstallMode, "installVersionOrCommit", "Install upgrade customflag for version bump")
	flag.Var(&customflag.ServiceFlag.Channel, "channel", "channel to use on install or upgrade")
	flag.Parse()

	cfg, err = shared.EnvConfig("entrypoint")
	if err != nil {
		return
	}

	os.Exit(m.Run())
}

func TestSelinuxSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Selinux Test Suite")
}

var _ = AfterSuite(func() {
	g := GinkgoT()
	if customflag.ServiceFlag.ClusterConfig.Destroy {
		status, err := factory.DestroyCluster(g)
		Expect(err).NotTo(HaveOccurred())
		Expect(status).To(Equal("cluster destroyed"))
	}
})
