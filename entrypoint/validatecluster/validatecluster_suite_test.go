package validatecluster

import (
	"flag"
	"os"
	"strings"
	"testing"

	"github.com/rancher/distros-test-framework/pkg/customflag"
	"github.com/rancher/distros-test-framework/pkg/qase"
	"github.com/rancher/distros-test-framework/pkg/testcase"
	"github.com/rancher/distros-test-framework/shared"
	"github.com/rancher/distros-test-framework/shared/config"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	qaseReport    = os.Getenv("REPORT_TO_QASE")
	flags         *customflag.FlagConfig
	cluster       *shared.Cluster
	infraConfig   shared.InfraConfig
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
		shared.LogLevel("error", "error adding env vars: %w\n", err)
		os.Exit(1)
	}

	checkSelinuxTest()

	setupCluster()

	os.Exit(m.Run())
}

func checkSelinuxTest() {
	if !customflag.ServiceFlag.SelinuxTest {
		shared.LogLevel("info", "Skipping selinux test")
		return
	}

	if !strings.Contains(os.Getenv("server_flags"), "selinux: true") {
		shared.LogLevel("error", "selinux test is enabled but server_flags does not contain selinux: true")
		os.Exit(1)
	}
	shared.LogLevel("info", "Running selinux test")
}

func setupCluster() {
	kubeconfig := os.Getenv("KUBE_CONFIG")
	if kubeconfig != "" {
		// gets a cluster from existing kubeconfig.
		cluster = shared.KubeConfigCluster(kubeconfig)
		shared.LogLevel("info", "Using existing cluster from kubeconfig")

		return
	}

	infraConfig = shared.InfraConfig{
		Product:        cfg.Product,
		Module:         cfg.Module,
		ResourceName:   cfg.ResourceName,
		InfraProvider:  cfg.InfraProvider,
		InstallVersion: cfg.InstallVersion,
		TFVars:         cfg.TFVars,
		QAInfraModule:  cfg.QAInfraModule,
		QAInfraConfig: &shared.QAInfraConfig{
			SSHConfig: shared.SSHConfig{
				User:    cfg.SSHUser,
				KeyPath: cfg.SSHKeyPath,
			},
		},
	}

	cluster, err = shared.ProvisionInfrastructure(infraConfig, cluster)
	if err != nil {
		shared.LogLevel("error", "error provisioning infrastructure: %w\n", err)
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
		shared.LogLevel("info", "Qase reporting is not enabled")
	}
})

var _ = AfterSuite(func() {
	reportSummary, reportErr = shared.SummaryReportData(cluster, flags)
	if reportErr != nil {
		shared.LogLevel("error", "error getting report summary data: %v\n", reportErr)
	}

	if customflag.ServiceFlag.Destroy {
		if customflag.ServiceFlag.KillAllUninstallTest {
			if !strings.Contains(os.Getenv("server_flags"), "docker: true") {
				shared.LogLevel("info", "Running kill all and uninstall tests before destroying the cluster")
				testcase.TestKillAllUninstall(cluster, cfg)
			}
		}

		if customflag.ServiceFlag.SelinuxTest {
			if strings.Contains(os.Getenv("server_flags"), "selinux: true") {
				shared.LogLevel("info", "Running selinux test post killall before cluster destroy with uninstall false")
				testcase.TestUninstallPolicy(cluster, false)
			}
		}

		status, err := shared.DestroyInfrastructure(infraConfig.Product, infraConfig.Module)
		Expect(err).NotTo(HaveOccurred())
		Expect(status).To(Equal("cluster destroyed"))
	}
})

func FailWithReport(message string, callerSkip ...int) {
	Fail(message, callerSkip[0]+1)
}
