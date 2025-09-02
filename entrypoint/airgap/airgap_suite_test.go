package airgap

import (
	"flag"
	"os"
	"strings"
	"testing"

	"github.com/rancher/distros-test-framework/pkg/customflag"
	"github.com/rancher/distros-test-framework/pkg/qase"
	"github.com/rancher/distros-test-framework/shared"
	"github.com/rancher/distros-test-framework/shared/config"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	qaseReport    = os.Getenv("REPORT_TO_QASE")
	flags         *customflag.FlagConfig
	cluster       *shared.Cluster
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
		shared.LogLevel("error", "error adding env vars: %w\n", err)
		os.Exit(1)
	}

	validateAirgap()

	// TODO: Implement using kubeconfig for airgap setup
	cluster = shared.ClusterConfig(cfg.Product, cfg.Module)

	os.Exit(m.Run())
}

// validateAirgap pre-validation for airgap tests.
func validateAirgap() {
	serverFlags := os.Getenv("server_flags")
	cniSlice := []string{"calico", "flannel"}
	if cfg.Module == "" {
		shared.LogLevel("error", "ENV_MODULE is not set, should be airgap\n")
		os.Exit(1)
	}

	if os.Getenv("no_of_bastion_nodes") == "0" {
		shared.LogLevel("error", "no_of_bastion_nodes is not set, should be 1\n")
		os.Exit(1)
	}

	if strings.Contains(os.Getenv("install_mode"), "COMMIT") {
		shared.LogLevel("error", "airgap with commit installs is not supported\n")
		os.Exit(1)
	}

	if cfg.Product == "k3s" {
		if strings.Contains(serverFlags, "protect") {
			shared.LogLevel("error", "airgap with hardened setup is not supported\n")
			os.Exit(1)
		}
		if flags.AirgapFlag.ImageRegistryUrl != "" {
			shared.LogLevel("info", "imageRegistryUrl is not supported for k3s, setting is empty\n")
			flags.AirgapFlag.ImageRegistryUrl = ""
		}
	}

	if cfg.Product == "rke2" {
		if strings.Contains(serverFlags, "profile") {
			shared.LogLevel("error", "airgap with hardened setup is not supported\n")
			os.Exit(1)
		}
		if os.Getenv("no_of_windows_worker_nodes") != "0" {
			if !shared.SliceContainsString(cniSlice, serverFlags) ||
				strings.Contains(serverFlags, "multus") {
				shared.LogLevel("error", "only calico or flannel cni is supported for Windows agent\n")
				shared.LogLevel("error", "found server_flags -> %v\n", serverFlags)
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
		shared.LogLevel("info", "Qase reporting is not enabled")
	}
})

var _ = AfterSuite(func() {
	reportSummary, reportErr = shared.SummaryReportData(cluster, flags)
	if reportErr != nil {
		shared.LogLevel("error", "error getting report summary data: %v\n", reportErr)
	}

	if customflag.ServiceFlag.Destroy {
		status, err := shared.DestroyInfrastructure(cfg.Product, cfg.Module)
		Expect(err).NotTo(HaveOccurred())
		Expect(status).To(Equal("cluster destroyed"))
	}
})

func FailWithReport(message string, callerSkip ...int) {
	Fail(message, callerSkip[0]+1)
}
