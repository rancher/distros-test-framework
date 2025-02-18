package sonobuoyconformance

import (
	"flag"
	"os"
	"strconv"
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
	kubeconfig string
	cluster    *shared.Cluster
	cfg        *config.Env
	err        error
)

func TestMain(m *testing.M) {
	flag.StringVar(&customflag.ServiceFlag.External.SonobuoyVersion, "sonobuoyVersion", "0.57.2", "Sonobuoy binary version")
	flag.Var(&customflag.ServiceFlag.Destroy, "destroy", "Destroy cluster after test")
	flag.Parse()

	verifyClusterNodes()

	cfg, err = config.AddEnv()
	if err != nil {
		shared.LogLevel("error", "error adding env vars: %w\n", err)
		os.Exit(1)
	}

	kubeconfig = os.Getenv("KUBE_CONFIG")
	if kubeconfig == "" {
		// gets a cluster from terraform.
		cluster = shared.ClusterConfig(cfg)
	} else {
		// gets a cluster from kubeconfig.
		cluster = shared.KubeConfigCluster(kubeconfig)
	}

	os.Exit(m.Run())
	os.Exit(1)
}

func TestConformance(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Run Conformance Suite")
}

var _ = ReportAfterSuite("Conformance Suite", func(report Report) {

	if strings.ToLower(qaseReport) == "true" {
		qaseClient, err := qase.AddQase()
		Expect(err).ToNot(HaveOccurred(), "error adding qase")

		qaseClient.SpecReportTestResults(qaseClient.Ctx, &report, cfg.InstallVersion)
	} else {
		shared.LogLevel("info", "Qase reporting is not enabled")
	}
})

var _ = AfterSuite(func() {
	if customflag.ServiceFlag.Destroy {
		status, err := shared.DestroyCluster(cfg)
		Expect(err).NotTo(HaveOccurred())
		Expect(status).To(Equal("cluster destroyed"))
	}
})

func verifyClusterNodes() {
	// if re-running locally the env variables are not set after cleanup
	shared.LogLevel("info", "verying cluster configuration matches minimum requirements for conformance tests")
	serverNum, err := strconv.Atoi(os.Getenv("no_of_server_nodes"))
	if err != nil {
		shared.LogLevel("error", "error converting no_of_server_nodes to int: %w", err)
		os.Exit(1)
	}

	agentNum, _ := strconv.Atoi(os.Getenv("no_of_agent_nodes"))
	if err != nil {
		shared.LogLevel("error", "error converting no_of_agent_nodes to int: %w", err)
		os.Exit(1)
	}

	if serverNum < 1 && agentNum < 1 {
		shared.LogLevel("error", "%s", "cluster must at least consist of 1 server and 1 agent")
		os.Exit(1)
	}

}
