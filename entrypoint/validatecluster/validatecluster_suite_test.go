package validatecluster

import (
	"flag"
	"os"
	"strings"
	"testing"

	"github.com/rancher/distros-test-framework/config"
	"github.com/rancher/distros-test-framework/internal/provisioning"

	"github.com/rancher/distros-test-framework/entrypoint"
	"github.com/rancher/distros-test-framework/internal/pkg/customflag"
	"github.com/rancher/distros-test-framework/internal/pkg/testcase"
	"github.com/rancher/distros-test-framework/internal/provisioning/driver"
	"github.com/rancher/distros-test-framework/internal/report"
	"github.com/rancher/distros-test-framework/internal/resources"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
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
	flag.Var(&flags.SelinuxTest, "selinux", "Run selinux test")
	flag.Var(&flags.KillAllUninstallTest, "killalluninstall", "Run killall-uninstall test")
	flag.Parse()

	cfg, err = config.AddEnv()
	if err != nil {
		resources.LogLevel("error", "error adding env vars: %w\n", err)
		os.Exit(1)
	}

	entrypoint.CheckSelinuxTest(cfg.ServerFlags, bool(customflag.ServiceFlag.SelinuxTest))
	entrypoint.CheckIngressCompat(cfg)
	cluster, infraConfig = entrypoint.SetupClusterInfra(cfg)
	os.Exit(m.Run())
}

func TestValidateClusterSuite(t *testing.T) {
	RegisterFailHandler(entrypoint.FailWithReport)
	RunSpecs(t, "Validate Cluster Test Suite")
}

var _ = ReportAfterSuite("Validate Cluster Test Suite",
	entrypoint.ReportAfterSuite(&cluster, &reportSummary))

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
				resources.LogLevel("info", "Running uninstall policy test before cluster destroy with uninstall true")
				testcase.TestUninstallPolicy(cluster, true)
			}
		}

		status, err := provisioning.DestroyInfrastructure(infraConfig.ProvisionerModule, infraConfig.Product, infraConfig.Module)
		Expect(err).ToNot(HaveOccurred())
		Expect(status).To(Equal("cluster destroyed"))
	}
})
