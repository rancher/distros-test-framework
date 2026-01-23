package selinux

import (
	"flag"
	"os"
	"testing"

	"github.com/rancher/distros-test-framework/config"
	"github.com/rancher/distros-test-framework/internal/pkg/customflag"
	"github.com/rancher/distros-test-framework/internal/pkg/k8s"
	"github.com/rancher/distros-test-framework/internal/pkg/testcase"
	"github.com/rancher/distros-test-framework/internal/provisioning"
	"github.com/rancher/distros-test-framework/internal/provisioning/driver"
	"github.com/rancher/distros-test-framework/internal/provisioning/legacy"
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

	setupClusterInfra()

	k8sClient, err = k8s.AddClient()
	if err != nil {
		resources.LogLevel("error", "error adding k8s: %w\n", err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

func TestSelinuxSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Selinux Test Suite")
}

func setupClusterInfra() {
	kubeconfig := os.Getenv("KUBE_CONFIG")
	if kubeconfig != "" {
		// gets a cluster from existing kubeconfig.
		cluster = legacy.KubeConfigCluster(kubeconfig)
		resources.LogLevel("info", "Using existing cluster from kubeconfig")

		return
	}

	// initial data load needed for provisioning coming from config env vars.
	infraConfig = &driver.InfraConfig{
		Product:           cfg.Product,
		Module:            cfg.Module,
		ResourceName:      cfg.ResourceName,
		ProvisionerModule: cfg.ProvisionerModule,
		ProvisionerType:   cfg.ProvisionerType,
		InstallVersion:    cfg.InstallVersion,
		QAInfraProvider:   cfg.QAInfraProvider,
		NodeOS:            cfg.NodeOS,
		CNI:               cfg.CNI,
		Cluster: &driver.Cluster{
			Config: driver.Config{
				Arch:        cfg.Arch,
				ServerFlags: cfg.ServerFlags,
				WorkerFlags: cfg.WorkerFlags,
				Channel:     cfg.Channel,
			},
			SSH: driver.SSHConfig{
				User:        cfg.SSHUser,
				PrivKeyPath: cfg.SSHKeyPath,
				KeyName:     cfg.SSHKeyName,
			},
		},
	}

	cluster, err = provisioning.ProvisionInfrastructure(infraConfig)
	if err != nil {
		resources.LogLevel("error", "error provisioning infrastructure: %w\n", err)
		os.Exit(1)
	}

	resources.LogLevel("info", "Cluster provisioned successfully with %+v", cluster.Config)
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
