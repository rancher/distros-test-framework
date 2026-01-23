package airgap

import (
	"flag"
	"os"
	"strings"
	"testing"

	"github.com/rancher/distros-test-framework/config"
	"github.com/rancher/distros-test-framework/internal/pkg/customflag"
	"github.com/rancher/distros-test-framework/internal/pkg/qase"
	"github.com/rancher/distros-test-framework/internal/provisioning"
	"github.com/rancher/distros-test-framework/internal/provisioning/driver"
	"github.com/rancher/distros-test-framework/internal/provisioning/legacy"
	"github.com/rancher/distros-test-framework/internal/report"
	"github.com/rancher/distros-test-framework/internal/resources"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	qaseReport    = os.Getenv("REPORT_TO_QASE")
	flags         *customflag.FlagConfig
	cluster       *driver.Cluster
	infraConfig   *driver.InfraConfig
	cfg           *config.Env
	reportSummary string
	reportErr     error
	err           error
)

func TestMain(m *testing.M) {
	flags = &customflag.ServiceFlag
	flag.Var(&flags.Destroy, "destroy", "Destroy cluster after test")
	flag.StringVar(&flags.AirgapFlag.ImageRegistryUrl, "imageRegistryUrl", "", "image registry url to get the images from")
	flag.StringVar(&flags.AirgapFlag.RegistryUsername, "registryUsername", "testuser", "private registry username")
	flag.StringVar(&flags.AirgapFlag.RegistryPassword, "registryPassword", "testpass765", "private registry password")
	flag.StringVar(&flags.AirgapFlag.TarballType, "tarballType", "", "artifact tarball type")
	flag.Parse()

	cfg, err = config.AddEnv()
	if err != nil {
		resources.LogLevel("error", "error adding env vars: %w\n", err)
		os.Exit(1)
	}

	setupClusterInfra()

	validateAirgap()

	os.Exit(m.Run())
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

// validateAirgap pre-validation for airgap tests.
func validateAirgap() {
	serverFlags := os.Getenv("server_flags")
	cniSlice := []string{"calico", "flannel"}
	if cfg.Module == "" {
		resources.LogLevel("error", "ENV_MODULE is not set, should be airgap\n")
		os.Exit(1)
	}

	if os.Getenv("no_of_bastion_nodes") == "0" {
		resources.LogLevel("error", "no_of_bastion_nodes is not set, should be 1\n")
		os.Exit(1)
	}

	if strings.Contains(os.Getenv("install_mode"), "COMMIT") {
		resources.LogLevel("error", "airgap with commit installs is not supported\n")
		os.Exit(1)
	}

	if cfg.Product == "k3s" {
		if strings.Contains(serverFlags, "protect") {
			resources.LogLevel("error", "airgap with hardened setup is not supported\n")
			os.Exit(1)
		}
		if flags.AirgapFlag.ImageRegistryUrl != "" {
			resources.LogLevel("info", "imageRegistryUrl is not supported for k3s, setting is empty\n")
			flags.AirgapFlag.ImageRegistryUrl = ""
		}
	}

	if cfg.Product == "rke2" {
		if strings.Contains(serverFlags, "profile") {
			resources.LogLevel("error", "airgap with hardened setup is not supported\n")
			os.Exit(1)
		}
		if os.Getenv("no_of_windows_worker_nodes") != "0" {
			if !resources.SliceContainsString(cniSlice, serverFlags) ||
				strings.Contains(serverFlags, "multus") {
				resources.LogLevel("error", "only calico or flannel cni is supported for Windows agent\n")
				resources.LogLevel("error", "found server_flags -> %v\n", serverFlags)
				os.Exit(1)
			}
		}
	}
}

func TestAirgapClusterSuite(t *testing.T) {
	RegisterFailHandler(FailWithReport)
	RunSpecs(t, "Create Airgap Cluster Test Suite")
}

var _ = ReportAfterSuite("Create Airgap Cluster Test Suite", func(report Report) {
	// Add Qase reporting capabilities.
	if strings.ToLower(qaseReport) == "true" {
		qaseClient, err := qase.AddQase()
		Expect(err).ToNot(HaveOccurred(), "error adding qase")

		qaseClient.SpecReportTestResults(qaseClient.Ctx, cluster, &report, reportSummary)
	} else {
		resources.LogLevel("info", "Qase reporting is not enabled")
	}
})

var _ = AfterSuite(func() {
	reportSummary, reportErr = report.SummaryReportData(cluster, flags)
	if reportErr != nil {
		resources.LogLevel("error", "error getting report summary data: %v\n", reportErr)
	}

	if customflag.ServiceFlag.Destroy {
		status, err := provisioning.DestroyInfrastructure(
			infraConfig.ProvisionerModule, infraConfig.Product, infraConfig.Module)
		Expect(err).ToNot(HaveOccurred())
		Expect(status).To(Equal("cluster destroyed"))
	}
})

func FailWithReport(message string, callerSkip ...int) {
	Fail(message, callerSkip[0]+1)
}
