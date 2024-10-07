package airgap

import (
	"flag"
	"os"
	"strings"
	"testing"

	"github.com/rancher/distros-test-framework/config"
	"github.com/rancher/distros-test-framework/pkg/customflag"
	"github.com/rancher/distros-test-framework/pkg/qase"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	qaseReport = os.Getenv("REPORT_TO_QASE")
	flags      *customflag.FlagConfig
	cluster    *shared.Cluster
	cfg        *config.Product
	err        error
)

func TestMain(m *testing.M) {
	flags = &customflag.ServiceFlag
	flag.Var(&flags.Destroy, "destroy", "Destroy cluster after test")
	flag.StringVar(&flags.AirgapFlag.RegistryUsername, "registryUsername", "testuser", "private registry username")
	flag.StringVar(&flags.AirgapFlag.RegistryPassword, "registryPassword", "testpass765", "private registry password")
	flag.StringVar(&flags.AirgapFlag.TarballType, "tarballType", "", "artifact tarball type")
	flag.Parse()

	cfg, err = config.AddEnv()
	if err != nil {
		shared.LogLevel("error", "error adding env vars: %w\n", err)
		os.Exit(1)
	}

	// Validate the right module is set.
	validateAirgap()

	// TODO: Implement using kubeconfig for airgap setup
	cluster = shared.ClusterConfig()

	os.Exit(m.Run())
}

func validateAirgap() {
	if os.Getenv("ENV_MODULE") == "" {
		shared.LogLevel("error", "ENV_MODULE is not set, should be airgap\n")
		os.Exit(1)
	}

	if os.Getenv("no_of_bastion_nodes") == "0" {
		shared.LogLevel("error", "no_of_bastion_nodes is not set, should be 1\n")
		os.Exit(1)
	}

	if os.Getenv("arch") == "arm" {
		shared.LogLevel("error", "airgap with arm architecture is not supported\n")
		os.Exit(1)
	}

	if strings.Contains(os.Getenv("install_mode"), "COMMIT") {
		shared.LogLevel("error", "airgap with commit installs is not supported\n")
		os.Exit(1)
	}

	if (cfg.Product == "k3s" && strings.Contains(os.Getenv("server_flags"), "protect")) ||
		(cfg.Product == "rke2" && strings.Contains(os.Getenv("server_flags"), "profile")) {
		shared.LogLevel("error", "airgap with hardened setup is not supported\n")
		os.Exit(1)
	}
}

func TestAirgapSuite(t *testing.T) {
	RegisterFailHandler(FailWithReport)
	RunSpecs(t, "Create Airgap Cluster Test Suite")
}

var _ = ReportAfterSuite("Test Restart Service", func(report Report) {
	// Add Qase reporting capabilities.
	if qaseReport == "true" {
		qaseClient, err := qase.AddQase()
		if err != nil {
			shared.LogLevel("error", "error adding qase: %w\n", err)
		}

		qaseClient.ReportTestResults(qaseClient.Ctx, report)
	} else {
		shared.LogLevel("info", "Qase reporting is not enabled")
	}
})

var _ = AfterSuite(func() {
	if customflag.ServiceFlag.Destroy {
		status, err := shared.DestroyCluster()
		Expect(err).NotTo(HaveOccurred())
		Expect(status).To(Equal("cluster destroyed"))
	}
})

func FailWithReport(message string, callerSkip ...int) {
	Fail(message, callerSkip[0]+1)
}
