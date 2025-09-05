package killalluninstall

import (
	"flag"
	"os"
	"strings"
	"testing"

	"github.com/rancher/distros-test-framework/pkg/customflag"
	"github.com/rancher/distros-test-framework/pkg/qase"
	"github.com/rancher/distros-test-framework/shared"
	"github.com/rancher/distros-test-framework/config"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	qaseReport    = os.Getenv("REPORT_TO_QASE")
	flags         *customflag.FlagConfig
	cluster       *shared.Cluster
	cfg           *config.Env
	kubeconfig    string
	reportSummary string
	reportErr     error
	err           error
)

func TestMain(m *testing.M) {
	flags = &customflag.ServiceFlag
	flag.Var(&flags.Destroy, "destroy", "Destroy cluster after test")
	flag.Parse()

	cfg, err = config.AddEnv()
	if err != nil {
		shared.LogLevel("error", "error adding env vars: %w\n", err)
		os.Exit(1)
	}

	kubeconfig = os.Getenv("KUBE_CONFIG")
	if kubeconfig == "" {
		// gets a cluster from terraform.
		cluster = shared.ClusterConfig(cfg.Product, cfg.Module)
	} else {
		// gets a cluster from kubeconfig.
		cluster = shared.KubeConfigCluster(kubeconfig)
	}

	os.Exit(m.Run())
}

func TestKillAllUninstallSuite(t *testing.T) {
	RegisterFailHandler(FailWithReport)
	RunSpecs(t, "killAllUninstall Test Suite")
}

var _ = ReportAfterSuite("killAllUninstall Test Suite", func(report Report) {
	// AddClient Qase reporting capabilities.
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
