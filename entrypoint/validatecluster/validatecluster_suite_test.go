package validatecluster

import (
	"flag"
	"os"
	"strings"
	"testing"

	"github.com/rancher/distros-test-framework/config"
	"github.com/rancher/distros-test-framework/internal/provisioning"

	"github.com/rancher/distros-test-framework/internal/pkg/customflag"
	"github.com/rancher/distros-test-framework/internal/pkg/qase"
	"github.com/rancher/distros-test-framework/internal/pkg/testcase"
	"github.com/rancher/distros-test-framework/internal/provisioning/qainfra"
	"github.com/rancher/distros-test-framework/internal/report"
	"github.com/rancher/distros-test-framework/internal/resources"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	qaseReport    = os.Getenv("REPORT_TO_QASE")
	flags         *customflag.FlagConfig
	cluster       *resources.Cluster
	infraConfig   qainfra.Config
	cfg           *config.Env
	reportSummary string
	reportErr     error
	err           error
)

func TestMain(m *testing.M) {
	flags = &customflag.ServiceFlag
	flag.Var(&flags.Destroy, "destroy", "Destroy cluster after test")
	flag.Var(&flags.SelinuxTest, "selinux", "Run selinux test")
	flag.Var(&flags.KillAllUninstallTest, "killalluninstall", "Run killall-uninstall test")
	flag.Parse()

	cfg, err = config.AddEnv()
	if err != nil {
		resources.LogLevel("error", "error adding env vars: %w\n", err)
		os.Exit(1)
	}

	checkSelinuxTest()

	setupCluster()

	os.Exit(m.Run())
}

func checkSelinuxTest() {
	if !customflag.ServiceFlag.SelinuxTest {
		resources.LogLevel("info", "Skipping selinux test")
		return
	}

	if !strings.Contains(os.Getenv("server_flags"), "selinux: true") {
		resources.LogLevel("error", "selinux test is enabled but server_flags does not contain selinux: true")
		os.Exit(1)
	}
	resources.LogLevel("info", "Running selinux test")
}

func setupCluster() {
	kubeconfig := os.Getenv("KUBE_CONFIG")
	if kubeconfig != "" {
		// gets a cluster from existing kubeconfig.
		cluster = resources.KubeConfigCluster(kubeconfig)
		resources.LogLevel("info", "Using existing cluster from kubeconfig")

		return
	}

	infraConfig = qainfra.Config{
		Product:        cfg.Product,
		Module:         cfg.Module,
		ResourceName:   cfg.ResourceName,
		Provisioner:    cfg.InfraProvider,
		InstallVersion: cfg.InstallVersion,
		TFVars:         cfg.TFVars,
		QAInfraModule:  cfg.QAInfraModule,
		InfraProvisionerConfig: &qainfra.InfraProvisionerConfig{
			SSHConfig: resources.SSHConfig{
				User:    cfg.SSHUser,
				KeyPath: cfg.SSHKeyPath,
			},
		},
	}

	cluster, err = provisioning.ProvisionInfrastructure(infraConfig, cluster)
	if err != nil {
		resources.LogLevel("error", "error provisioning infrastructure: %w\n", err)
		os.Exit(1)
	}
}

func TestValidateClusterSuite(t *testing.T) {
	RegisterFailHandler(FailWithReport)
	RunSpecs(t, "Validate Cluster Test Suite")
}

var _ = ReportAfterSuite("Validate Cluster Test Suite", func(report Report) {
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
		if customflag.ServiceFlag.KillAllUninstallTest {
			if !strings.Contains(os.Getenv("server_flags"), "docker: true") {
				resources.LogLevel("info", "Running kill all and uninstall tests before destroying the cluster")
				testcase.TestKillAllUninstall(cluster, cfg)
			}
		}

		if customflag.ServiceFlag.SelinuxTest {
			if strings.Contains(os.Getenv("server_flags"), "selinux: true") {
				resources.LogLevel("info", "Running selinux test post killall before cluster destroy with uninstall false")
				testcase.TestUninstallPolicy(cluster, false)
			}
		}

		status, err := provisioning.DestroyInfrastructure(infraConfig.Provisioner, infraConfig.Product, infraConfig.Module)
		Expect(err).NotTo(HaveOccurred())
		Expect(status).To(Equal("cluster destroyed"))
	}
})

func FailWithReport(message string, callerSkip ...int) {
	Fail(message, callerSkip[0]+1)
}
