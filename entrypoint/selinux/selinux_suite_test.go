package selinux

import (
	"flag"
	"os"
	"testing"

	"github.com/rancher/distros-test-framework/config"
	"github.com/rancher/distros-test-framework/entrypoint"
	"github.com/rancher/distros-test-framework/internal/pkg/customflag"
	"github.com/rancher/distros-test-framework/internal/pkg/k8s"
	"github.com/rancher/distros-test-framework/internal/pkg/testcase"
	"github.com/rancher/distros-test-framework/internal/provisioning"
	"github.com/rancher/distros-test-framework/internal/provisioning/driver"
	"github.com/rancher/distros-test-framework/internal/resources"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	cluster     *driver.Cluster
	k8sClient   *k8s.Client
	cfg         *config.Env
	infraConfig *driver.InfraConfig
	err         error
)

func TestMain(m *testing.M) {
	flag.Var(&customflag.ServiceFlag.Destroy, "destroy", "Destroy cluster after test")
	flag.Var(&customflag.ServiceFlag.InstallMode, "installVersionOrCommit", "Install upgrade customflag for version bump")
	flag.Var(&customflag.ServiceFlag.Channel, "channel", "channel to use on install or upgrade")
	flag.Parse()

	cfg, err = config.AddEnv()
	if err != nil {
		resources.LogLevel("error", "error adding env vars: %w\n", err)
		os.Exit(1)
	}
	cluster, infraConfig = entrypoint.SetupClusterInfra(cfg)
	k8sClient, err = k8s.AddClient()
	if err != nil {
		resources.LogLevel("error", "error adding k8s: %w\n", err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

func TestSelinuxSuite(t *testing.T) {
	RegisterFailHandler(entrypoint.FailWithReport)
	RunSpecs(t, "Selinux Test Suite")
}

var _ = AfterSuite(func() {
	if customflag.ServiceFlag.Destroy {
		testcase.TestUninstallPolicy(cluster, true)
		status, err := provisioning.DestroyInfrastructure(
			infraConfig.ProvisionerModule, infraConfig.Product, infraConfig.Module)
		Expect(err).NotTo(HaveOccurred())
		Expect(status).To(Equal("cluster destroyed"))
	}
})
