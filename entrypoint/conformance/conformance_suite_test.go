package sonobuoyconformance

import (
	"flag"
	"os"
	"strconv"
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
	kubeconfig    string
	cluster       *shared.Cluster
	flags         *customflag.FlagConfig
	cfg           *config.Env
	reportSummary string
	reportErr     error
	err           error
)

func TestMain(m *testing.M) {
	flags = &customflag.ServiceFlag
	flag.StringVar(&customflag.ServiceFlag.External.SonobuoyVersion, "sonobuoyVersion", "0.57.3", "Sonobuoy binary version")
	flag.Var(&customflag.ServiceFlag.Destroy, "destroy", "Destroy cluster after test")
	flag.Parse()

	cfg, err = config.AddEnv()
	if err != nil {
		shared.LogLevel("error", "error adding env vars: %w\n", err)
		os.Exit(1)
	}

	verifyClusterNodes()

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

func TestConformance(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Run Conformance Suite")
}

var _ = ReportAfterSuite("Conformance Suite", func(report Report) {
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

func verifyClusterNodes() {
	shared.LogLevel("info", "verying cluster configuration matches minimum requirements for conformance tests")
	s, serverErr := strconv.Atoi(os.Getenv("no_of_server_nodes"))
	w, workerErr := strconv.Atoi(os.Getenv("no_of_worker_nodes"))

	if serverErr != nil || workerErr != nil {
		shared.LogLevel("error", "Failed to convert node counts to integers: %v, %v", serverErr, workerErr)
		os.Exit(1)
	}

	if s < 1 && w < 1 {
		shared.LogLevel("error", "%s", "cluster must at least consist of 1 server and 1 agent")
		os.Exit(1)
	}
}
